package api

import (
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/core"
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
		return problem(c, fiber.StatusBadRequest, "INVALID_REQUEST", "se requiere el campo multipart 'file'")
	}

	if !isPDF(fileHeader.Filename) {
		return problem(c, fiber.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "solo se aceptan archivos PDF")
	}

	// El tamaño ya lo limita Fiber BodyLimit.
	file, err := fileHeader.Open()
	if err != nil {
		return problem(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "no se pudo abrir el archivo subido")
	}
	defer file.Close()

	// El Request-ID vive en Locals: el middleware lo genera si el cliente no lo envió.
	result, err := h.client.Extract(c.Context(), core.RequestIDFrom(c), fileHeader.Filename, file)
	if err != nil {
		return respondUpstream(c, err, "fallo en el servicio de extracción")
	}

	// Respuesta incompleta del extractor: no es un C1 válido.
	if result.Content == "" || result.PageCount == 0 {
		return problem(c, fiber.StatusBadGateway, "BAD_GATEWAY", "el servicio de extracción devolvió una respuesta incompleta")
	}

	// Contrato público: exactamente {content, page_count}; el endpoint es 100% stateless.
	return c.Status(fiber.StatusOK).JSON(fiber.Map{
		"content":    result.Content,
		"page_count": result.PageCount,
	})
}

func isPDF(filename string) bool {
	return strings.EqualFold(filepath.Ext(filename), ".pdf")
}
