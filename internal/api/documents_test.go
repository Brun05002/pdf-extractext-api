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
	"github.com/pdf-extractext/api/internal/core"
	"github.com/pdf-extractext/api/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

// MockPersistenceRepository simula el microservicio de persistencia.
type MockPersistenceRepository struct {
	mock.Mock
}

func (m *MockPersistenceRepository) Save(ctx context.Context, requestID string, doc *services.Document) (*services.Document, error) {
	args := m.Called(ctx, requestID, doc)
	if result, ok := args.Get(0).(*services.Document); ok {
		return result, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockPersistenceRepository) FindByID(ctx context.Context, requestID, id string) (*services.Document, error) {
	args := m.Called(ctx, requestID, id)
	if result, ok := args.Get(0).(*services.Document); ok {
		return result, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockPersistenceRepository) FindAll(ctx context.Context, requestID string) ([]*services.Document, error) {
	args := m.Called(ctx, requestID)
	if result, ok := args.Get(0).([]*services.Document); ok {
		return result, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockPersistenceRepository) Update(ctx context.Context, requestID, id string, doc *services.Document) (*services.Document, error) {
	args := m.Called(ctx, requestID, id, doc)
	if result, ok := args.Get(0).(*services.Document); ok {
		return result, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockPersistenceRepository) Delete(ctx context.Context, requestID, id string) error {
	args := m.Called(ctx, requestID, id)
	return args.Error(0)
}

// newDocumentsApp incluye el CustomErrorHandler para probar la traducción
// de errores de dominio end-to-end.
func newDocumentsApp(repo services.PersistenceRepository) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: CustomErrorHandler})
	app.Use(core.RequestID())
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
	repo.On("FindAll", mock.Anything, mock.Anything).Return([]*services.Document{
		{ID: "507f1f77bcf86cd799439011", Filename: "a.pdf", ExtractedText: "texto A", Checksum: "sum-a"},
		{ID: "doc-2", Filename: "b.pdf", ExtractedText: "texto B", Checksum: "sum-b"},
	}, nil)

	status, body := getJSON(t, newDocumentsApp(repo), "/documents")

	assert.Equal(t, fiber.StatusOK, status)
	assert.JSONEq(t, `[
		{"id":"507f1f77bcf86cd799439011","filename":"a.pdf","extracted_text":"texto A","checksum":"sum-a"},
		{"id":"doc-2","filename":"b.pdf","extracted_text":"texto B","checksum":"sum-b"}
	]`, body)
	repo.AssertExpectations(t)
}

func TestGetDocumentByID_OK(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("FindByID", mock.Anything, mock.Anything, "507f1f77bcf86cd799439011").
		Return(&services.Document{ID: "507f1f77bcf86cd799439011", Filename: "a.pdf", ExtractedText: "texto A", Checksum: "sum-a"}, nil)

	status, body := getJSON(t, newDocumentsApp(repo), "/documents/507f1f77bcf86cd799439011")

	assert.Equal(t, fiber.StatusOK, status)
	assert.JSONEq(t, `{"id":"507f1f77bcf86cd799439011","filename":"a.pdf","extracted_text":"texto A","checksum":"sum-a"}`, body)
	repo.AssertExpectations(t)
}

func TestGetDocumentByID_NotFound(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("FindByID", mock.Anything, mock.Anything, "507f1f77bcf86cd799439012").Return(nil, services.ErrNotFound)

	status, body := getJSON(t, newDocumentsApp(repo), "/documents/507f1f77bcf86cd799439012")

	assert.Equal(t, fiber.StatusNotFound, status)
	assertProblemBody(t, body, fiber.StatusNotFound, "NOT_FOUND", "not-found", "documento no encontrado")
	repo.AssertExpectations(t)
}

func TestUpdateDocument_OK(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("Update", mock.Anything, mock.Anything, "507f1f77bcf86cd799439011", mock.MatchedBy(func(d *services.Document) bool {
		return d.Filename == "renombrado.pdf"
	})).Return(&services.Document{ID: "507f1f77bcf86cd799439011", Filename: "renombrado.pdf", ExtractedText: "texto", Checksum: "sum-1"}, nil)

	req := httptest.NewRequest(http.MethodPatch, "/documents/507f1f77bcf86cd799439011",
		strings.NewReader(`{"filename":"renombrado.pdf"}`))
	req.Header.Set("Content-Type", "application/json")

	app := newDocumentsApp(repo)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.JSONEq(t, `{"id":"507f1f77bcf86cd799439011","filename":"renombrado.pdf","extracted_text":"texto","checksum":"sum-1"}`, string(body))
	repo.AssertExpectations(t)
}

func TestUpdateDocument_InvalidBody(t *testing.T) {
	repo := new(MockPersistenceRepository)

	req := httptest.NewRequest(http.MethodPatch, "/documents/507f1f77bcf86cd799439011", strings.NewReader(`{json-malformado`))
	req.Header.Set("Content-Type", "application/json")

	app := newDocumentsApp(repo)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	assertProblemBody(t, string(body), fiber.StatusBadRequest, "INVALID_REQUEST", "invalid-request", "el cuerpo de la petición no es un JSON válido")
	repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything, mock.Anything)
}

func TestUpdateDocument_NotFound(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("Update", mock.Anything, mock.Anything, "507f1f77bcf86cd799439012", mock.Anything).Return(nil, services.ErrNotFound)

	req := httptest.NewRequest(http.MethodPatch, "/documents/507f1f77bcf86cd799439012",
		strings.NewReader(`{"filename":"x.pdf"}`))
	req.Header.Set("Content-Type", "application/json")

	app := newDocumentsApp(repo)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assertProblemBody(t, string(body), fiber.StatusNotFound, "NOT_FOUND", "not-found", "documento no encontrado")
	repo.AssertExpectations(t)
}

func TestDeleteDocument_NoContent(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("Delete", mock.Anything, mock.Anything, "507f1f77bcf86cd799439011").Return(nil)

	req := httptest.NewRequest(http.MethodDelete, "/documents/507f1f77bcf86cd799439011", nil)
	app := newDocumentsApp(repo)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, fiber.StatusNoContent, resp.StatusCode)
	repo.AssertExpectations(t)
}

func TestDeleteDocument_NotFound(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("Delete", mock.Anything, mock.Anything, "507f1f77bcf86cd799439012").Return(services.ErrNotFound)

	req := httptest.NewRequest(http.MethodDelete, "/documents/507f1f77bcf86cd799439012", nil)
	app := newDocumentsApp(repo)
	resp, err := app.Test(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	assert.Equal(t, fiber.StatusNotFound, resp.StatusCode)
	assertProblemBody(t, string(body), fiber.StatusNotFound, "NOT_FOUND", "not-found", "documento no encontrado")
	repo.AssertExpectations(t)
}

// id con formato distinto de hex-24 -> 400 sin tocar el upstream.
func TestDocuments_InvalidID_Returns400(t *testing.T) {
	for _, path := range []string{
		"/documents/no-es-un-objectid",
		"/documents/507f1f77bcf86cd79943901",   // 23 caracteres
		"/documents/507f1f77bcf86cd7994390111", // 25 caracteres
		"/documents/507F1F77BCF86CD799439011",  // mayúsculas
	} {
		t.Run(path, func(t *testing.T) {
			repo := new(MockPersistenceRepository)

			status, body := getJSON(t, newDocumentsApp(repo), path)

			assert.Equal(t, fiber.StatusBadRequest, status)
			assertProblemBody(t, body, fiber.StatusBadRequest, "INVALID_REQUEST", "invalid-request", "el id debe ser hexadecimal de 24 caracteres")
			repo.AssertNotCalled(t, "FindByID", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}

// un 400 del upstream Persistence se re-emite como 400, nunca 500.
func TestGetDocumentByID_Upstream400_Propagates(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("FindByID", mock.Anything, mock.Anything, "507f1f77bcf86cd799439011").
		Return(nil, &services.UpstreamError{
			Status: fiber.StatusBadRequest,
			Type:   "urn:pdf-extractext:invalid-request",
			Title:  "Petición inválida",
			Code:   "INVALID_REQUEST",
			Detail: "el id debe ser hexadecimal de 24 caracteres",
		})

	status, body := getJSON(t, newDocumentsApp(repo), "/documents/507f1f77bcf86cd799439011")

	assert.Equal(t, fiber.StatusBadRequest, status)
	assertProblemBody(t, body, fiber.StatusBadRequest, "INVALID_REQUEST", "invalid-request", "el id debe ser hexadecimal de 24 caracteres")
	repo.AssertExpectations(t)
}

func TestListDocuments_UpstreamError(t *testing.T) {
	repo := new(MockPersistenceRepository)
	repo.On("FindAll", mock.Anything, mock.Anything).Return(nil, errors.New("persistence upstream: connection refused"))

	status, body := getJSON(t, newDocumentsApp(repo), "/documents")

	assert.Equal(t, fiber.StatusInternalServerError, status)
	assertProblemBody(t, body, fiber.StatusInternalServerError, "INTERNAL_ERROR", "internal-error", "error interno del servidor")
	repo.AssertExpectations(t)
}
