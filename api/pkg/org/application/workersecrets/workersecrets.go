package workersecrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/workersecret"
)

type SourceResolver interface {
	Validate(ctx context.Context, binding workersecret.Binding) error
	Resolve(ctx context.Context, binding workersecret.Binding) (workersecret.Resolved, error)
}

type Recorder func(ctx context.Context, organizationID string, workerID orgchart.NodeID, name, sourceID string, result workersecret.Resolved, err error)
type Catalog func(context.Context, string, orgchart.NodeID) ([]workersecret.AvailableSource, error)

type Service struct {
	repo     store.WorkerSecretBindings
	nodes    store.Nodes
	resolver SourceResolver
	now      func() time.Time
	record   Recorder
	catalog  Catalog
}

func New(repo store.WorkerSecretBindings, nodes store.Nodes, resolver SourceResolver, now func() time.Time, record Recorder, catalog ...Catalog) (*Service, error) {
	if repo == nil || nodes == nil || resolver == nil {
		return nil, fmt.Errorf("worker secrets requires bindings, nodes, and resolver")
	}
	if now == nil {
		now = time.Now
	}
	s := &Service{repo: repo, nodes: nodes, resolver: resolver, now: now, record: record}
	if len(catalog) > 0 {
		s.catalog = catalog[0]
	}
	return s, nil
}

func (s *Service) Available(ctx context.Context, orgID string, workerID orgchart.NodeID) ([]workersecret.AvailableSource, error) {
	if _, err := s.nodes.Get(ctx, orgID, workerID); err != nil {
		return nil, fmt.Errorf("get worker: %w", err)
	}
	if s.catalog == nil {
		return []workersecret.AvailableSource{}, nil
	}
	sources, err := s.catalog(ctx, orgID, workerID)
	if err != nil {
		return nil, err
	}
	bindings, err := s.repo.List(ctx, orgID, workerID)
	if err != nil {
		return nil, err
	}
	for i := range sources {
		for _, b := range bindings {
			if sourceID(b) == availableSourceID(sources[i]) {
				sources[i].AlreadyBound = true
				break
			}
		}
	}
	return sources, nil
}
func availableSourceID(s workersecret.AvailableSource) string {
	if s.SourceKind == workersecret.SourceHelixSecret {
		return s.SecretID
	}
	return s.AccountID + "/" + s.ExportKey
}

func (s *Service) Put(ctx context.Context, binding workersecret.Binding) (workersecret.Binding, error) {
	binding.Name = strings.TrimSpace(binding.Name)
	if err := binding.Validate(); err != nil {
		return workersecret.Binding{}, err
	}
	if _, err := s.nodes.Get(ctx, binding.OrganizationID, binding.WorkerID); err != nil {
		return workersecret.Binding{}, fmt.Errorf("get worker: %w", err)
	}
	if err := s.resolver.Validate(ctx, binding); err != nil {
		return workersecret.Binding{}, fmt.Errorf("validate secret source: %w", err)
	}
	now := s.now().UTC()
	existing, err := s.repo.Get(ctx, binding.OrganizationID, binding.WorkerID, binding.Name)
	if err == nil {
		binding.CreatedAt = existing.CreatedAt
		binding.UpdatedAt = now
		if err := s.repo.Update(ctx, binding); err != nil {
			return workersecret.Binding{}, err
		}
		return binding, nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return workersecret.Binding{}, err
	}
	binding.CreatedAt = now
	binding.UpdatedAt = now
	if err := s.repo.Create(ctx, binding); err != nil {
		return workersecret.Binding{}, err
	}
	return binding, nil
}
func (s *Service) List(ctx context.Context, orgID string, workerID orgchart.NodeID) ([]workersecret.Binding, error) {
	if _, err := s.nodes.Get(ctx, orgID, workerID); err != nil {
		return nil, fmt.Errorf("get worker: %w", err)
	}
	return s.repo.List(ctx, orgID, workerID)
}
func (s *Service) Delete(ctx context.Context, orgID string, workerID orgchart.NodeID, name string) error {
	return s.repo.Delete(ctx, orgID, workerID, name)
}
func (s *Service) Descriptors(ctx context.Context, orgID string, workerID orgchart.NodeID) ([]workersecret.Descriptor, error) {
	bindings, err := s.List(ctx, orgID, workerID)
	if err != nil {
		return nil, err
	}
	out := make([]workersecret.Descriptor, 0, len(bindings))
	for _, b := range bindings {
		d := descriptor(b)
		if err := s.resolver.Validate(ctx, b); err != nil {
			d.Available = false
		} else {
			d.Available = true
		}
		out = append(out, d)
	}
	return out, nil
}
func (s *Service) Get(ctx context.Context, orgID string, workerID orgchart.NodeID, name string) (workersecret.Resolved, error) {
	b, err := s.repo.Get(ctx, orgID, workerID, strings.TrimSpace(name))
	if err != nil {
		return workersecret.Resolved{}, err
	}
	res, err := s.resolver.Resolve(ctx, b)
	if s.record != nil {
		s.record(ctx, orgID, workerID, b.Name, sourceID(b), res, err)
	}
	if err != nil {
		return workersecret.Resolved{}, fmt.Errorf("resolve secret %q: %w", b.Name, err)
	}
	res.Name = b.Name
	return res, nil
}
func descriptor(b workersecret.Binding) workersecret.Descriptor {
	return workersecret.Descriptor{Name: b.Name, Description: b.Description, Usage: b.Usage, ContentType: b.ContentType, SuggestedFilename: b.SuggestedFilename}
}
func sourceID(b workersecret.Binding) string {
	if b.SourceKind == workersecret.SourceHelixSecret {
		return b.SecretID
	}
	return b.AccountID + "/" + b.ExportKey
}
