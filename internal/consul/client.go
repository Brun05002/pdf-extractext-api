package consul

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// consulTokenHeader es el header ACL que exige Consul cuando hay token configurado.
const consulTokenHeader = "X-Consul-Token"

// Client es un cliente mínimo de Consul construido solo con net/http.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client
}

// NewClient crea el cliente con timeout acotado para no bloquear el arranque.
func NewClient(baseURL, token string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Token:      token,
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
	}
}

// Register da de alta un servicio: PUT /v1/agent/service/register.
func (c *Client) Register(ctx context.Context, payload any) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(payload); err != nil {
		return fmt.Errorf("serializar registro de Consul: %w", err)
	}

	resp, err := c.do(ctx, http.MethodPut, "/v1/agent/service/register", &body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return statusError(resp)
	}
	return nil
}

// Deregister da de baja un servicio: PUT /v1/agent/service/deregister/{id}.
func (c *Client) Deregister(ctx context.Context, id string) error {
	resp, err := c.do(ctx, http.MethodPut, "/v1/agent/service/deregister/"+url.PathEscape(id), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return statusError(resp)
	}
	return nil
}

// HealthPassing indica si hay al menos una instancia passing para el servicio.
func (c *Client) HealthPassing(ctx context.Context, service string) (bool, error) {
	path := "/v1/health/service/" + url.PathEscape(service) + "?passing=true"
	resp, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, statusError(resp)
	}

	var instances []json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&instances); err != nil {
		return false, fmt.Errorf("decodificar health de Consul: %w", err)
	}
	return len(instances) > 0, nil
}

// KVList lee un prefijo completo y devuelve las claves relativas ya decodificadas.
func (c *Client) KVList(ctx context.Context, prefix string) (map[string]string, error) {
	cleanPrefix := strings.Trim(prefix, "/")
	resp, err := c.do(ctx, http.MethodGet, "/v1/kv/"+escapePath(cleanPrefix)+"?recurse=true", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, statusError(resp)
	}

	var entries []struct {
		Key   string  `json:"Key"`
		Value *string `json:"Value"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("decodificar KV de Consul: %w", err)
	}

	values := make(map[string]string, len(entries))
	for _, entry := range entries {
		key := strings.TrimPrefix(entry.Key, cleanPrefix+"/")
		value, err := decodeValue(entry.Value)
		if err != nil {
			return nil, fmt.Errorf("decodificar valor KV %q: %w", key, err)
		}
		values[key] = value
	}
	return values, nil
}

func decodeValue(raw *string) (string, error) {
	if raw == nil || *raw == "" {
		return "", nil
	}
	decoded, err := base64.StdEncoding.DecodeString(*raw)
	if err != nil {
		return "", err
	}
	return string(decoded), nil
}

// do centraliza la inyección del token ACL en todas las peticiones.
func (c *Client) do(ctx context.Context, method, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, fmt.Errorf("construir request de Consul: %w", err)
	}
	if c.Token != "" {
		req.Header.Set(consulTokenHeader, c.Token)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("consul: %w", err)
	}
	return resp, nil
}

func statusError(resp *http.Response) error {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("consul respondió status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}

// escapePath codifica cada segmento sin romper las barras del prefijo KV.
func escapePath(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		segments[i] = url.PathEscape(segment)
	}
	return strings.Join(segments, "/")
}
