package api

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/core"
	"github.com/pdf-extractext/api/internal/services"
)

// uploadTimeout es el límite estricto end-to-end hacia el extractor.
// Es variable para permitir acortarlo en tests.
var uploadTimeout = 30 * time.Second

type uploadPDFHandler struct {
	extractor services.ExtractionClient
}

// RegisterUploadPDF registra POST /upload-pdf, orquestando la extracción
// con timeout estricto y propagación del X-Request-ID.
func RegisterUploadPDF(app *fiber.App, extractor services.ExtractionClient) {
	h := &uploadPDFHandler{extractor: extractor}
	app.Post("/upload-pdf", h.handle)
}

func (h *uploadPDFHandler) handle(c *fiber.Ctx) error {
	fileHeader, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "se requiere el campo multipart 'file'",
		})
	}

	if !isPDF(fileHeader.Filename) {
		return c.Status(fiber.StatusUnsupportedMediaType).JSON(fiber.Map{
			"error": "solo se aceptan archivos PDF",
		})
	}

	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "no se pudo abrir el archivo subido",
		})
	}
	defer file.Close()

	// X-Request-ID entrante (o el generado por el middleware).
	requestID := c.Get(core.RequestIDHeader)

	// Timeout estricto derivado del request original.
	ctx, cancel := context.WithTimeout(c.Context(), uploadTimeout)
	defer cancel()

	result, err := h.extractor.Extract(ctx, requestID, fileHeader.Filename, file)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || ctx.Err() == context.DeadlineExceeded {
			return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{
				"error":      "el servicio de extracción excedió el tiempo límite",
				"request_id": requestID,
			})
		}
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":      "fallo en el servicio de extracción",
			"request_id": requestID,
		})
	}

	// Validación del contrato C1 del extractor.
	if err := validateExtractionResult(result); err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error":      "respuesta del extractor no cumple el contrato: " + err.Error(),
			"request_id": requestID,
		})
	}

	return c.Status(fiber.StatusOK).JSON(result)
}

// validateExtractionResult verifica el contrato C1: contenido no vacío y
// conteo de páginas positivo.
func validateExtractionResult(r *services.ExtractionResult) error {
	if r == nil {
		return errors.New("respuesta vacía")
	}
	if r.Content == "" {
		return errors.New("content vacío")
	}
	if r.PageCount <= 0 {
		return errors.New("page_count debe ser positivo")
	}
	return nil
}
