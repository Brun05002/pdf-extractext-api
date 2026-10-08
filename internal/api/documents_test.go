package api

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockPersistenceRepository simula el microservicio de persistencia.
type MockPersistenceRepository struct {
	mock.Mock
}

func (m *MockPersistenceRepository) Save(ctx context.Context, doc *services.Document) (*services.Document, error) {
	args := m.Called(ctx, doc)
	if result, ok := args.Get(0).(*services.Document); ok {
		return result, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockPersistenceRepository) FindByID(ctx context.Context, id string) (*services.Document, error) {
	args := m.Called(ctx, id)
	if result, ok := args.Get(0).(*services.Document); ok {
		return result, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockPersistenceRepository) FindAll(ctx context.Context) ([]*services.Document, error) {
	args := m.Called(ctx)
	if result, ok := args.Get(0).([]*services.Document); ok {
		return result, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockPersistenceRepository) Update(ctx context.Context, id string, doc *services.Document) (*services.Document, error) {
	args := m.Called(ctx, id, doc)
	if result, ok := args.Get(0).(*services.Document); ok {
		return result, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockPersistenceRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

// newDocumentsApp incluye el CustomErrorHandler para probar la traducción
// de errores de dominio end-to-end.
func newDocumentsApp(repo services.PersistenceRepository) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: CustomErrorHandler})
	RegisterDocuments(app, repo)
	return app
}

func getJSON(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestListDocuments_OK(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("FindAll", mock.Anything).Return([]*services.Document{
		{ID: "doc-1", Filename: "a.pdf", Content: "texto A", PageCount: 1},
		{ID: "doc-2", Filename: "b.pdf", Content: "texto B", PageCount: 2},
	}, nil)

	status, body := getJSON(t, newDocumentsApp(repo), "/documents")

	assert.Equal(t, fiber.StatusOK, status)
	assert.JSONEq(t, `[
		{"id":"doc-1","filename":"a.pdf","content":"texto A","page_count":1},
		{"id":"doc-2","filename":"b.pdf","content":"texto B","page_count":2}
	]`, body)
	repo.AssertExpectations(t)
}

func TestGetDocumentByID_OK(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("FindByID", mock.Anything, "doc-1").
		Return(&services.Document{ID: "doc-1", Filename: "a.pdf", Content: "texto A", PageCount: 1}, nil)

	status, body := getJSON(t, newDocumentsApp(repo), "/documents/doc-1")

	assert.Equal(t, fiber.StatusOK, status)
	assert.JSONEq(t, `{"id":"doc-1","filename":"a.pdf","content":"texto A","page_count":1}`, body)
	repo.AssertExpectations(t)
}

func TestGetDocumentByID_NotFound(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("FindByID", mock.Anything, "inexistente").Return(nil, services.ErrNotFound)

	status, body := getJSON(t, newDocumentsApp(repo), "/documents/inexistente")

	assert.Equal(t, fiber.StatusNotFound, status)
	assert.JSONEq(t, `{"code":"NOT_FOUND","message":"documento no encontrado"}`, body)
	repo.AssertExpectations(t)
}

func TestUpdateDocument_OK(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("Update", mock.Anything, "doc-1", mock.MatchedBy(func(d *services.Document) bool {
		return d.Filename == "renombrado.pdf"
	})).Return(&services.Document{ID: "doc-1", Filename: "renombrado.pdf", PageCount: 3}, nil)

	req := httptest.NewRequest(http.MethodPatch, "/documents/doc-1",
		strings.NewReader(`{"filename":"renombrado.pdf"}`))
	req.Header.Set("Content-Type", "application/json")

	app := newDocumentsApp(repo)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.JSONEq(t, `{"id":"doc-1","filename":"renombrado.pdf","content":"","page_count":3}`, string(body))
	repo.AssertExpectations(t)
}

func TestUpdateDocument_InvalidBody(t *testing.T) {
	repo := new(MockPersistenceRepository)

	req := httptest.NewRequest(http.MethodPatch, "/documents/doc-1", strings.NewReader(`{json-malformado`))
	req.Header.Set("Content-Type", "application/json")

	app := newDocumentsApp(repo)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	assert.JSONEq(t, `{"code":"BAD_REQUEST","message":"el cuerpo de la petición no es un JSON válido"}`, string(body))
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything)
}

func TestUpdateDocument_NotFound(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("Update", mock.Anything, "inexistente", mock.Anything).Return(nil, services.ErrNotFound)

	req := httptest.NewRequest(http.MethodPatch, "/documents/inexistente",
		strings.NewReader(`{"filename":"x.pdf"}`))
	req.Header.Set("Content-Type", "application/json")

	app := newDocumentsApp(repo)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assert.JSONEq(t, `{"code":"NOT_FOUND","message":"documento no encontrado"}`, string(body))
	repo.AssertExpectations(t)
}

func TestDeleteDocument_NoContent(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("Delete", mock.Anything, "doc-1").Return(nil)

	req := httptest.NewRequest(http.MethodDelete, "/documents/doc-1", nil)
	app := newDocumentsApp(repo)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)
	repo.AssertExpectations(t)
}

func TestDeleteDocument_NotFound(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("Delete", mock.Anything, "inexistente").Return(services.ErrNotFound)

	req := httptest.NewRequest(http.MethodDelete, "/documents/inexistente", nil)
	app := newDocumentsApp(repo)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assert.JSONEq(t, `{"code":"NOT_FOUND","message":"documento no encontrado"}`, string(body))
	repo.AssertExpectations(t)
}

func TestListDocuments_UpstreamError(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("FindAll", mock.Anything).Return(nil, errors.New("persistence upstream: connection refused"))

	status, body := getJSON(t, newDocumentsApp(repo), "/documents")

	assert.Equal(t, fiber.StatusInternalServerError, status)
	assert.JSONEq(t, `{"code":"INTERNAL_ERROR","message":"error interno del servidor"}`, body)
	repo.AssertExpectations(t)
}
