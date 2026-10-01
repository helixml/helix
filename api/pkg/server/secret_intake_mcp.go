package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	helixorgstore "github.com/helixml/helix/api/pkg/org/domain/store"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

type secretIntakeMCPService struct {
	api      *HelixAPIServer
	orgStore *helixorgstore.Store
}

func (s *secretIntakeMCPService) botProject(ctx context.Context, orgID, botID string) (string, error) {
	state, err := runtimehelix.LoadState(ctx, s.orgStore, orgID, orgchart.NodeID(botID))
	if err != nil || state.ProjectID == "" {
		return "", fmt.Errorf("caller has no Helix project")
	}
	project, err := s.api.Store.GetProject(ctx, state.ProjectID)
	if err != nil || project.OrganizationID != orgID {
		return "", fmt.Errorf("caller project is unavailable")
	}
	return project.ID, nil
}

func (s *secretIntakeMCPService) Create(ctx context.Context, orgID, botID string, input types.SecretIntakeCreateRequest) (types.SecretIntakeCreateResult, error) {
	projectID, err := s.botProject(ctx, orgID, botID)
	if err != nil {
		return types.SecretIntakeCreateResult{}, err
	}
	// Bind the intake to the requesting session (instance or bot main
	// session) so the submission wake reaches the right conversation.
	view, link, err := s.api.createSecretIntake(ctx, projectID, input, runtimehelix.SessionIDFromContext(ctx))
	if err != nil {
		return types.SecretIntakeCreateResult{}, err
	}
	return types.SecretIntakeCreateResult{ID: view.ID, Status: view.Status, InviteURL: link}, nil
}

func (s *secretIntakeMCPService) Status(ctx context.Context, orgID, botID, intakeID string) (types.SecretIntakeStatusResult, error) {
	projectID, err := s.botProject(ctx, orgID, botID)
	if err != nil {
		return types.SecretIntakeStatusResult{}, err
	}
	if !s.api.secretIntakeEnabled() {
		return types.SecretIntakeStatusResult{}, fmt.Errorf("secret intake is disabled")
	}
	item, err := s.api.Store.GetSecretIntake(ctx, projectID, intakeID)
	if err != nil {
		if !errors.Is(err, store.ErrNotFound) {
			return types.SecretIntakeStatusResult{}, fmt.Errorf("unable to load intake")
		}
		return types.SecretIntakeStatusResult{}, fmt.Errorf("intake not found")
	}
	return types.SecretIntakeStatusResult{ID: item.ID, Status: secretIntakeStatus(item), ExpiresAt: item.InvitationExpiresAt}, nil
}
