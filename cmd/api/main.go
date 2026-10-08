package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/api"
	"github.com/pdf-extractext/api/internal/core"
	"github.com/pdf-extractext/api/internal/services"
)

func main() {
	// Logging estructurado JSON por defecto para todo el proceso.
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	cfg, err := core.LoadConfig()
	if err != nil {
		slog.Error("configuración inválida", "error", err)
		os.Exit(1)
	}

	app := fiber.New(fiber.Config{
		// Límite duro en memoria; nunca se vuelca a disco.
		BodyLimit: int(cfg.MaxFileSizeMB) * 1024 * 1024,
		// Traducción global de errores al formato estándar {"code","message"}.
		ErrorHandler: api.CustomErrorHandler,
	})

	app.Use(core.RequestID())
	api.RegisterRoot(app)

	// Timeout estricto: backpressure contra un upstream lento.
	extraction := services.NewHTTPExtractionClient(cfg.ExtractionURL, 30*time.Second)
	api.RegisterExtract(app, extraction, cfg.MaxFileSizeMB)
	api.RegisterUploadPDF(app, extraction)

	persistence := services.NewHTTPPersistenceClient(cfg.PersistenceURL, 10*time.Second)
	api.RegisterDocuments(app, persistence)

	slog.Info("iniciando servidor", "puerto", cfg.Port)
	if err := app.Listen(cfg.Port); err != nil {
		slog.Error("el servidor terminó con error", "error", err)
		os.Exit(1)
	}
}
