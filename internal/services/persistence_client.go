package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Errores de dominio mapeados desde las respuestas del servicio de persistencia.
var (
	ErrNotFound = errors.New("documento no encontrado")
	ErrConflict = errors.New("el documento ya existe")
)

// Document es la entidad del microservicio de persistencia.
type Document struct {
	ID            string `json:"id,omitempty"`
	Filename      string `json:"filename"`
	ExtractedText string `json:"extracted_text"`
	Checksum      string `json:"checksum"`
}

// saveDocumentRequest es el payload exacto de POST /documents.
type saveDocumentRequest struct {
	Filename      string `json:"filename"`
	ExtractedText string `json:"extracted_text,omitempty"`
	Checksum      string `json:"checksum"`
}

// PersistenceRepository abstrae el CRUD contra el microservicio de persistencia.
// requestID propaga X-Request-ID end-to-end en los logs (FIND-014).
type PersistenceRepository interface {
	Save(ctx context.Context, requestID string, doc *Document) (*Document, error)
	FindByID(ctx context.Context, requestID, id string) (*Document, error)
	FindAll(ctx context.Context, requestID string) ([]*Document, error)
	Update(ctx context.Context, requestID, id string, doc *Document) (*Document, error)
	Delete(ctx context.Context, requestID, id string) error
}

// HTTPPersistenceClient implementa PersistenceRepository via HTTP/JSON.
type HTTPPersistenceClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPPersistenceClient(baseURL string, timeout time.Duration) *HTTPPersistenceClient {
	return &HTTPPersistenceClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

// newRequest construye la petición con X-Request-ID para correlación (FIND-014).
func (c *HTTPPersistenceClient) newRequest(ctx context.Context, method, path, requestID string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("construir request: %w", err)
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}
	return req, nil
}

// Save crea un documento (POST /documents). 409 -> ErrConflict.
func (c *HTTPPersistenceClient) Save(ctx context.Context, requestID string, doc *Document) (*Document, error) {
	payload, err := json.Marshal(saveDocumentRequest{
		Filename:      doc.Filename,
		ExtractedText: doc.ExtractedText,
		Checksum:      doc.Checksum,
	})
	if err != nil {
		return nil, fmt.Errorf("serializar documento: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPost, "/documents", requestID, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("persistence upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict {
		return nil, ErrConflict
	}
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, readUpstreamError(resp)
	}

	var saved Document
	if err := json.NewDecoder(resp.Body).Decode(&saved); err != nil {
		return nil, fmt.Errorf("decodificar respuesta: %w", err)
	}
	return &saved, nil
}

// FindByID obtiene un documento (GET /documents/{id}). 404 -> ErrNotFound.
func (c *HTTPPersistenceClient) FindByID(ctx context.Context, requestID, id string) (*Document, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/documents/"+id, requestID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("persistence upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, readUpstreamError(resp)
	}

	var doc Document
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decodificar respuesta: %w", err)
	}
	return &doc, nil
}

// Update reemplaza campos de un documento (PATCH /documents/{id}).
func (c *HTTPPersistenceClient) Update(ctx context.Context, requestID, id string, doc *Document) (*Document, error) {
	// Persistence solo admite filename en PATCH (decodificación estricta).
	payload, err := json.Marshal(struct {
		Filename string `json:"filename"`
	}{Filename: doc.Filename})
	if err != nil {
		return nil, fmt.Errorf("serializar documento: %w", err)
	}

	req, err := c.newRequest(ctx, http.MethodPatch, "/documents/"+id, requestID, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("persistence upstream: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound:
		return nil, ErrNotFound
	case http.StatusConflict:
		return nil, ErrConflict
	case http.StatusOK:
		// continúa a la decodificación
	default:
		return nil, readUpstreamError(resp)
	}

	var updated Document
	if err := json.NewDecoder(resp.Body).Decode(&updated); err != nil {
		return nil, fmt.Errorf("decodificar respuesta: %w", err)
	}
	return &updated, nil
}

func (c *HTTPPersistenceClient) Delete(ctx context.Context, requestID, id string) error {
	req, err := c.newRequest(ctx, http.MethodDelete, "/documents/"+id, requestID, nil)
	if err != nil {
		return err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("persistence upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return readUpstreamError(resp)
	}
	return nil
}

func (c *HTTPPersistenceClient) FindAll(ctx context.Context, requestID string) ([]*Document, error) {
	req, err := c.newRequest(ctx, http.MethodGet, "/documents", requestID, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("persistence upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, readUpstreamError(resp)
	}

	var docs []*Document
	if err := json.NewDecoder(resp.Body).Decode(&docs); err != nil {
		return nil, fmt.Errorf("decodificar respuesta: %w", err)
	}
	return docs, nil
}
