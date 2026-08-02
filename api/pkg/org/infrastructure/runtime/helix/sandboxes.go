package helix

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	orgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/types"
)

type SandboxController interface {
	RuntimeNames() []string
	DefaultRuntimeName() string
	List(ctx context.Context, orgID, projectID string) ([]*types.Sandbox, error)
	Get(ctx context.Context, id string) (*types.Sandbox, error)
	Create(ctx context.Context, orgID, owner string, req *types.CreateSandboxRequest) (*types.Sandbox, error)
	Update(ctx context.Context, id string, req *types.UpdateSandboxRequest) (*types.Sandbox, error)
	Delete(ctx context.Context, id string) error
	HydraClient(sandbox *types.Sandbox) (*hydra.RevDialClient, error)
}

type SandboxProjectStore interface {
	GetProject(ctx context.Context, id string) (*types.Project, error)
}

type Sandboxes struct {
	orgStore *orgstore.Store
	control  SandboxController
	projects SandboxProjectStore
}

func NewSandboxes(orgStore *orgstore.Store, control SandboxController, projects SandboxProjectStore) (*Sandboxes, error) {
	if orgStore == nil {
		return nil, errors.New("helix.NewSandboxes: org store is nil")
	}
	if control == nil {
		return nil, errors.New("helix.NewSandboxes: controller is nil")
	}
	if projects == nil {
		return nil, errors.New("helix.NewSandboxes: project store is nil")
	}
	return &Sandboxes{orgStore: orgStore, control: control, projects: projects}, nil
}

var _ runtime.Sandboxes = (*Sandboxes)(nil)

func (s *Sandboxes) ListRuntimes(_ context.Context) (runtime.SandboxRuntimeCatalog, error) {
	names := append([]string(nil), s.control.RuntimeNames()...)
	sort.Strings(names)
	return runtime.SandboxRuntimeCatalog{
		Runtimes:       names,
		DefaultRuntime: s.control.DefaultRuntimeName(),
	}, nil
}

func (s *Sandboxes) List(ctx context.Context, orgID, projectID string) ([]runtime.SandboxView, error) {
	if orgID == "" {
		return nil, errors.New("orgID is required")
	}
	values, err := s.control.List(ctx, orgID, projectID)
	if err != nil {
		return nil, fmt.Errorf("list sandboxes: %w", err)
	}
	views := make([]runtime.SandboxView, 0, len(values))
	for _, value := range values {
		view, err := sandboxView(value)
		if err != nil {
			return nil, err
		}
		views = append(views, view)
	}
	return views, nil
}

func (s *Sandboxes) Get(ctx context.Context, orgID, sandboxID string) (runtime.SandboxView, error) {
	value, err := s.ownedSandbox(ctx, orgID, sandboxID)
	if err != nil {
		return runtime.SandboxView{}, err
	}
	return sandboxView(value)
}

func (s *Sandboxes) Create(ctx context.Context, orgID string, botID orgchart.NodeID, in runtime.CreateSandboxInput) (runtime.SandboxView, error) {
	if orgID == "" {
		return runtime.SandboxView{}, errors.New("orgID is required")
	}
	if botID == "" {
		return runtime.SandboxView{}, errors.New("botID is required")
	}
	if in.ProjectID != "" {
		project, err := s.projects.GetProject(ctx, in.ProjectID)
		if err != nil {
			return runtime.SandboxView{}, fmt.Errorf("get project: %w", err)
		}
		if project.OrganizationID != orgID {
			return runtime.SandboxView{}, fmt.Errorf("project %s does not belong to this organization", in.ProjectID)
		}
	}
	owner, err := s.ownerID(ctx, orgID, botID)
	if err != nil {
		return runtime.SandboxView{}, err
	}
	value, err := s.control.Create(ctx, orgID, owner, &types.CreateSandboxRequest{
		Name:           in.Name,
		Runtime:        types.SandboxRuntime(in.Runtime),
		Image:          in.Image,
		Env:            in.Env,
		Tags:           in.Tags,
		TimeoutSeconds: in.TimeoutSeconds,
		VCPUs:          in.VCPUs,
		MemoryMB:       in.MemoryMB,
		DisplayWidth:   in.DisplayWidth,
		DisplayHeight:  in.DisplayHeight,
		DisplayFPS:     in.DisplayFPS,
		ProjectID:      in.ProjectID,
		Persistent:     in.Persistent,
	})
	if err != nil {
		return runtime.SandboxView{}, fmt.Errorf("create sandbox: %w", err)
	}
	return sandboxView(value)
}

func (s *Sandboxes) Update(ctx context.Context, orgID, sandboxID string, in runtime.UpdateSandboxInput) (runtime.SandboxView, error) {
	value, err := s.ownedSandbox(ctx, orgID, sandboxID)
	if err != nil {
		return runtime.SandboxView{}, err
	}
	updated, err := s.control.Update(ctx, value.ID, &types.UpdateSandboxRequest{
		Name:           in.Name,
		TimeoutSeconds: in.TimeoutSeconds,
		Tags:           in.Tags,
	})
	if err != nil {
		return runtime.SandboxView{}, fmt.Errorf("update sandbox: %w", err)
	}
	return sandboxView(updated)
}

func (s *Sandboxes) Delete(ctx context.Context, orgID, sandboxID string) error {
	value, err := s.ownedSandbox(ctx, orgID, sandboxID)
	if err != nil {
		return err
	}
	if err := s.control.Delete(ctx, value.ID); err != nil {
		return fmt.Errorf("delete sandbox: %w", err)
	}
	return nil
}

func (s *Sandboxes) OpenTerminal(ctx context.Context, orgID, sandboxID, shell string) (runtime.SandboxTerminal, error) {
	value, err := s.ownedSandbox(ctx, orgID, sandboxID)
	if err != nil {
		return nil, err
	}
	if value.Status != types.SandboxStatusRunning {
		return nil, fmt.Errorf("sandbox %s is not running (status=%s)", sandboxID, value.Status)
	}
	client, err := s.control.HydraClient(value)
	if err != nil {
		return nil, fmt.Errorf("connect to sandbox host: %w", err)
	}
	terminal, err := client.OpenSandboxTerminal(ctx, value.ID, shell)
	if err != nil {
		return nil, fmt.Errorf("open sandbox terminal: %w", err)
	}
	return terminal, nil
}

func (s *Sandboxes) ownedSandbox(ctx context.Context, orgID, sandboxID string) (*types.Sandbox, error) {
	if orgID == "" {
		return nil, errors.New("orgID is required")
	}
	if strings.TrimSpace(sandboxID) == "" {
		return nil, errors.New("sandbox_id is required")
	}
	value, err := s.control.Get(ctx, sandboxID)
	if err != nil {
		return nil, fmt.Errorf("get sandbox: %w", err)
	}
	if value.OrganizationID != orgID {
		return nil, fmt.Errorf("sandbox %s does not belong to this organization", sandboxID)
	}
	return value, nil
}

func (s *Sandboxes) ownerID(ctx context.Context, orgID string, botID orgchart.NodeID) (string, error) {
	if userID := strings.TrimSpace(UserIDFromContext(ctx)); userID != "" {
		return userID, nil
	}
	state, err := LoadState(ctx, s.orgStore, orgID, botID)
	if err != nil {
		return "", fmt.Errorf("resolve sandbox owner: %w", err)
	}
	if strings.TrimSpace(state.HiringUserID) == "" {
		return "", fmt.Errorf("bot %s has no hiring user; restart it from Helix before creating a sandbox", botID)
	}
	return state.HiringUserID, nil
}

func sandboxView(value *types.Sandbox) (runtime.SandboxView, error) {
	if value == nil {
		return runtime.SandboxView{}, errors.New("sandbox is nil")
	}
	var tags map[string]string
	if len(value.Tags) > 0 {
		if err := json.Unmarshal(value.Tags, &tags); err != nil {
			return runtime.SandboxView{}, fmt.Errorf("decode sandbox %s tags: %w", value.ID, err)
		}
	}
	return runtime.SandboxView{
		ID:             value.ID,
		Name:           value.Name,
		OrganizationID: value.OrganizationID,
		ProjectID:      value.ProjectID,
		Owner:          value.Owner,
		Runtime:        string(value.Runtime),
		Image:          value.Image,
		Status:         string(value.Status),
		StatusMessage:  value.StatusMessage,
		VCPUs:          value.VCPUs,
		MemoryMB:       value.MemoryMB,
		Persistent:     value.Persistent,
		Tags:           tags,
		TimeoutSeconds: value.TimeoutSeconds,
		CreatedAt:      value.CreatedAt,
		UpdatedAt:      value.UpdatedAt,
		StartedAt:      value.StartedAt,
		StoppedAt:      value.StoppedAt,
		ExpiresAt:      value.ExpiresAt,
	}, nil
}
