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

func newUploadApp(client services.ExtractionClient) *fiber.App {
	app := fiber.New()
	RegisterUploadPDF(app, client)
	return app
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

func TestUploadPDF_OK(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, "req-abc-123", "documento.pdf", mock.Anything).
		Return(&services.ExtractionResult{Content: "texto", PageCount: 2}, nil)

	app := newUploadApp(client)
	resp, err := app.Test(uploadRequest(t, "file", "documento.pdf", []byte("pdf"), "req-abc-123"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.JSONEq(t, `{"content":"texto","page_count":2}`, string(body))
	client.AssertExpectations(t)
}

func TestUploadPDF_PropagatesRequestID(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, "mi-request-id", mock.Anything, mock.Anything).
		Return(&services.ExtractionResult{Content: "x", PageCount: 1}, nil)

	app := newUploadApp(client)
	resp, err := app.Test(uploadRequest(t, "file", "a.pdf", []byte("pdf"), "mi-request-id"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
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

	app := newUploadApp(client)

	start := time.Now()
	resp, err := app.Test(uploadRequest(t, "file", "lento.pdf", []byte("pdf"), "req-timeout"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusGatewayTimeout, resp.StatusCode)
	assert.Less(t, time.Since(start), 5*time.Second, "el handler debe cortar por su propio timeout")

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "req-timeout")
	client.AssertExpectations(t)
}

func TestUploadPDF_MissingFile(t *testing.T) {
	client := new(MockExtractionClient)
	app := newUploadApp(client)

	resp, err := app.Test(uploadRequest(t, "campo-mal", "doc.pdf", []byte("x"), "req-1"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "file")
	client.AssertNotCalled(t, "Extract", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

func TestUploadPDF_UpstreamError(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(nil, errors.New("extraction upstream: connection refused"))

	app := newUploadApp(client)
	resp, err := app.Test(uploadRequest(t, "file", "doc.pdf", []byte("pdf"), "req-err"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadGateway, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "req-err")
}

func TestUploadPDF_InvalidContract(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(&services.ExtractionResult{Content: "", PageCount: 0}, nil)

	app := newUploadApp(client)
	resp, err := app.Test(uploadRequest(t, "file", "doc.pdf", []byte("pdf"), "req-contrato"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadGateway, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "contrato")
}

// TestUploadPDF_RequestIDHeaderThroughHTTPClient verifica de punta a punta que
// el X-Request-ID llega realmente al upstream vía el cliente HTTP real.
func TestUploadPDF_RequestIDHeaderThroughHTTPClient(t *testing.T) {
	var gotRequestID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRequestID = r.Header.Get("X-Request-ID")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"content":"ok","page_count":1}`))
	}))
	defer upstream.Close()

	client := services.NewHTTPExtractionClient(upstream.URL, 5*time.Second)
	app := newUploadApp(client)

	resp, err := app.Test(uploadRequest(t, "file", "e2e.pdf", []byte("pdf"), "req-e2e-999"))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Equal(t, "req-e2e-999", gotRequestID, "el X-Request-ID debe llegar al upstream")
}
