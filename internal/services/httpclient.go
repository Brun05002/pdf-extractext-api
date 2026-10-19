package services

import (
	"net"
	"net/http"
	"time"
)

// maxIdleConnsPerHost cubre la concurrencia esperada del spike (100 VUs).
const maxIdleConnsPerHost = 100

// newHTTPClient construye un cliente con pool keep-alive explícito: sin él,
// Go usa el DefaultTransport (2 conexiones idle por host) y las conexiones
// sobrantes se cierran en lugar de reutilizarse bajo carga.
func newHTTPClient(timeout time.Duration) *http.Client {
	transport := &http.Transport{
		MaxIdleConns:        maxIdleConnsPerHost,
		MaxIdleConnsPerHost: maxIdleConnsPerHost,
		IdleConnTimeout:     90 * time.Second,
		DialContext: (&net.Dialer{
			Timeout:   5 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
	}
	return &http.Client{
		Timeout:   timeout,
		Transport: transport,
	}
}
