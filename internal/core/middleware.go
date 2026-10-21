package core

import (
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
