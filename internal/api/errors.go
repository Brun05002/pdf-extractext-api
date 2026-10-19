package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/pdf-extractext/api/internal/services"
)

// problemMeta agrupa el URN y el title de cada code (RFC 9457 §3).
type problemMeta struct {
	urn   string
	title string
}

// problemCatalog es el catálogo cerrado de errores de la API. Prohibido
// añadir códigos fuera de esta lista (YAGNI).
var problemCatalog = map[string]problemMeta{
	"INVALID_REQUEST":        {"invalid-request", "Petición inválida"},
	"NOT_FOUND":              {"not-found", "Recurso no encontrado"},
	"CONFLICT":               {"conflict", "Conflicto con el recurso"},
	"METHOD_NOT_ALLOWED":     {"method-not-allowed", "Método no permitido"},
	"UNSUPPORTED_MEDIA_TYPE": {"unsupported-media-type", "Tipo de contenido no soportado"},
	"REQUEST_TOO_LARGE":      {"request-too-large", "Cuerpo demasiado grande"},
	"INVALID_PDF_CONTENT":    {"invalid-pdf-content", "Contenido PDF inválido"},
	"SERVICE_OVERLOADED":     {"service-overloaded", "Servicio saturado"},
	"BAD_GATEWAY":            {"bad-gateway", "Fallo en un servicio interno"},
	"GATEWAY_TIMEOUT":        {"gateway-timeout", "Tiempo de espera agotado"},
	"INTERNAL_ERROR":         {"internal-error", "Error interno del servidor"},
}

// statusCodeStrings mapea códigos HTTP a su code del catálogo.
var statusCodeStrings = map[int]string{
	fiber.StatusBadRequest:            "INVALID_REQUEST",
	fiber.StatusNotFound:              "NOT_FOUND",
	fiber.StatusMethodNotAllowed:      "METHOD_NOT_ALLOWED",
	fiber.StatusUnsupportedMediaType:  "UNSUPPORTED_MEDIA_TYPE",
	fiber.StatusRequestEntityTooLarge: "REQUEST_TOO_LARGE",
	fiber.StatusBadGateway:            "BAD_GATEWAY",
	fiber.StatusGatewayTimeout:        "GATEWAY_TIMEOUT",
	fiber.StatusRequestTimeout:        "GATEWAY_TIMEOUT",
}

// problem responde con el problem RFC 9457 del code indicado.
func problem(c *fiber.Ctx, status int, code, detail string) error {
	meta, ok := problemCatalog[code]
	if !ok { // fallback defensivo: nunca emitir un code fuera del catálogo
		meta = problemCatalog["INTERNAL_ERROR"]
		code = "INTERNAL_ERROR"
		status = fiber.StatusInternalServerError
	}
	// El header se setea después de JSON(): Fiber lo sobrescribe a application/json.
	err := c.Status(status).JSON(fiber.Map{
		"type":   "urn:pdf-extractext:" + meta.urn,
		"title":  meta.title,
		"status": status,
		"detail": detail,
		"code":   code,
	})
	c.Set(fiber.HeaderContentType, "application/problem+json")
	return err
}

// problemUpstream re-emite el problem del upstream preservando su URN, code
// y detail, y el header Retry-After si lo trae.
func problemUpstream(c *fiber.Ctx, ue *services.UpstreamError) error {
	if ue.RetryAfter != "" {
		c.Set(fiber.HeaderRetryAfter, ue.RetryAfter)
	}
	body := fiber.Map{
		"type":   ue.Type,
		"title":  ue.Title,
		"status": ue.Status,
		"detail": ue.Detail,
		"code":   ue.Code,
	}
	if body["type"] == "" { // upstream sin problem+json: URN genérico
		body["type"] = "urn:pdf-extractext:upstream-error"
		body["title"] = "Error de un servicio interno"
	}
	err := c.Status(ue.Status).JSON(body)
	c.Set(fiber.HeaderContentType, "application/problem+json")
	return err
}

// isTimeout detecta agotamiento de tiempo de cualquier capa (contexto, red o
// deadline) sin depender de qué timeout disparó primero.
func isTimeout(err error) bool {
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, os.ErrDeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// respondUpstream traduce un error de upstream hacia el cliente: propaga 503
// (con Retry-After) y los 4xx del upstream; red/timeout/500 caen a 502.
func respondUpstream(c *fiber.Ctx, err error, fallbackDetail string) error {
	if isTimeout(err) {
		return problem(c, fiber.StatusGatewayTimeout, "GATEWAY_TIMEOUT", "un servicio interno excedió el tiempo límite")
	}
	var ue *services.UpstreamError
	if errors.As(err, &ue) &&
		(ue.Status == http.StatusServiceUnavailable || (ue.Status >= 400 && ue.Status < 500)) {
		return problemUpstream(c, ue)
	}
	return problem(c, fiber.StatusBadGateway, "BAD_GATEWAY", fallbackDetail)
}

// CustomErrorHandler traduce cualquier error al problem RFC 9457 estándar
// cumpliendo la firma fiber.ErrorHandler.
func CustomErrorHandler(c *fiber.Ctx, err error) error {
	var ue *services.UpstreamError
	if errors.As(err, &ue) {
		return problemUpstream(c, ue)
	}

	switch {
	case errors.Is(err, services.ErrNotFound):
		return problem(c, fiber.StatusNotFound, "NOT_FOUND", err.Error())
	case errors.Is(err, services.ErrConflict):
		return problem(c, fiber.StatusConflict, "CONFLICT", err.Error())
	}

	var fiberErr *fiber.Error
	if errors.As(err, &fiberErr) {
		code, ok := statusCodeStrings[fiberErr.Code]
		if !ok {
			if fiberErr.Code < fiber.StatusInternalServerError {
				code = "INVALID_REQUEST"
			} else {
				code = "INTERNAL_ERROR"
			}
		}
		return problem(c, fiberErr.Code, code, fiberErr.Message)
	}

	// Mensaje interno nunca se filtra al cliente.
	return problem(c, fiber.StatusInternalServerError, "INTERNAL_ERROR", "error interno del servidor")
}
