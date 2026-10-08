package api

import (
	"errors"
	"net/http"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/services"
)

// statusCodeStrings mapea códigos HTTP a su nombre estándar de la API.
var statusCodeStrings = map[int]string{
	fiber.StatusBadRequest:            "BAD_REQUEST",
	fiber.StatusNotFound:              "NOT_FOUND",
	fiber.StatusMethodNotAllowed:      "METHOD_NOT_ALLOWED",
	fiber.StatusBadGateway:            "BAD_GATEWAY",
	fiber.StatusServiceUnavailable:    "SERVICE_UNAVAILABLE",
	fiber.StatusGatewayTimeout:        "GATEWAY_TIMEOUT",
	fiber.StatusRequestTimeout:        "GATEWAY_TIMEOUT",
	fiber.StatusUnsupportedMediaType:  "UNSUPPORTED_MEDIA_TYPE",
	fiber.StatusRequestEntityTooLarge: "REQUEST_TOO_LARGE",
}

// CustomErrorHandler traduce cualquier error al JSON estándar
// {"code": "...", "message": "..."} cumpliendo la firma fiber.ErrorHandler.
func CustomErrorHandler(c *fiber.Ctx, err error) error {
	status := fiber.StatusInternalServerError
	code := "INTERNAL_ERROR"
	message := "error interno del servidor"

	switch {
	case errors.Is(err, services.ErrNotFound):
		status = fiber.StatusNotFound
		code = "NOT_FOUND"
		message = err.Error()
	case errors.Is(err, services.ErrConflict):
		status = fiber.StatusConflict
		code = "CONFLICT"
		message = err.Error()
	default:
		var fiberErr *fiber.Error
		if errors.As(err, &fiberErr) {
			status = fiberErr.Code
			if mapped, ok := statusCodeStrings[status]; ok {
				code = mapped
			} else if status < fiber.StatusInternalServerError {
				code = http.StatusText(status)
				if code == "" {
					code = "CLIENT_ERROR"
				}
			} else {
				code = "INTERNAL_ERROR"
			}
			message = fiberErr.Message
		}
	}

	return c.Status(status).JSON(fiber.Map{
		"code":    code,
		"message": message,
	})
}
