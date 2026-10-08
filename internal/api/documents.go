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
