package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ExtractionResult es la respuesta del servicio de extracción.
type ExtractionResult struct {
	Content   string `json:"content"`
	PageCount int    `json:"page_count"`
}

// ExtractionClient abstrae la comunicación con el microservicio de extracción.
type ExtractionClient interface {
	Extract(ctx context.Context, body io.Reader) (*ExtractionResult, error)
}

// HTTPExtractionClient envía el stream al upstream con timeout estricto.
type HTTPExtractionClient struct {
	baseURL string
	client  *http.Client
}

func NewHTTPExtractionClient(baseURL string, timeout time.Duration) *HTTPExtractionClient {
	return &HTTPExtractionClient{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *HTTPExtractionClient) Extract(ctx context.Context, body io.Reader) (*ExtractionResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/extract", body)
	if err != nil {
		return nil, fmt.Errorf("construir request: %w", err)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("extraction upstream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("extraction upstream respondió status %d", resp.StatusCode)
	}

	var result ExtractionResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decodificar respuesta del upstream: %w", err)
	}
	return &result, nil
}
