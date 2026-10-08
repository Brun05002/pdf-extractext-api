package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Errores de dominio mapeados desde las respuestas del servicio de persistencia.
var (
	ErrNotFound = errors.New("documento no encontrado")
	ErrConflict = errors.New("el documento ya existe")
)

// Document es la entidad persistida por el microservicio de persistencia.
type Document struct {
	ID        string `json:"id"`
	Filename  string `json:"filename"`
	Content   string `json:"content"`
	PageCount int    `json:"page_count"`
}

// PersistenceRepository abstrae el CRUD contra el microservicio de persistencia.
type PersistenceRepository interface {
	Save(ctx context.Context, doc *Document) (*Document, error)
	FindByID(ctx context.Context, id string) (*Document, error)
	FindAll(ctx context.Context) ([]*Document, error)
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

// Save crea un documento (POST /documents). 409 -> ErrConflict.
func (c *HTTPPersistenceClient) Save(ctx context.Context, doc *Document) (*Document, error) {
	body, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("serializar documento: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/documents", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("construir request: %w", err)
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
		return nil, fmt.Errorf("persistence upstream respondió status %d", resp.StatusCode)
	}

	var saved Document
	if err := json.NewDecoder(resp.Body).Decode(&saved); err != nil {
		return nil, fmt.Errorf("decodificar respuesta: %w", err)
	}
	return &saved, nil
}

// FindByID obtiene un documento (GET /documents/{id}). 404 -> ErrNotFound.
func (c *HTTPPersistenceClient) FindByID(ctx context.Context, id string) (*Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/documents/"+id, nil)
	if err != nil {
		return nil, fmt.Errorf("construir request: %w", err)
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
		return nil, fmt.Errorf("persistence upstream respondió status %d", resp.StatusCode)
	}

	var doc Document
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return nil, fmt.Errorf("decodificar respuesta: %w", err)
	}
	return &doc, nil
}

// FindAll lista todos los documentos (GET /documents).
func (c *HTTPPersistenceClient) FindAll(ctx context.Context) ([]*Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/documents", nil)
	if err != nil {
		return nil, fmt.Errorf("construir request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("persistence upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("persistence upstream respondió status %d", resp.StatusCode)
	}

	var docs []*Document
	if err := json.NewDecoder(resp.Body).Decode(&docs); err != nil {
		return nil, fmt.Errorf("decodificar respuesta: %w", err)
	}
	return docs, nil
}
