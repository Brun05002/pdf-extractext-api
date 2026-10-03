package main

import (
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/tu-usuario/pdf-extractext-api/internal/api"
	"github.com/tu-usuario/pdf-extractext-api/internal/core"
)

func main() {
	cfg, err := core.LoadConfig()
	if err != nil {
		log.Fatalf("config: %v", err)
	}

	app := fiber.New(fiber.Config{
		// Cero volcados a disco: Fiber usa buffers en memoria, límite duro por config.
		BodyLimit: int(cfg.MaxFileSizeMB) * 1024 * 1024,
	})

	app.Use(core.RequestID())
	api.RegisterRoot(app)

	log.Fatal(app.Listen(cfg.Port))
}
