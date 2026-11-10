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
	repo      services.PersistenceRepository
	maxBytes  int64
}

// RegisterUploadPDF registra POST /upload-pdf: extrae (C1) y persiste (C2) con X-Request-ID y timeout estricto.
func RegisterUploadPDF(app *fiber.App, extractor services.ExtractionClient, repo services.PersistenceRepository, maxSizeMB int64) {
	h := &uploadPDFHandler{
		extractor: extractor,
		repo:      repo,
		maxBytes:  maxSizeMB * 1024 * 1024,
	}
	app.Post("/upload-pdf", h.handle)
}

func (h *uploadPDFHandler) handle(c *fiber.Ctx) error {
	input, err := openPDFInput(c, h.maxBytes)
	if err != nil {
		return err
	}
	defer input.Close()

	// El Request-ID vive en Locals: el middleware lo genera si el cliente no lo envió.
	requestID := core.RequestIDFrom(c)

	// Timeout estricto derivado del request original.
	ctx, cancel := context.WithTimeout(c.Context(), uploadTimeout)
	defer cancel()

	result, err := h.extractor.Extract(ctx, requestID, input.Filename, input)
	if err != nil {
		if input.LimitExceeded() {
			return requestTooLargeError()
		}
		if isTimeout(err) || ctx.Err() != nil {
			return problem(c, fiber.StatusGatewayTimeout, "GATEWAY_TIMEOUT", "el servicio de extracción excedió el tiempo límite")
		}
		return respondUpstream(c, err, "fallo en el servicio de extracción")
	}
	if input.LimitExceeded() {
		return requestTooLargeError()
	}

	// Validación del contrato C1.
	if err = validateExtractionResult(result); err != nil {
		return problem(c, fiber.StatusBadGateway, "BAD_GATEWAY", "respuesta del extractor no cumple el contrato: "+err.Error())
	}

	// El índice único de checksum es la barrera anti-TOCTOU: un duplicado llega como 409.
	saved, err := h.repo.Save(ctx, requestID, &services.Document{
		Filename:      result.Filename,
		ExtractedText: result.ExtractedText,
		Checksum:      result.Checksum,
	})
	if err != nil {
		if errors.Is(err, services.ErrConflict) {
			// El error handler global lo traduce a 409 CONFLICT.
			return err
		}
		return respondUpstream(c, err, "fallo en el servicio de persistencia")
	}

	return c.Status(fiber.StatusCreated).JSON(saved)
}

// validateExtractionResult verifica el contrato C1: content no vacío y page_count positivo.
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
