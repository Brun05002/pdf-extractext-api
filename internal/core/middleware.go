package core

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
)

const RequestIDHeader = "X-Request-ID"

func RequestID() fiber.Handler {
	return func(c *fiber.Ctx) error {
		rid := c.Get(RequestIDHeader)
		if rid == "" {
			rid = uuid.NewString()
		}
		c.Locals("requestID", rid)
		c.Set(RequestIDHeader, rid)
		return c.Next()
	}
}

// RequestIDFrom devuelve el X-Request-ID del contexto (entrante o generado por RequestID).
func RequestIDFrom(c *fiber.Ctx) string {
	rid, _ := c.Locals("requestID").(string)
	return rid
}

// Logger registra method, path, status, duración y X-Request-ID de cada request.
// statusFor permite resolver el status real antes de que Fiber ejecute el
// ErrorHandler global cuando el handler devuelve un error.
func Logger(statusFor ...func(error) int) fiber.Handler {
	var resolver func(error) int
	if len(statusFor) > 0 {
		resolver = statusFor[0]
	}

	return func(c *fiber.Ctx) error {
		start := time.Now()
		err := c.Next()
		// GET / ya lo registra RegisterRoot; se omite para no duplicar.
		if c.Method() == fiber.MethodGet && c.Path() == "/" {
			return err
		}

		status := c.Response().StatusCode()
		if err != nil {
			status = fiber.StatusInternalServerError
			if resolver != nil {
				status = resolver(err)
			}
		}

		slog.Info("request",
			"method", c.Method(),
			"path", c.Path(),
			"status", status,
			"duration_ms", time.Since(start).Milliseconds(),
			"request_id", RequestIDFrom(c),
		)
		return err
	}
}
