package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// Sin X-Request-ID entrante el middleware genera uno y los handlers deben
// propagarlo a los upstreams (regresión de P1: se leía el header, no Locals).
func TestRequestID_GeneratedWithoutHeader_PropagatedToUpstreams(t *testing.T) {
	isNonEmpty := mock.MatchedBy(func(rid string) bool { return rid != "" })

	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, isNonEmpty, "documento.pdf", mock.Anything).
		Return(validC1Result(), nil).Once()

	repo := new(MockPersistenceRepository)
	repo.On("Save", mock.Anything, isNonEmpty, mock.Anything).
		Return(&services.Document{ID: "doc-1"}, nil).Once()

	app := newUploadApp(client, repo)
	resp, err := app.Test(uploadRequest(t, "file", "documento.pdf", []byte("pdf"), ""))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
	assert.NotEmpty(t, resp.Header.Get("X-Request-ID"), "la respuesta debe incluir el ID generado")
	client.AssertExpectations(t)
	repo.AssertExpectations(t)
}

// Con X-Request-ID entrante se propaga el valor del cliente, sin generarlo.
func TestRequestID_IncomingHeader_PropagatedToUpstreams(t *testing.T) {
	const incoming = "req-cliente-42"

	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, incoming, "documento.pdf", mock.Anything).
		Return(validC1Result(), nil).Once()

	repo := new(MockPersistenceRepository)
	repo.On("Save", mock.Anything, incoming, mock.Anything).
		Return(&services.Document{ID: "doc-1"}, nil).Once()

	app := newUploadApp(client, repo)
	resp, err := app.Test(uploadRequest(t, "file", "documento.pdf", []byte("pdf"), incoming))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
	assert.Equal(t, incoming, resp.Header.Get("X-Request-ID"))
	client.AssertExpectations(t)
	repo.AssertExpectations(t)
}

// GET /documents también debe enviar el ID generado (Locals) al upstream.
func TestRequestID_Generated_ListDocumentsPropagated(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("FindAll", mock.Anything, mock.MatchedBy(func(rid string) bool {
		return rid != ""
	})).Return([]*services.Document{}, nil).Once()

	app := newDocumentsApp(repo)
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, "/documents", nil))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	repo.AssertExpectations(t)
}
