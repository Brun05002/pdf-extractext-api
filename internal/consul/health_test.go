package consul

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCachedHealth_Healthy(t *testing.T) {
	query := &fakeHealthQuery{healthy: true}
	health := NewCachedHealth(query, time.Minute, true)

	healthy, err := health.IsHealthy(t.Context(), "extraction")
	require.NoError(t, err)
	assert.True(t, healthy)
	assert.Equal(t, []string{"extraction"}, query.services)
}

func TestCachedHealth_Unhealthy(t *testing.T) {
	query := &fakeHealthQuery{healthy: false}
	health := NewCachedHealth(query, time.Minute, true)

	healthy, err := health.IsHealthy(t.Context(), "persistence")
	require.NoError(t, err)
	assert.False(t, healthy)
}

func TestCachedHealth_CacheHitWithinTTL(t *testing.T) {
	query := &fakeHealthQuery{healthy: true}
	health := NewCachedHealth(query, time.Minute, true)

	_, _ = health.IsHealthy(t.Context(), "extraction")
	_, _ = health.IsHealthy(t.Context(), "extraction")

	assert.Equal(t, 1, query.callCount())
}

func TestCachedHealth_CacheExpiresAfterTTL(t *testing.T) {
	query := &fakeHealthQuery{healthy: true}
	health := NewCachedHealth(query, time.Second, true)
	current := time.Unix(0, 0)
	health.now = func() time.Time { return current }

	_, _ = health.IsHealthy(t.Context(), "extraction")
	current = current.Add(2 * time.Second)
	_, _ = health.IsHealthy(t.Context(), "extraction")

	assert.Equal(t, 2, query.callCount())
}

func TestCachedHealth_FailOpenAllowsCall(t *testing.T) {
	query := &fakeHealthQuery{err: errors.New("consul caído")}
	health := NewCachedHealth(query, time.Minute, true)

	healthy, err := health.IsHealthy(t.Context(), "extraction")
	require.NoError(t, err)
	assert.True(t, healthy, "fail-open debe permitir la llamada")
}

func TestCachedHealth_FailClosedReturnsError(t *testing.T) {
	query := &fakeHealthQuery{err: errors.New("consul caído")}
	health := NewCachedHealth(query, time.Minute, false)

	healthy, err := health.IsHealthy(t.Context(), "extraction")
	require.Error(t, err)
	assert.False(t, healthy)
}

func TestCachedHealth_ConcurrentAccess(t *testing.T) {
	query := &fakeHealthQuery{healthy: true}
	health := NewCachedHealth(query, time.Hour, true)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			healthy, err := health.IsHealthy(context.Background(), "extraction")
			assert.NoError(t, err)
			assert.True(t, healthy)
		}()
	}
	wg.Wait()

	assert.GreaterOrEqual(t, query.callCount(), 1)
}

type fakeHealthQuery struct {
	mu       sync.Mutex
	healthy  bool
	err      error
	calls    int
	services []string
}

func (f *fakeHealthQuery) HealthPassing(ctx context.Context, service string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.services = append(f.services, service)
	return f.healthy, f.err
}

func (f *fakeHealthQuery) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}
