package services

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGatedExtraction_HealthyCallsUpstream(t *testing.T) {
	upstream := &countingExtraction{}
	gate := NewHealthGatedExtraction(upstream, &fakeChecker{healthy: true}, "extraction")

	result, err := gate.Extract(t.Context(), "rid", "doc.pdf", nil)
	require.NoError(t, err)
	assert.Equal(t, "ok", result.Content)
	assert.Equal(t, 1, upstream.calls)
}

func TestGatedExtraction_UnhealthySkipsUpstream(t *testing.T) {
	upstream := &countingExtraction{}
	gate := NewHealthGatedExtraction(upstream, &fakeChecker{healthy: false}, "extraction")

	_, err := gate.Extract(t.Context(), "rid", "doc.pdf", nil)
	require.ErrorIs(t, err, ErrDependencyUnavailable)
	assert.Equal(t, 0, upstream.calls, "no debe invocarse el upstream sin instancias sanas")
}

func TestGatedExtraction_UsesConfiguredService(t *testing.T) {
	checker := &fakeChecker{healthy: true}
	gate := NewHealthGatedExtraction(&countingExtraction{}, checker, "extraction")

	_, _ = gate.Extract(t.Context(), "rid", "doc.pdf", nil)
	assert.Equal(t, "extraction", checker.service)
}

func TestGatedPersistence_UnhealthySkipsAllMethods(t *testing.T) {
	upstream := &countingPersistence{}
	gate := NewHealthGatedPersistence(upstream, &fakeChecker{healthy: false}, "persistence")
	ctx := t.Context()

	_, err := gate.Save(ctx, "rid", &Document{Filename: "a.pdf"})
	require.ErrorIs(t, err, ErrDependencyUnavailable)
	_, err = gate.FindByID(ctx, "rid", "id")
	require.ErrorIs(t, err, ErrDependencyUnavailable)
	_, err = gate.FindAll(ctx, "rid")
	require.ErrorIs(t, err, ErrDependencyUnavailable)
	_, err = gate.Update(ctx, "rid", "id", &Document{})
	require.ErrorIs(t, err, ErrDependencyUnavailable)
	err = gate.Delete(ctx, "rid", "id")
	require.ErrorIs(t, err, ErrDependencyUnavailable)

	assert.Equal(t, 0, upstream.calls)
}

func TestGatedPersistence_HealthyCallsUpstream(t *testing.T) {
	upstream := &countingPersistence{}
	gate := NewHealthGatedPersistence(upstream, &fakeChecker{healthy: true}, "persistence")

	saved, err := gate.Save(t.Context(), "rid", &Document{Filename: "a.pdf"})
	require.NoError(t, err)
	assert.Equal(t, "id", saved.ID)
	assert.Equal(t, 1, upstream.calls)
}

func TestGatedExtraction_ConsulErrorPropagatesAsUnavailable(t *testing.T) {
	gate := NewHealthGatedExtraction(&countingExtraction{}, &fakeChecker{err: errors.New("consul caído")}, "extraction")

	_, err := gate.Extract(t.Context(), "rid", "doc.pdf", nil)
	require.ErrorIs(t, err, ErrDependencyUnavailable)
}

type fakeChecker struct {
	healthy bool
	err     error
	service string
}

func (f *fakeChecker) IsHealthy(ctx context.Context, service string) (bool, error) {
	f.service = service
	return f.healthy, f.err
}

type countingExtraction struct {
	calls int
}

func (c *countingExtraction) Extract(ctx context.Context, requestID, filename string, body io.Reader) (*ExtractionResult, error) {
	c.calls++
	return &ExtractionResult{Content: "ok", PageCount: 1}, nil
}

type countingPersistence struct {
	calls int
}

func (c *countingPersistence) Save(ctx context.Context, requestID string, doc *Document) (*Document, error) {
	c.calls++
	return &Document{ID: "id", Filename: doc.Filename}, nil
}

func (c *countingPersistence) FindByID(ctx context.Context, requestID, id string) (*Document, error) {
	c.calls++
	return &Document{ID: id}, nil
}

func (c *countingPersistence) FindByChecksum(ctx context.Context, requestID, checksum string) (*Document, error) {
	c.calls++
	return nil, ErrNotFound
}

func (c *countingPersistence) FindAll(ctx context.Context, requestID string) ([]*Document, error) {
	c.calls++
	return nil, nil
}

func (c *countingPersistence) Update(ctx context.Context, requestID, id string, doc *Document) (*Document, error) {
	c.calls++
	return &Document{ID: id}, nil
}

func (c *countingPersistence) Delete(ctx context.Context, requestID, id string) error {
	c.calls++
	return nil
}
