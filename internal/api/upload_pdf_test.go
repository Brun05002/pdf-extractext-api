package api

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func newUploadApp(client services.ExtractionClient, repo services.PersistenceRepository) *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: CustomErrorHandler})
	RegisterUploadPDF(app, client, repo)
	return app
}

func validC1Result() *services.ExtractionResult {
	return &services.ExtractionResult{
		Filename:      "documento.pdf",
		ExtractedText: "texto",
		Checksum:      "sum-1",
		Content:       "texto",
		PageCount:     2,
	}
}

func uploadRequest(t *testing.T, field, filename string, content []byte, requestID string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile(field, filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(fiber.MethodPost, "/upload-pdf", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	return req
}

// TestUploadPDF_OK: persiste vía Save() con {filename, extracted_text, checksum} y responde 201 (C2).
func TestUploadPDF_OK(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, "req-abc-123", "documento.pdf", mock.Anything).
		Return(validC1Result(), nil)

	repo := new(MockPersistenceRepository)
	repo.On("Save", mock.Anything, mock.Anything, mock.MatchedBy(func(d *services.Document) bool {
		return d.Filename == "documento.pdf" &&
			d.ExtractedText == "texto" &&
			d.Checksum == "sum-1"
	})).Return(&services.Document{
		ID:            "doc-1",
		Filename:      "documento.pdf",
		ExtractedText: "texto",
		Checksum:      "sum-1",
	}, nil)

	app := newUploadApp(client, repo)
	resp, err := app.Test(uploadRequest(t, "file", "documento.pdf", []byte("pdf"), "req-abc-123"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.JSONEq(t, `{"id":"doc-1","filename":"documento.pdf","extracted_text":"texto","checksum":"sum-1"}`, string(body))
	client.AssertExpectations(t)
	repo.AssertExpectations(t)
}

// TestUploadPDF_Conflict: el 409 anti-TOCTOU de Persistence se propaga como 409 global, nunca 500.
func TestUploadPDF_Conflict(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(validC1Result(), nil)

	repo := new(MockPersistenceRepository)
	repo.On("Save", mock.Anything, mock.Anything, mock.Anything).Return(nil, services.ErrConflict)

	app := newUploadApp(client, repo)
	resp, err := app.Test(uploadRequest(t, "file", "documento.pdf", []byte("pdf"), "req-dup"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusConflict, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assertProblemBody(t, string(body), fiber.StatusConflict, "CONFLICT", "conflict", "el documento ya existe")
	client.AssertExpectations(t)
	repo.AssertExpectations(t)
}

func TestUploadPDF_PropagatesRequestID(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, "mi-request-id", mock.Anything, mock.Anything).
		Return(validC1Result(), nil)

	repo := new(MockPersistenceRepository)
	repo.On("Save", mock.Anything, mock.Anything, mock.Anything).
		Return(&services.Document{ID: "doc-1", Filename: "a.pdf", ExtractedText: "texto", Checksum: "sum-1"}, nil)

	app := newUploadApp(client, repo)
	resp, err := app.Test(uploadRequest(t, "file", "a.pdf", []byte("pdf"), "mi-request-id"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
	client.AssertExpectations(t)
	client.AssertCalled(t, "Extract", mock.Anything, "mi-request-id", "a.pdf", mock.Anything)
}

func TestUploadPDF_Timeout(t *testing.T) {
	// Acorta el timeout del handler para no esperar 30s en el test.
	orig := uploadTimeout
	uploadTimeout = 50 * time.Millisecond
	t.Cleanup(func() { uploadTimeout = orig })

	client := new(MockExtractionClient)
	// El mock simula un upstream lento que nunca responde a tiempo: devuelve
	// el error del contexto, igual que haría http.Client al expirar el ctx.
	client.On("Extract", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			ctx := args.Get(0).(context.Context)
			<-ctx.Done() // bloquea hasta que el timeout del handler cancele
		}).
		Return(nil, context.DeadlineExceeded)

	repo := new(MockPersistenceRepository)
	app := newUploadApp(client, repo)

	start := time.Now()
	resp, err := app.Test(uploadRequest(t, "file", "lento.pdf", []byte("pdf"), "req-timeout"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusGatewayTimeout, resp.StatusCode)
	assert.Less(t, time.Since(start), 5*time.Second, "el handler debe cortar por su propio timeout")

	body, _ := io.ReadAll(resp.Body)
	assertProblemBody(t, string(body), fiber.StatusGatewayTimeout, "GATEWAY_TIMEOUT", "gateway-timeout", "el servicio de extracción excedió el tiempo límite")
	client.AssertExpectations(t)
	repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
}

func TestUploadPDF_MissingFile(t *testing.T) {
	client := new(MockExtractionClient)
	repo := new(MockPersistenceRepository)
	app := newUploadApp(client, repo)

	resp, err := app.Test(uploadRequest(t, "campo-mal", "doc.pdf", []byte("x"), "req-1"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "file")
	client.AssertNotCalled(t, "Extract", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
}

func TestUploadPDF_UpstreamError(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("extraction upstream: connection refused"))

	repo := new(MockPersistenceRepository)
	app := newUploadApp(client, repo)
	resp, err := app.Test(uploadRequest(t, "file", "doc.pdf", []byte("pdf"), "req-err"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadGateway, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assertProblemBody(t, string(body), fiber.StatusBadGateway, "BAD_GATEWAY", "bad-gateway", "fallo en el servicio de extracción")
	repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything, mock.Anything)
}

func TestUploadPDF_InvalidContract(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&services.ExtractionResult{Content: "", PageCount: 0}, nil)

	repo := new(MockPersistenceRepository)
	app := newUploadApp(client, repo)
	resp, err := app.Test(uploadRequest(t, "file", "doc.pdf", []byte("pdf"), "req-contrato"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadGateway, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "contrato")
	repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything)
}

func TestUploadPDF_PersistenceError(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(validC1Result(), nil)

	repo := new(MockPersistenceRepository)
	repo.On("Save", mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("persistence upstream: connection refused"))

	app := newUploadApp(client, repo)
	resp, err := app.Test(uploadRequest(t, "file", "doc.pdf", []byte("pdf"), "req-persist"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadGateway, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assertProblemBody(t, string(body), fiber.StatusBadGateway, "BAD_GATEWAY", "bad-gateway", "fallo en el servicio de persistencia")
}

// TestUploadPDF_RequestIDHeaderThroughHTTPClient verifica de punta a punta que
// el X-Request-ID llega realmente al upstream vía el cliente HTTP real.
func TestUploadPDF_RequestIDHeaderThroughHTTPClient(t *testing.T) {
	var gotRequestID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get("X-Request-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"filename":"e2e.pdf","extracted_text":"ok","checksum":"sum-e2e","content":"ok","page_count":1}`))
	}))
	defer upstream.Close()

	client := services.NewHTTPExtractionClient(upstream.URL, 5*time.Second)
	repo := new(MockPersistenceRepository)
	repo.On("Save", mock.Anything, mock.Anything, mock.MatchedBy(func(d *services.Document) bool {
		return d.Filename == "e2e.pdf" && d.ExtractedText == "ok" && d.Checksum == "sum-e2e"
	})).Return(&services.Document{ID: "doc-e2e", Filename: "e2e.pdf", ExtractedText: "ok", Checksum: "sum-e2e"}, nil)

	app := newUploadApp(client, repo)

	resp, err := app.Test(uploadRequest(t, "file", "e2e.pdf", []byte("pdf"), "req-e2e-999"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusCreated, resp.StatusCode)
	assert.Equal(t, "req-e2e-999", gotRequestID, "el X-Request-ID debe llegar al upstream")
	repo.AssertExpectations(t)
}

// TestUploadPDF_UpstreamPropagation_E2E verifica FIND-012 con el cliente HTTP
// real: el 503 (con Retry-After) y el 422 del extractor se propagan tal cual;
// un 500 del extractor se responde como 502 genérico sin filtrar detalle.
func TestUploadPDF_UpstreamPropagation_E2E(t *testing.T) {
	cases := []struct {
		name          string
		upstream      func(w http.ResponseWriter)
		wantStatus    int
		wantCode      string
		wantURN       string
		wantDetail    string
		wantRetryAfter string
	}{
		{
			name: "503 con Retry-After se propaga con el header",
			upstream: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.Header().Set("Retry-After", "7")
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"type":"urn:pdf-extractext:service-overloaded","title":"Servicio saturado","status":503,"detail":"Servicio saturado. Reintente más tarde.","code":"SERVICE_OVERLOADED"}`))
			},
			wantStatus:     fiber.StatusServiceUnavailable,
			wantCode:       "SERVICE_OVERLOADED",
			wantURN:        "service-overloaded",
			wantDetail:     "Servicio saturado. Reintente más tarde.",
			wantRetryAfter: "7",
		},
		{
			name: "422 INVALID_PDF_CONTENT se propaga tal cual",
			upstream: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"type":"urn:pdf-extractext:invalid-pdf-content","title":"Contenido PDF inválido","status":422,"detail":"El contenido recibido no es un PDF válido","code":"INVALID_PDF_CONTENT"}`))
			},
			wantStatus: fiber.StatusUnprocessableEntity,
			wantCode:   "INVALID_PDF_CONTENT",
			wantURN:    "invalid-pdf-content",
			wantDetail: "El contenido recibido no es un PDF válido",
		},
		{
			name: "500 del extractor se responde 502 sin detalle interno",
			upstream: func(w http.ResponseWriter) {
				w.Header().Set("Content-Type", "application/problem+json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"type":"urn:pdf-extractext:internal-error","title":"Error interno del servidor","status":500,"detail":"Fallo inesperado procesando el PDF","code":"INTERNAL_ERROR"}`))
			},
			wantStatus: fiber.StatusBadGateway,
			wantCode:   "BAD_GATEWAY",
			wantURN:    "bad-gateway",
			wantDetail: "fallo en el servicio de extracción",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				tc.upstream(w)
			}))
			defer upstream.Close()

			client := services.NewHTTPExtractionClient(upstream.URL, 5*time.Second)
			repo := new(MockPersistenceRepository)
			app := newUploadApp(client, repo)

			resp, err := app.Test(uploadRequest(t, "file", "doc.pdf", []byte("pdf"), "req-prop"))
			require.NoError(t, err)
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			assertProblemContentType(t, resp)
			assertProblemBody(t, string(body), tc.wantStatus, tc.wantCode, tc.wantURN, tc.wantDetail)
			if tc.wantRetryAfter != "" {
				assert.Equal(t, tc.wantRetryAfter, resp.Header.Get("Retry-After"), "el Retry-After upstream debe conservarse")
			} else {
				assert.Empty(t, resp.Header.Get("Retry-After"))
			}
			repo.AssertNotCalled(t, "Save", mock.Anything, mock.Anything, mock.Anything)
		})
	}
}
