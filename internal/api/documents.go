package api

import (
	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/services"
)

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

func (h *documentsHandler) update(c *fiber.Ctx) error {
	id := c.Params("id")

	var doc services.Document
	if err := c.BodyParser(&doc); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "el cuerpo de la petición no es un JSON válido")
	}

	updated, err := h.repo.Update(c.Context(), id, &doc)
	if err != nil {
		return err // ErrNotFound -> 404, ErrConflict -> 409
	}
	return c.Status(fiber.StatusOK).JSON(updated)
}

func (h *documentsHandler) delete(c *fiber.Ctx) error {
	id := c.Params("id")
	if err := h.repo.Delete(c.Context(), id); err != nil {
		return err // ErrNotFound -> 404
	}
	return c.SendStatus(fiber.StatusNoContent)
}

func (h *documentsHandler) list(c *fiber.Ctx) error {
	docs, err := h.repo.FindAll(c.Context())
	if err != nil {
		return err // traducido por CustomErrorHandler
	}
	return c.Status(fiber.StatusOK).JSON(docs)
}

func (h *documentsHandler) getByID(c *fiber.Ctx) error {
	id := c.Params("id")
	doc, err := h.repo.FindByID(c.Context(), id)
	if err != nil {
		return err // ErrNotFound -> 404 vía CustomErrorHandler
	}
	return c.Status(fiber.StatusOK).JSON(doc)
}
