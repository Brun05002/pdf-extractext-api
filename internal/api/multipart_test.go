package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/stretchr/testify/require"
)

func multipartRequestFor(t *testing.T, path, field, filename string, content []byte, requestID string) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	part, err := writer.CreateFormFile(field, filename)
	require.NoError(t, err)
	_, err = part.Write(content)
	require.NoError(t, err)
	require.NoError(t, writer.Close())

	req := httptest.NewRequest(fiber.MethodPost, path, &buf)
	req.Header.Set(fiber.HeaderContentType, writer.FormDataContentType())
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	return req
}
