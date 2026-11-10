package services

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExtractionClient_StreamsBodyAndParsesResponse(t *testing.T) {
	payload := strings.Repeat("fake-pdf-bytes", 1000)

	server := httptest.NewServer(httpServerHandler(t, payload))
	defer server.Close()

	client := NewHTTPExtractionClient(server.URL, "extraction.test", 3*time.Second)

	result, err := client.Extract(t.Context(), "test-rid", "informe.pdf", strings.NewReader(payload))
	require.NoError(t, err)
	assert.Equal(t, "texto extraido", result.Content)
	assert.Equal(t, 3, result.PageCount)
	assert.Equal(t, "informe.pdf", result.Filename)
	assert.Equal(t, "texto extraido", result.ExtractedText)
	assert.Equal(t, "abc123", result.Checksum)
}

func httpServerHandler(t *testing.T, expected string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// multipart/form-data con campo "file".
		require.Contains(t, r.Header.Get("Content-Type"), "multipart/form-data")
		file, header, err := r.FormFile("file")
		require.NoError(t, err)
		defer file.Close()
		require.Equal(t, "informe.pdf", header.Filename)

		body, err := io.ReadAll(file)
		require.NoError(t, err)
		require.Equal(t, expected, string(body), "el stream debe llegar íntegro y sin buffers fijos")

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"filename":"informe.pdf","extracted_text":"texto extraido","checksum":"abc123","content":"texto extraido","page_count":3}`))
	}
}

func TestExtractionClient_SetsConfiguredHost(t *testing.T) {
	var gotHost string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		_, _ = w.Write([]byte(`{"content":"texto","page_count":1}`))
	}))
	defer server.Close()

	client := NewHTTPExtractionClient(server.URL, "extraction.custom.host", 3*time.Second)
	_, err := client.Extract(t.Context(), "rid", "doc.pdf", strings.NewReader("pdf"))
	require.NoError(t, err)
	assert.Equal(t, "extraction.custom.host", gotHost)
}

func TestExtractionClient_UpstreamError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewHTTPExtractionClient(server.URL, "extraction.test", 3*time.Second)

	_, err := client.Extract(t.Context(), "test-rid", "doc.pdf", strings.NewReader("pdf"))
	var ue *UpstreamError
	require.ErrorAs(t, err, &ue, "el error upstream debe preservarse como UpstreamError")
	assert.Equal(t, http.StatusInternalServerError, ue.Status)
}

func TestExtractionClient_ClosesPipeOnRequestConstructionError(t *testing.T) {
	client := NewHTTPExtractionClient("://invalid-url", "extraction.test", time.Second)

	_, err := client.Extract(t.Context(), "test-rid", "doc.pdf", strings.NewReader("pdf"))
	require.Error(t, err)

	assertNoExtractProducerGoroutine(t)
}

func TestExtractionClient_ClosesPipeOnCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"content":"x","page_count":1}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewHTTPExtractionClient(server.URL, "extraction.test", time.Second)
	_, err := client.Extract(ctx, "test-rid", "doc.pdf", strings.NewReader(strings.Repeat("pdf", 1<<16)))
	require.Error(t, err)

	assertNoExtractProducerGoroutine(t)
}

// assertNoExtractProducerGoroutine detecta fugas de la goroutine que escribe en io.Pipe.
func assertNoExtractProducerGoroutine(t *testing.T) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		buf := make([]byte, 1<<20)
		n := runtime.Stack(buf, true)
		if !bytes.Contains(buf[:n], []byte("HTTPExtractionClient).Extract.func1")) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("la goroutine productora del multipart quedó bloqueada")
}

func TestExtractionClient_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond) // upstream lento
		_, _ = w.Write([]byte(`{"content":"x","page_count":1}`))
	}))
	defer server.Close()

	client := NewHTTPExtractionClient(server.URL, "extraction.test", 50*time.Millisecond)

	_, err := client.Extract(t.Context(), "test-rid", "doc.pdf", strings.NewReader("pdf"))
	require.Error(t, err, "el timeout estricto debe cortar goroutines colgadas")

	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	assert.True(t, netErr.Timeout(), "el error debe ser reconocible como timeout para mapear a 504")
}
