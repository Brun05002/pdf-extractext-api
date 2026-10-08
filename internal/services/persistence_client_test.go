package services

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestClient(t *testing.T, handler http.Handler) *HTTPPersistenceClient {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	return NewHTTPPersistenceClient(server.URL, 5*time.Second)
}

func TestSave_Created(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/documents", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var doc Document
		require.NoError(t, json.NewDecoder(r.Body).Decode(&doc))
		doc.ID = "doc-123"

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(doc)
	}))

	saved, err := client.Save(context.Background(), &Document{Filename: "a.pdf", Content: "texto", PageCount: 3})
	require.NoError(t, err)
	assert.Equal(t, "doc-123", saved.ID)
	assert.Equal(t, "a.pdf", saved.Filename)
}

func TestSave_Conflict(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))

	_, err := client.Save(context.Background(), &Document{Filename: "a.pdf"})
	assert.ErrorIs(t, err, ErrConflict)
}

func TestFindByID_OK(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/documents/doc-123", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Document{ID: "doc-123", Filename: "a.pdf", Content: "texto", PageCount: 3})
	}))

	doc, err := client.FindByID(context.Background(), "doc-123")
	require.NoError(t, err)
	assert.Equal(t, "doc-123", doc.ID)
	assert.Equal(t, 3, doc.PageCount)
}

func TestFindByID_NotFound(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := client.FindByID(context.Background(), "inexistente")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestFindAll_OK(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodGet, r.Method)
		assert.Equal(t, "/documents", r.URL.Path)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]*Document{
			{ID: "doc-1", Filename: "a.pdf"},
			{ID: "doc-2", Filename: "b.pdf"},
		})
	}))

	docs, err := client.FindAll(context.Background())
	require.NoError(t, err)
	require.Len(t, docs, 2)
	assert.Equal(t, "doc-1", docs[0].ID)
}

func TestUpdate_OK(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "/documents/doc-1", r.URL.Path)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		var doc Document
		require.NoError(t, json.NewDecoder(r.Body).Decode(&doc))
		assert.Equal(t, "renombrado.pdf", doc.Filename)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(Document{ID: "doc-1", Filename: doc.Filename, PageCount: 3})
	}))

	updated, err := client.Update(context.Background(), "doc-1", &Document{Filename: "renombrado.pdf"})
	require.NoError(t, err)
	assert.Equal(t, "doc-1", updated.ID)
	assert.Equal(t, "renombrado.pdf", updated.Filename)
}

func TestUpdate_NotFound(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	_, err := client.Update(context.Background(), "inexistente", &Document{Filename: "x.pdf"})
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestUpdate_Conflict(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
	}))

	_, err := client.Update(context.Background(), "doc-1", &Document{Filename: "duplicado.pdf"})
	assert.ErrorIs(t, err, ErrConflict)
}

func TestDelete_NoContent(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodDelete, r.Method)
		assert.Equal(t, "/documents/doc-1", r.URL.Path)
		w.WriteHeader(http.StatusNoContent)
	}))

	require.NoError(t, client.Delete(context.Background(), "doc-1"))
}

func TestDelete_NotFound(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))

	assert.ErrorIs(t, client.Delete(context.Background(), "inexistente"), ErrNotFound)
}

func TestFindAll_UpstreamError(t *testing.T) {
	client := newTestClient(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))

	_, err := client.FindAll(context.Background())
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrNotFound))
	assert.Contains(t, err.Error(), "500")
}
