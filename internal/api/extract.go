package api

import (
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/services"
)

type extractHandler struct {
	client services.ExtractionClient
}

// RegisterExtract registra POST /extract inyectando el cliente de extracción.
func RegisterExtract(app *fiber.App, client services.ExtractionClient, maxSizeMB int64) {
	h := &extractHandler{client: client}
	app.Post("/extract", h.handle)
}

func (h *extractHandler) handle(c *fiber.Ctx) error {
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

	// El tamaño ya lo limita Fiber BodyLimit.
	file, err := fileHeader.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "no se pudo abrir el archivo subido",
		})
	}
	defer file.Close()

	// multipart.File es io.Reader: stream directo al upstream.
	result, err := h.client.Extract(c.Context(), fileHeader.Filename, file)
	if err != nil {
		return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
			"error": "fallo en el servicio de extracción",
		})
	}

	return c.Status(fiber.StatusOK).JSON(result)
}

func isPDF(filename string) bool {
	return strings.EqualFold(filepath.Ext(filename), ".pdf")
}
