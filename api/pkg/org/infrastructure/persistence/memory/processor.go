package memory

import (
	"context"
	"fmt"
	"sort"
	"sync"

	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

// processorsRepo is the in-memory implementation of store.Processors.
// Mirrors topicsRepo: composite (orgID, id) key, (orgID, name)
// uniqueness, ErrNotFound on cross-tenant lookups.
type processorsRepo struct {
	mu   sync.RWMutex
	rows map[orgKey]processor.Processor
}

func (s *processorsRepo) Create(_ context.Context, p processor.Processor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := orgKey{OrgID: p.OrganizationID, ID: string(p.ID)}
	if _, ok := s.rows[k]; ok {
		return fmt.Errorf("processor %q in org %q: already exists", p.ID, p.OrganizationID)
	}
	for k2, ex := range s.rows {
		if k2.OrgID == p.OrganizationID && ex.Name == p.Name {
			return fmt.Errorf("processor name %q already in use in org %q", p.Name, p.OrganizationID)
		}
	}
	s.rows[k] = p
	return nil
}

func (s *processorsRepo) Get(_ context.Context, orgID string, id processor.ProcessorID) (processor.Processor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if p, ok := s.rows[orgKey{OrgID: orgID, ID: string(id)}]; ok {
		return p, nil
	}
	return processor.Processor{}, fmt.Errorf("processor %q in org %q: %w", id, orgID, store.ErrNotFound)
}

func (s *processorsRepo) List(_ context.Context, orgID string) ([]processor.Processor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]processor.Processor, 0)
	for k, p := range s.rows {
		if k.OrgID == orgID {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *processorsRepo) ListByInputTopic(_ context.Context, orgID string, in streaming.TopicID) ([]processor.Processor, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]processor.Processor, 0)
	for k, p := range s.rows {
		if k.OrgID == orgID && p.InputTopicID == in {
			out = append(out, p)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (s *processorsRepo) Update(_ context.Context, p processor.Processor) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := orgKey{OrgID: p.OrganizationID, ID: string(p.ID)}
	existing, ok := s.rows[k]
	if !ok {
		return fmt.Errorf("processor %q in org %q: %w", p.ID, p.OrganizationID, store.ErrNotFound)
	}
	if p.Name != existing.Name {
		for k2, ex := range s.rows {
			if k2 == k {
				continue
			}
			if k2.OrgID == p.OrganizationID && ex.Name == p.Name {
				return fmt.Errorf("processor name %q already in use in org %q", p.Name, p.OrganizationID)
			}
		}
	}
	// Mutable fields only — keep ID / OrgID / CreatedBy / CreatedAt.
	existing.Name = p.Name
	existing.InputTopicID = p.InputTopicID
	existing.Kind = p.Kind
	existing.Config = p.Config
	existing.Outputs = p.Outputs
	s.rows[k] = existing
	return nil
}

func (s *processorsRepo) Delete(_ context.Context, orgID string, id processor.ProcessorID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	k := orgKey{OrgID: orgID, ID: string(id)}
	if _, ok := s.rows[k]; !ok {
		return fmt.Errorf("processor %q in org %q: %w", id, orgID, store.ErrNotFound)
	}
	delete(s.rows, k)
	return nil
}
