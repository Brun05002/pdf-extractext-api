package api

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"syscall"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/pdf-extractext/api/internal/services"
)

// MockExtractionClient simula el microservicio de extracción.
type MockExtractionClient struct {
	mock.Mock
}

func (m *MockExtractionClient) Extract(ctx context.Context, body io.Reader) (*services.ExtractionResult, error) {
	args := m.Called(ctx, body)
	if result, ok := args.Get(0).(*services.ExtractionResult); ok {
		return result, args.Error(1)
	}
	return nil, args.Error(1)
}

func newExtractApp(client services.ExtractionClient, maxSizeMB int64) *fiber.App {
	app := fiber.New(fiber.Config{BodyLimit: int(maxSizeMB) * 1024 * 1024})
	RegisterExtract(app, client, maxSizeMB)
	return app
}

func multipartRequest(t *testing.T, field, filename string, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile(field, filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(fiber.MethodPost, "/extract", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestExtract_OK_StreamsToClient(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, mock.Anything).
		Return(&services.ExtractionResult{Content: "texto extraido", PageCount: 3}, nil)

	app := newExtractApp(client, 5)
	resp, err := app.Test(multipartRequest(t, "file", "documento.pdf", []byte("fake-pdf")))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusOK, resp.StatusCode)
	body, _ := io.ReadAll(resp.Body)
	assert.JSONEq(t, `{"content":"texto extraido","page_count":3}`, string(body))
	client.AssertExpectations(t)
}

func TestExtract_RejectsNonPDF(t *testing.T) {
	client := new(MockExtractionClient)
	app := newExtractApp(client, 5)

	resp, err := app.Test(multipartRequest(t, "file", "imagen.png", []byte("no-pdf")))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusUnsupportedMediaType, resp.StatusCode)
	client.AssertNotCalled(t, "Extract", mock.Anything, mock.Anything)
}

func TestExtract_MissingFile(t *testing.T) {
	client := new(MockExtractionClient)
	app := newExtractApp(client, 5)

	resp, err := app.Test(multipartRequest(t, "otro-campo", "doc.pdf", []byte("x")))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadRequest, resp.StatusCode)
	client.AssertNotCalled(t, "Extract", mock.Anything, mock.Anything)
}

func TestExtract_ExceedsMaxSize(t *testing.T) {
	client := new(MockExtractionClient)
	app := newExtractApp(client, 1) // 1MB

	// app.Test descarta el 413 cuando fasthttp rechaza el body: se usa un listener real.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, app.Shutdown())
	})
	go func() { _ = app.Listener(ln) }()

	big := []byte(strings.Repeat("x", 2*1024*1024)) // 2MB
	req := netHttpRequest(t, big)
	req.URL.Scheme = "http"
	req.URL.Host = ln.Addr().String()
	req.RequestURI = "" // requerido por http.Client

	// fasthttp responde 413 y cierra sin leer los 2MB: se lee la respuesta a nivel TCP crudo.
	conn, err := net.Dial("tcp", ln.Addr().String())
	require.NoError(t, err)
	defer conn.Close()

	writeErr := req.Write(conn)
	resp, readErr := http.ReadResponse(bufio.NewReader(conn), req)
	require.NoError(t, readErr, "el servidor debe responder 413 pese al cierre temprano")
	defer resp.Body.Close()
	if writeErr != nil {
		// Solo se admite EPIPE/ECONNRESET por el cierre temprano del servidor.
		var opErr *net.OpError
		isReset := errors.Is(writeErr, syscall.EPIPE) ||
			errors.Is(writeErr, syscall.ECONNRESET) ||
			(errors.As(writeErr, &opErr) &&
				(errors.Is(opErr.Err, syscall.EPIPE) ||
					errors.Is(opErr.Err, syscall.ECONNRESET))) ||
			strings.Contains(writeErr.Error(), "broken pipe") ||
			strings.Contains(writeErr.Error(), "connection reset by peer")
		require.True(t, isReset,
			"el único error de escritura admisible es el cierre por rechazo del body, se obtuvo: %v", writeErr)
	}

	assert.Equal(t, fiber.StatusRequestEntityTooLarge, resp.StatusCode)
	client.AssertNotCalled(t, "Extract", mock.Anything, mock.Anything)
}

func netHttpRequest(t *testing.T, content []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile("file", "grande.pdf")
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(fiber.MethodPost, "/extract", &buf)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestExtract_UpstreamError(t *testing.T) {
	client := new(MockExtractionClient)
	client.On("Extract", mock.Anything, mock.Anything).
		Return(nil, errors.New("extraction upstream: timeout"))

	app := newExtractApp(client, 5)
	resp, err := app.Test(multipartRequest(t, "file", "doc.pdf", []byte("pdf")))
	require.NoError(t, err)

	assert.Equal(t, fiber.StatusBadGateway, resp.StatusCode)
}
