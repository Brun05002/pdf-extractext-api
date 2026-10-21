package core

import (
	"bytes"
	"io"
	"log/slog"
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

// Sin el middleware, RequestIDFrom devuelve vacío (no pánico).
func TestRequestIDFrom_NoMiddleware_ReturnsEmpty(t *testing.T) {
	app := fiber.New()
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString(RequestIDFrom(c))
	})

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/", nil))
	require.NoError(t, err)
	assert.Empty(t, stringBody(t, resp))
}

// RequestIDFrom refleja lo que el middleware guardó en Locals.
func TestRequestIDFrom_ReadsLocals(t *testing.T) {
	app := fiber.New()
	app.Use(RequestID())
	app.Get("/", func(c *fiber.Ctx) error {
		return c.SendString(RequestIDFrom(c))
	})

	req := httptest.NewRequest(fiber.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "rid-77")

	resp, err := app.Test(req)
	require.NoError(t, err)
	assert.Equal(t, "rid-77", stringBody(t, resp))
}

// captureLogs redirige el logger default durante el test.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestLogger_RegistraRequestConRequestID(t *testing.T) {
	buf := captureLogs(t)

	app := fiber.New()
	app.Use(RequestID())
	app.Use(Logger())
	app.Get("/hola", func(c *fiber.Ctx) error { return c.SendString("ok") })

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/hola", nil))
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, resp.StatusCode)

	log := buf.String()
	assert.Contains(t, log, `"method":"GET"`)
	assert.Contains(t, log, `"path":"/hola"`)
	assert.Contains(t, log, `"status":200`)
	assert.Contains(t, log, resp.Header.Get("X-Request-ID"), "debe incluir el X-Request-ID")
}

func TestLogger_OmiteHealthcheck(t *testing.T) {
	buf := captureLogs(t)

	app := fiber.New()
	app.Use(RequestID())
	app.Use(Logger())
	app.Get("/", func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })

	resp, err := app.Test(httptest.NewRequest(fiber.MethodGet, "/", nil))
	require.NoError(t, err)
	require.Equal(t, fiber.StatusOK, resp.StatusCode)
	assert.Empty(t, buf.String(), "GET / no debe registrarse en el access-log")
}
