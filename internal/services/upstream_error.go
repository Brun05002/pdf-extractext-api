package services

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// UpstreamError preserva el error RFC 9457 de un upstream (status, URN,
// code, detail y el header Retry-After si existe) para que la capa de API
// pueda re-emirlo sin colapsarlo en un 502/500 genérico (FIND-012/FIND-013).
type UpstreamError struct {
	Status     int
	Type       string
	Title      string
	Code       string
	Detail     string
	RetryAfter string
}

func (e *UpstreamError) Error() string {
	return fmt.Sprintf("upstream respondió status %d (%s)", e.Status, e.Code)
}

// readUpstreamError lee el problem+json de una respuesta de error y lo
// convierte en *UpstreamError. Si el cuerpo no es un problem válido,
// conserva un detail genérico con el status.
func readUpstreamError(resp *http.Response) *UpstreamError {
	ue := &UpstreamError{
		Status:     resp.StatusCode,
		RetryAfter: resp.Header.Get("Retry-After"),
	}
	var problem struct {
		Type   string `json:"type"`
		Title  string `json:"title"`
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&problem); err == nil {
		ue.Type = problem.Type
		ue.Title = problem.Title
		ue.Code = problem.Code
		ue.Detail = problem.Detail
	}
	if ue.Detail == "" {
		ue.Detail = fmt.Sprintf("el servicio respondió status %d", resp.StatusCode)
	}
	return ue
}
