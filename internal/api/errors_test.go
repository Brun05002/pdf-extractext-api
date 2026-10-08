package api

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/services"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newErrorApp() *fiber.App {
	app := fiber.New(fiber.Config{ErrorHandler: CustomErrorHandler})

	app.Get("/not-found", func(c *fiber.Ctx) error {
		return services.ErrNotFound
	})
	app.Get("/conflict", func(c *fiber.Ctx) error {
		return services.ErrConflict
	})
	app.Get("/fiber-error", func(c *fiber.Ctx) error {
		return fiber.NewError(fiber.StatusBadRequest, "el parámetro 'query' es obligatorio")
	})
	app.Get("/generic", func(c *fiber.Ctx) error {
		return errors.New("panic silencioso del subsistema X")
	})
	return app
}

func testGet(t *testing.T, app *fiber.App, path string) (int, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest(http.MethodGet, path, nil))
	require.NoError(t, err)
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, string(body)
}

func TestErrorHandler_NotFound(t *testing.T) {
	status, body := testGet(t, newErrorApp(), "/not-found")
	assert.Equal(t, fiber.StatusNotFound, status)
	assert.JSONEq(t, `{"code":"NOT_FOUND","message":"documento no encontrado"}`, body)
}

func TestErrorHandler_Conflict(t *testing.T) {
	status, body := testGet(t, newErrorApp(), "/conflict")
	assert.Equal(t, fiber.StatusConflict, status)
	assert.JSONEq(t, `{"code":"CONFLICT","message":"el documento ya existe"}`, body)
}

func TestErrorHandler_FiberError(t *testing.T) {
	status, body := testGet(t, newErrorApp(), "/fiber-error")
	assert.Equal(t, fiber.StatusBadRequest, status)
	assert.JSONEq(t, `{"code":"BAD_REQUEST","message":"el parámetro 'query' es obligatorio"}`, body)
}

func TestErrorHandler_GenericError(t *testing.T) {
	status, body := testGet(t, newErrorApp(), "/generic")
	assert.Equal(t, fiber.StatusInternalServerError, status)
	// El mensaje interno nunca debe filtrarse al cliente.
	assert.JSONEq(t, `{"code":"INTERNAL_ERROR","message":"error interno del servidor"}`, body)
}

func TestErrorHandler_RouteNotFound(t *testing.T) {
	// El 404 por defecto del framework (ruta inexistente) también se traduce.
	status, body := testGet(t, newErrorApp(), "/ruta/inexistente")
	assert.Equal(t, fiber.StatusNotFound, status)
	assert.JSONEq(t, `{"code":"NOT_FOUND","message":"Cannot GET /ruta/inexistente"}`, body)
}
