package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"time"
)

// La API solo expone {content, page_count} en POST /extract.
type ExtractionResult struct {
	Filename      string `json:"filename"`
	ExtractedText string `json:"extracted_text"`
	Checksum      string `json:"checksum"`
	Content       string `json:"content"`
	PageCount     int    `json:"page_count"`
}

// ExtractionClient abstrae la comunicación con el microservicio de extracción.
type ExtractionClient interface {
	Extract(ctx context.Context, requestID, filename string, body io.Reader) (*ExtractionResult, error)
}

// HTTPExtractionClient envía el stream al upstream con timeout estricto.
type HTTPExtractionClient struct {
	baseURL string
	host    string
	client  *http.Client
}

func NewHTTPExtractionClient(baseURL, host string, timeout time.Duration) *HTTPExtractionClient {
	return &HTTPExtractionClient{
		baseURL: baseURL,
		host:    host,
		client:  newHTTPClient(timeout),
	}
}

// Extract envía el PDF como multipart/form-data streameando por io.Pipe: el cuerpo nunca se materializa entero.
// Solo reintenta cuerpos rebobinables para no consumir streams de /extract más de una vez.
func (c *HTTPExtractionClient) Extract(ctx context.Context, requestID, filename string, body io.Reader) (*ExtractionResult, error) {
	seeker, replayable := body.(io.Seeker)
	if !replayable {
		return c.extractAttempt(ctx, requestID, filename, body, false)
	}

	retryAfter := ""
	for attempt := 1; attempt <= defaultMaxAttempts; attempt++ {
		if attempt > 1 {
			if _, err := seeker.Seek(0, io.SeekStart); err != nil {
				return nil, fmt.Errorf("rebobinar cuerpo: %w", err)
			}
			if err := sleepBeforeRetry(ctx, attempt-1, retryAfter); err != nil {
				return nil, err
			}
			retryAfter = ""
		}

		result, err := c.extractAttempt(ctx, requestID, filename, body, true)
		if err == nil {
			return result, nil
		}

		var upstream *UpstreamError
		if errors.As(err, &upstream) {
			retryAfter = upstream.RetryAfter
		}
		if !isRetryableExtractionError(err) || attempt == defaultMaxAttempts {
			return nil, err
		}
	}
	return nil, errors.New("extraction upstream: reintentos agotados")
}

func isRetryableExtractionError(err error) bool {
	var upstream *UpstreamError
	if errors.As(err, &upstream) {
		return retryableStatus(upstream.Status)
	}
	return retryableError(err)
}

func (c *HTTPExtractionClient) extractAttempt(ctx context.Context, requestID, filename string, body io.Reader, waitProducer bool) (*ExtractionResult, error) {
	pr, pw := io.Pipe()
	writer := multipart.NewWriter(pw)
	contentType := writer.FormDataContentType()
	done := make(chan struct{})
	go func() {
		defer close(done)
		part, err := writer.CreateFormFile("file", filename)
		if err == nil {
			_, err = io.Copy(part, body)
		}
		if closeErr := writer.Close(); err == nil {
			err = closeErr
		}
		_ = pw.CloseWithError(err)
	}()

	stopProducer := func() {
		_ = pr.CloseWithError(io.ErrClosedPipe)
		if waitProducer {
			<-done
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/extract", pr)
	if err != nil {
		stopProducer()
		return nil, fmt.Errorf("construir request: %w", err)
	}
	req.Header.Set("Content-Type", contentType)
	// El router interno de Traefik (:8090) enruta a Extraction por Host.
	if c.host != "" {
		req.Host = c.host
	}
	if requestID != "" {
		req.Header.Set("X-Request-ID", requestID)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		stopProducer()
		return nil, fmt.Errorf("extraction upstream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		upstreamErr := readUpstreamError(resp)
		_ = resp.Body.Close()
		stopProducer()
		return nil, upstreamErr
	}

	var result ExtractionResult
	decodeErr := json.NewDecoder(resp.Body).Decode(&result)
	_ = resp.Body.Close()
	stopProducer()
	if decodeErr != nil {
		return nil, fmt.Errorf("decodificar respuesta del upstream: %w", decodeErr)
	}
	return &result, nil
}
