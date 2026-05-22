package helixclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	runtimehelix "github.com/helixml/helix/api/pkg/org/runtime/helix"
	"github.com/helixml/helix/api/pkg/types"
)

// AsProjectService returns a runtimehelix.ProjectService adapter
// backed by this helixclient. Used during the H1.x transitional state
// — the H1.2 lift introduced the ProjectService port but kept the
// legacy helixclient as the runtime impl. Subsequent commits will swap
// the adapter for direct controller calls.
func AsProjectService(c Client) runtimehelix.ProjectService {
	return &projectServiceAdapter{c: c}
}

type projectServiceAdapter struct {
	c Client
}

func (a *projectServiceAdapter) WhoAmI(ctx context.Context) (string, error) {
	u, err := a.c.WhoAmI(ctx)
	if err != nil {
		return "", err
	}
	return u.User, nil
}

func (a *projectServiceAdapter) ApplyProject(ctx context.Context, req types.ProjectApplyRequest) (types.ProjectApplyResponse, error) {
	hcReq := ProjectApplyRequest{
		OrganizationID: req.OrganizationID,
		Name:           req.Name,
		Spec: ProjectSpec{
			Description:  req.Spec.Description,
			Technologies: req.Spec.Technologies,
			Guidelines:   req.Spec.Guidelines,
		},
	}
	if req.Spec.Agent != nil {
		hcReq.Spec.Agent = &ProjectAgentSpec{
			Name:        req.Spec.Agent.Name,
			Runtime:     req.Spec.Agent.Runtime,
			Provider:    req.Spec.Agent.Provider,
			Model:       req.Spec.Agent.Model,
			Credentials: req.Spec.Agent.Credentials,
		}
	}
	resp, err := a.c.ApplyProject(ctx, hcReq)
	if err != nil {
		return types.ProjectApplyResponse{}, err
	}
	return types.ProjectApplyResponse{
		ProjectID:  resp.ProjectID,
		AgentAppID: resp.AgentAppID,
		Created:    resp.Created,
	}, nil
}

func (a *projectServiceAdapter) GetProject(ctx context.Context, id string) (types.Project, error) {
	p, err := a.c.GetProject(ctx, id)
	if err != nil {
		if errors.Is(err, ErrNotFound) {
			return types.Project{}, runtimehelix.ErrProjectNotFound
		}
		return types.Project{}, err
	}
	return types.Project{
		ID:             p.ID,
		Name:           p.Name,
		UserID:         p.UserID,
		OrganizationID: p.OrganizationID,
		DefaultRepoID:  p.DefaultRepoID,
	}, nil
}

func (a *projectServiceAdapter) PutProjectSecret(ctx context.Context, projectID, name, value string) error {
	return a.c.PutProjectSecret(ctx, projectID, name, value)
}

func (a *projectServiceAdapter) CreateGitRepo(ctx context.Context, req types.GitRepositoryCreateRequest) (types.GitRepository, error) {
	repo, err := a.c.CreateGitRepo(ctx, CreateGitRepoRequest{
		Name:           req.Name,
		OwnerID:        req.OwnerID,
		OrganizationID: req.OrganizationID,
		InitialFiles:   req.InitialFiles,
	})
	if err != nil {
		return types.GitRepository{}, err
	}
	return types.GitRepository{
		ID:   repo.ID,
		Name: repo.Name,
	}, nil
}

func (a *projectServiceAdapter) AttachRepoToProject(ctx context.Context, projectID, repoID string, primary bool) error {
	return a.c.AttachRepoToProject(ctx, projectID, repoID, primary)
}

func (a *projectServiceAdapter) CreateBranch(ctx context.Context, repoID, branch, baseBranch string) error {
	return a.c.CreateBranch(ctx, repoID, branch, baseBranch)
}

func (a *projectServiceAdapter) GetAppConfig(ctx context.Context, id string) (types.AppConfig, error) {
	app, err := a.c.GetApp(ctx, id)
	if err != nil {
		return types.AppConfig{}, err
	}
	if len(app.Config) == 0 {
		return types.AppConfig{}, nil
	}
	var cfg types.AppConfig
	if err := json.Unmarshal(app.Config, &cfg); err != nil {
		return types.AppConfig{}, fmt.Errorf("decode app config: %w", err)
	}
	return cfg, nil
}

func (a *projectServiceAdapter) UpdateAppConfig(ctx context.Context, id string, cfg types.AppConfig) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("encode app config: %w", err)
	}
	_, err = a.c.UpdateApp(ctx, id, AppRequest{Config: raw})
	return err
}
