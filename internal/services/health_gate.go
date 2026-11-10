package services

import (
	"context"
	"errors"
	"fmt"
	"io"
)

// ErrDependencyUnavailable indica que un upstream no tiene instancias sanas.
var ErrDependencyUnavailable = errors.New("dependencia no disponible")

// HealthChecker abstrae el health gate para desacoplar los clientes de Consul.
type HealthChecker interface {
	IsHealthy(ctx context.Context, service string) (bool, error)
}

type dependencyGuard struct {
	checker HealthChecker
	service string
}

func (g *dependencyGuard) allow(ctx context.Context) error {
	healthy, err := g.checker.IsHealthy(ctx, g.service)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrDependencyUnavailable, err)
	}
	if !healthy {
		return ErrDependencyUnavailable
	}
	return nil
}

type gatedExtraction struct {
	next  ExtractionClient
	guard *dependencyGuard
}

// NewHealthGatedExtraction decora el cliente sin cambiar su interfaz.
func NewHealthGatedExtraction(next ExtractionClient, checker HealthChecker, service string) ExtractionClient {
	return &gatedExtraction{
		next:  next,
		guard: &dependencyGuard{checker: checker, service: service},
	}
}

func (g *gatedExtraction) Extract(ctx context.Context, requestID, filename string, body io.Reader) (*ExtractionResult, error) {
	if err := g.guard.allow(ctx); err != nil {
		return nil, err
	}
	return g.next.Extract(ctx, requestID, filename, body)
}

type gatedPersistence struct {
	next  PersistenceRepository
	guard *dependencyGuard
}

// NewHealthGatedPersistence decora el repositorio sin cambiar su interfaz.
func NewHealthGatedPersistence(next PersistenceRepository, checker HealthChecker, service string) PersistenceRepository {
	return &gatedPersistence{
		next:  next,
		guard: &dependencyGuard{checker: checker, service: service},
	}
}

func (g *gatedPersistence) Save(ctx context.Context, requestID string, doc *Document) (*Document, error) {
	if err := g.guard.allow(ctx); err != nil {
		return nil, err
	}
	return g.next.Save(ctx, requestID, doc)
}

func (g *gatedPersistence) FindByID(ctx context.Context, requestID, id string) (*Document, error) {
	if err := g.guard.allow(ctx); err != nil {
		return nil, err
	}
	return g.next.FindByID(ctx, requestID, id)
}

func (g *gatedPersistence) FindByChecksum(ctx context.Context, requestID, checksum string) (*Document, error) {
	if err := g.guard.allow(ctx); err != nil {
		return nil, err
	}
	return g.next.FindByChecksum(ctx, requestID, checksum)
}

func (g *gatedPersistence) FindAll(ctx context.Context, requestID string) ([]*Document, error) {
	if err := g.guard.allow(ctx); err != nil {
		return nil, err
	}
	return g.next.FindAll(ctx, requestID)
}

func (g *gatedPersistence) Update(ctx context.Context, requestID, id string, doc *Document) (*Document, error) {
	if err := g.guard.allow(ctx); err != nil {
		return nil, err
	}
	return g.next.Update(ctx, requestID, id, doc)
}

func (g *gatedPersistence) Delete(ctx context.Context, requestID, id string) error {
	if err := g.guard.allow(ctx); err != nil {
		return err
	}
	return g.next.Delete(ctx, requestID, id)
}
