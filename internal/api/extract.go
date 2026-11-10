package api

import (
	"path/filepath"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/core"
	"github.com/pdf-extractext/api/internal/services"
)

type extractHandler struct {
	client   services.ExtractionClient
	maxBytes int64
}

// RegisterExtract registra POST /extract inyectando el cliente de extracción.
func RegisterExtract(app *fiber.App, client services.ExtractionClient, maxSizeMB int64) {
	h := &extractHandler{
		client:   client,
		maxBytes: maxSizeMB * 1024 * 1024,
	}
	app.Post("/extract", h.handle)
}

func (h *extractHandler) handle(c *fiber.Ctx) error {
	input, err := openPDFInput(c, h.maxBytes)
	if err != nil {
		return err
	}
	defer input.Close()

	// El Request-ID vive en Locals: el middleware lo genera si el cliente no lo envió.
	result, err := h.client.Extract(c.Context(), core.RequestIDFrom(c), input.Filename, input)
	if err != nil {
		if input.LimitExceeded() {
			return requestTooLargeError()
		}
		return respondUpstream(c, err, "fallo en el servicio de extracción")
	}
	if input.LimitExceeded() {
		return requestTooLargeError()
	}

	// Respuesta incompleta del extractor: no es un C1 válido.
	if err := validateExtractionResult(result); err != nil {
		return problem(c, fiber.StatusBadGateway, "BAD_GATEWAY", "respuesta del extractor no cumple el contrato: "+err.Error())
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
