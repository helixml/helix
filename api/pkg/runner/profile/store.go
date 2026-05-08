// Package profile is the runner-profile service: thin layer over the
// underlying store that re-derives Models + GPURequirement.Count from the
// compose YAML on every save, while persisting operator-declared GPU
// compatibility fields verbatim.
//
// Callers (HTTP handlers, the assignment endpoint, the router) use this
// package; they should not call into store.Store for runner profiles
// directly because that would bypass the parse-on-save invariant.
package profile

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/runner/composeparse"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
)

// DB is the subset of store.Store this package needs. Defined as a local
// interface so tests can mock it without pulling in the full store mock.
type DB interface {
	CreateRunnerProfile(ctx context.Context, p *types.RunnerProfile) (*types.RunnerProfile, error)
	GetRunnerProfile(ctx context.Context, id string) (*types.RunnerProfile, error)
	GetRunnerProfileByName(ctx context.Context, name string) (*types.RunnerProfile, error)
	UpdateRunnerProfile(ctx context.Context, p *types.RunnerProfile) (*types.RunnerProfile, error)
	DeleteRunnerProfile(ctx context.Context, id string) error
	ListRunnerProfiles(ctx context.Context) ([]*types.RunnerProfile, error)
}

// Service handles profile CRUD with parse-on-save semantics.
type Service struct {
	db DB
}

// New creates a profile service backed by the given DB. In production wire
// it with the real store.Store; in tests pass a mock.
func New(db DB) *Service {
	return &Service{db: db}
}

// SaveInput is what callers pass for create or update. The compose YAML is
// the source of truth for Models + GPURequirement.Count; vendor /
// architectures / model_match / min_vram_bytes are taken verbatim.
//
// On Update, ID must be set. On Create, ID is generated.
type SaveInput struct {
	ID            string // empty on Create, set on Update
	Name          string
	Description   string
	ComposeYAML   string
	Vendor        types.GPUVendor
	Architectures []string
	ModelMatch    string
	MinVRAMBytes  int64
}

// Create parses the compose YAML, derives metadata, and persists the
// profile. Returns the persisted profile (including its generated ID).
func (s *Service) Create(ctx context.Context, in SaveInput) (*types.RunnerProfile, error) {
	if in.ID != "" {
		return nil, errors.New("Create: ID must be empty (use Update for existing profiles)")
	}
	p, err := s.buildProfile(in)
	if err != nil {
		return nil, err
	}
	p.ID = system.GenerateRunnerProfileID()
	return s.db.CreateRunnerProfile(ctx, p)
}

// Update parses the new compose YAML, re-derives metadata, and persists.
// Returns the persisted profile.
func (s *Service) Update(ctx context.Context, in SaveInput) (*types.RunnerProfile, error) {
	if in.ID == "" {
		return nil, errors.New("Update: ID is required")
	}
	// Make sure it exists (returns ErrNotFound if not).
	existing, err := s.db.GetRunnerProfile(ctx, in.ID)
	if err != nil {
		return nil, err
	}
	p, err := s.buildProfile(in)
	if err != nil {
		return nil, err
	}
	p.ID = existing.ID
	p.CreatedAt = existing.CreatedAt
	return s.db.UpdateRunnerProfile(ctx, p)
}

// Get returns a profile by ID. Returns store.ErrNotFound if absent.
func (s *Service) Get(ctx context.Context, id string) (*types.RunnerProfile, error) {
	return s.db.GetRunnerProfile(ctx, id)
}

// GetByName returns a profile by its (unique) name.
func (s *Service) GetByName(ctx context.Context, name string) (*types.RunnerProfile, error) {
	return s.db.GetRunnerProfileByName(ctx, name)
}

// List returns all profiles ordered by name.
func (s *Service) List(ctx context.Context) ([]*types.RunnerProfile, error) {
	return s.db.ListRunnerProfiles(ctx)
}

// Delete removes a profile by ID. Idempotent at the DB layer; returns
// ErrNotFound for missing IDs so callers can return 404.
func (s *Service) Delete(ctx context.Context, id string) error {
	return s.db.DeleteRunnerProfile(ctx, id)
}

// buildProfile validates input, parses the compose YAML, and constructs a
// profile struct ready to persist. Does not touch the DB.
func (s *Service) buildProfile(in SaveInput) (*types.RunnerProfile, error) {
	if in.Name == "" {
		return nil, errors.New("profile name is required")
	}
	if in.ComposeYAML == "" {
		return nil, errors.New("compose YAML is required")
	}
	parsed, err := composeparse.Parse([]byte(in.ComposeYAML))
	if err != nil {
		return nil, fmt.Errorf("compose parse: %w", err)
	}
	if in.Vendor != "" && in.Vendor != types.GPUVendorNVIDIA && in.Vendor != types.GPUVendorAMD {
		return nil, fmt.Errorf("invalid vendor %q (must be empty, %q, or %q)", in.Vendor, types.GPUVendorNVIDIA, types.GPUVendorAMD)
	}
	return &types.RunnerProfile{
		Name:        in.Name,
		Description: in.Description,
		ComposeYAML: in.ComposeYAML,
		Models:      parsed.Models,
		GPURequirement: types.ProfileGPURequirement{
			Count:         parsed.GPUCount,
			Vendor:        in.Vendor,
			Architectures: in.Architectures,
			ModelMatch:    in.ModelMatch,
			MinVRAMBytes:  in.MinVRAMBytes,
		},
	}, nil
}

// Compile-time interface assertion that store.Store satisfies our DB
// interface. Catches drift between the store and this package.
var _ DB = (store.Store)(nil)
