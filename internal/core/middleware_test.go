package core

// FASE RED: tests del middleware de Request-ID antes de implementarlo.

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestApp() *fiber.App {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})
	return app
}

func stringBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return string(body)
}

func TestRequestID_GeneratesWhenMissing(t *testing.T) {
	app := newTestApp()
	req := httptest.NewRequest(fiber.MethodGet, "/", nil)

	resp, err := app.Test(req)
	require.NoError(t, err)

	rid := resp.Header.Get("X-Request-ID")
	assert.NotEmpty(t, rid, "debe generar un X-Request-ID")
	_, err = uuid.Parse(rid)
	assert.NoError(t, err, "el Request-ID generado debe ser un UUID válido")
}

func TestRequestID_PropagatesExisting(t *testing.T) {
	app := newTestApp()
	const incoming = "req-abc-123"
	req := httptest.NewRequest(fiber.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", incoming)

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, incoming, resp.Header.Get("X-Request-ID"), "debe propagar el Request-ID entrante")
}

func TestRequestID_AvailableInLocals(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/whoami", func(c *fiber.Ctx) error {
		return c.SendString(c.Locals("requestID").(string))
	})

	req := httptest.NewRequest(fiber.MethodGet, "/whoami", nil)
	req.Header.Set("X-Request-ID", "rid-42")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, "rid-42", stringBody(t, resp))
}
