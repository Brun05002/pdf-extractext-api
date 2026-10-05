package services

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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
	Extract(ctx context.Context, filename string, body io.Reader) (*ExtractionResult, error)
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

// Extract envía el PDF como multipart/form-data (contrato C1 del extractor,
// campo "file") streameando por io.Pipe: el cuerpo nunca se materializa entero.
func (c *HTTPExtractionClient) Extract(ctx context.Context, filename string, body io.Reader) (*ExtractionResult, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	go func() {
		part, err := writer.CreateFormFile("file", filename)
		if err == nil {
			_, err = io.Copy(part, body)
		}
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		_ = pw.CloseWithError(err)
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/extract", pr)
	if err != nil {
		return nil, fmt.Errorf("construir request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

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
