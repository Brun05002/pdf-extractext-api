package api

import (
	"regexp"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/core"
	"github.com/pdf-extractext/api/internal/services"
)

// objectIDPattern valida el id hex-24 de Mongo antes de llamar al upstream.
var objectIDPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)

type documentsHandler struct {
	repo services.PersistenceRepository
}

// RegisterDocuments registra los endpoints públicos de lectura de documentos.
func RegisterDocuments(app *fiber.App, repo services.PersistenceRepository) {
	h := &documentsHandler{repo: repo}
	app.Get("/documents", h.list)
	app.Get("/documents/:id", h.getByID)
	app.Patch("/documents/:id", h.update)
	app.Delete("/documents/:id", h.delete)
}

// validID usa fiber.NewError porque el ErrorHandler global mapea 400 a problem+json.
func validID(c *fiber.Ctx) (string, error) {
	id := c.Params("id")
	if !objectIDPattern.MatchString(id) {
		return "", fiber.NewError(fiber.StatusBadRequest, "el id debe ser hexadecimal de 24 caracteres")
	}
	return id, nil
}

func (h *documentsHandler) update(c *fiber.Ctx) error {
	id, err := validID(c)
	if err != nil {
		return err
	}

	var doc services.Document
	if err := c.BodyParser(&doc); err != nil {
		return problem(c, fiber.StatusBadRequest, "INVALID_REQUEST", "el cuerpo de la petición no es un JSON válido")
	}

	updated, err := h.repo.Update(c.Context(), core.RequestIDFrom(c), id, &doc)
	if err != nil {
		return err // ErrNotFound -> 404, ErrConflict -> 409, UpstreamError -> tal cual
	}
	return c.Status(fiber.StatusOK).JSON(updated)
}

func (h *documentsHandler) delete(c *fiber.Ctx) error {
	id, err := validID(c)
	if err != nil {
		return err
	}
	if err := h.repo.Delete(c.Context(), core.RequestIDFrom(c), id); err != nil {
		return err // ErrNotFound -> 404
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *documentsHandler) list(c *fiber.Ctx) error {
	docs, err := h.repo.FindAll(c.Context(), core.RequestIDFrom(c))
	if err != nil {
		return err // traducido por CustomErrorHandler
	}
	return c.Status(fiber.StatusOK).JSON(docs)
}

func (h *documentsHandler) getByID(c *fiber.Ctx) error {
	id, err := validID(c)
	if err != nil {
		return err
	}
	doc, err := h.repo.FindByID(c.Context(), core.RequestIDFrom(c), id)
	if err != nil {
		return err // ErrNotFound -> 404 vía CustomErrorHandler
	}
	return c.Status(fiber.StatusOK).JSON(doc)
}
