package services

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
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
		w.Write([]byte(`{"filename":"informe.pdf","extracted_text":"texto extraido","checksum":"abc123","content":"texto extraido","page_count":3}`))
	}
}

func TestExtractionClient_SetsConfiguredHost(t *testing.T) {
	var gotHost string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.Write([]byte(`{"content":"texto","page_count":1}`))
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

func TestExtractionClient_Timeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond) // upstream lento
		w.Write([]byte(`{"content":"x","page_count":1}`))
	}))
	defer server.Close()

	client := NewHTTPExtractionClient(server.URL, "extraction.test", 50*time.Millisecond)

	_, err := client.Extract(t.Context(), "test-rid", "doc.pdf", strings.NewReader("pdf"))
	require.Error(t, err, "el timeout estricto debe cortar goroutines colgadas")

	var netErr net.Error
	require.ErrorAs(t, err, &netErr)
	assert.True(t, netErr.Timeout(), "el error debe ser reconocible como timeout para mapear a 504")
}
