package consul

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// HealthQuery abstrae /v1/health/service para testear sin Consul.
type HealthQuery interface {
	HealthPassing(ctx context.Context, service string) (bool, error)
}

// CachedHealth evita consultar Consul en cada request con un TTL corto por servicio.
type CachedHealth struct {
	query    HealthQuery
	ttl      time.Duration
	failOpen bool
	now      func() time.Time

	mu    sync.RWMutex
	cache map[string]healthEntry
}

type healthEntry struct {
	healthy bool
	expires time.Time
}

// NewCachedHealth construye el caché con el TTL y la política fail-open configurados.
func NewCachedHealth(query HealthQuery, ttl time.Duration, failOpen bool) *CachedHealth {
	return &CachedHealth{
		query:    query,
		ttl:      ttl,
		failOpen: failOpen,
		now:      time.Now,
		cache:    make(map[string]healthEntry),
	}
}

// IsHealthy devuelve si el servicio tiene instancias passing, usando caché dentro del TTL.
func (h *CachedHealth) IsHealthy(ctx context.Context, service string) (bool, error) {
	if entry, ok := h.lookup(service); ok {
		return entry.healthy, nil
	}

	healthy, err := h.query.HealthPassing(ctx, service)
	if err != nil {
		if h.failOpen {
			slog.Warn("health gate: Consul no responde; fail-open permite la llamada", "service", service, "error", err)
			return true, nil
		}
		return false, err
	}

	h.store(service, healthy)
	return healthy, nil
}

func (h *CachedHealth) lookup(service string) (healthEntry, bool) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	entry, ok := h.cache[service]
	if !ok || !h.now().Before(entry.expires) {
		return healthEntry{}, false
	}
	return entry, true
}

func (h *CachedHealth) store(service string, healthy bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.cache[service] = healthEntry{healthy: healthy, expires: h.now().Add(h.ttl)}
}
