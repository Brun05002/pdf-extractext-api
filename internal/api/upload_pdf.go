package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/core"
	"github.com/pdf-extractext/api/internal/services"
)

// uploadTimeout acota la SAGA (lookup, extracción y persistencia).
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

	spooled, checksum, err := spoolPDF(input)
	if err != nil {
		if errors.Is(err, errBodyTooLarge) || input.LimitExceeded() {
			return requestTooLargeError()
		}
		return problem(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "no se pudo preparar el archivo")
	}
	defer spooled.Cleanup()

	// Timeout estricto derivado del request original.
	ctx, cancel := context.WithTimeout(c.Context(), uploadTimeout)
	defer cancel()

	existing, err := h.repo.FindByChecksum(ctx, requestID, checksum)
	if err == nil && existing != nil {
		// El error handler global lo traduce a 409 CONFLICT.
		return services.ErrConflict
	}
	if err != nil && !errors.Is(err, services.ErrNotFound) {
		return respondUpstream(c, err, "fallo en el servicio de persistencia")
	}

	result, err := h.extractor.Extract(ctx, requestID, input.Filename, spooled)
	if err != nil {
		if isTimeout(err) || ctx.Err() != nil {
			return problem(c, fiber.StatusGatewayTimeout, "GATEWAY_TIMEOUT", "el servicio de extracción excedió el tiempo límite")
		}
		return respondUpstream(c, err, "fallo en el servicio de extracción")
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

type spooledPDF struct {
	*os.File
}

func (s *spooledPDF) Cleanup() {
	name := s.Name()
	_ = s.Close()
	_ = os.Remove(name)
}

func spoolPDF(input *pdfInput) (*spooledPDF, string, error) {
	file, err := os.CreateTemp("", "pdf-extractext-upload-*.pdf")
	if err != nil {
		return nil, "", fmt.Errorf("crear archivo temporal: %w", err)
	}
	cleanup := true
	defer func() {
		if cleanup {
			name := file.Name()
			_ = file.Close()
			_ = os.Remove(name)
		}
	}()

	hasher := sha256.New()
	if _, err := io.Copy(io.MultiWriter(file, hasher), input); err != nil {
		if input.LimitExceeded() {
			return nil, "", errBodyTooLarge
		}
		return nil, "", fmt.Errorf("copiar PDF a temporal: %w", err)
	}
	if input.LimitExceeded() {
		return nil, "", errBodyTooLarge
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, "", fmt.Errorf("rebobinar temporal: %w", err)
	}

	cleanup = false
	return &spooledPDF{File: file}, hex.EncodeToString(hasher.Sum(nil)), nil
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
