package api

import (
	"log/slog"
	"time"

	"github.com/gofiber/fiber/v2"
)

// RegisterRoot expone GET / como endpoint de health público. Lo consumes el
// healthprobe (Docker) y los balanceadores.
func RegisterRoot(app *fiber.App) {
	app.Get("/", func(c *fiber.Ctx) error {
		slog.Info("healthcheck solicitado",
			"ip", c.IP(),
			"request_id", c.Get("X-Request-ID"),
		)
		return c.JSON(fiber.Map{
			"service": "pdf-extractext-api",
			"status":  "UP",
			"time":    time.Now().UTC().Format(time.RFC3339),
		})
	})
}
