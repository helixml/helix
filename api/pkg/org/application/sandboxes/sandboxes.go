// Package sandboxes exposes org-scoped standalone sandbox management to MCP
// tools without coupling the org application layer to Helix's sandbox
// controller.
package sandboxes

import (
	"context"
	"errors"
	"fmt"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
)

type MemberVerifier interface {
	GetBot(ctx context.Context, orgID string, id orgchart.NodeID) (orgchart.Node, error)
}

type Service struct {
	port    runtime.Sandboxes
	members MemberVerifier
}

func New(port runtime.Sandboxes, members MemberVerifier) *Service {
	return &Service{port: port, members: members}
}

func (s *Service) callerIdentity(ctx context.Context, caller tool.Caller) (string, orgchart.NodeID, error) {
	if caller == nil {
		return "", "", errors.New("caller missing on invocation")
	}
	orgID := caller.OrganizationID()
	if orgID == "" {
		return "", "", errors.New("caller has no organization id")
	}
	botID := orgchart.NodeID(caller.ID())
	if botID == "" {
		return "", "", errors.New("caller has no bot id")
	}
	if s.members != nil {
		if _, err := s.members.GetBot(ctx, orgID, botID); err != nil {
			return "", "", fmt.Errorf("caller bot %s is not a member of org %s: %w", botID, orgID, err)
		}
	}
	return orgID, botID, nil
}

func (s *Service) ListRuntimes(ctx context.Context, caller tool.Caller) (runtime.SandboxRuntimeCatalog, error) {
	if _, _, err := s.callerIdentity(ctx, caller); err != nil {
		return runtime.SandboxRuntimeCatalog{}, err
	}
	return s.port.ListRuntimes(ctx)
}

func (s *Service) List(ctx context.Context, caller tool.Caller, projectID string) ([]runtime.SandboxView, error) {
	orgID, _, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return nil, err
	}
	return s.port.List(ctx, orgID, projectID)
}

func (s *Service) Get(ctx context.Context, caller tool.Caller, sandboxID string) (runtime.SandboxView, error) {
	orgID, _, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SandboxView{}, err
	}
	return s.port.Get(ctx, orgID, sandboxID)
}

func (s *Service) Create(ctx context.Context, caller tool.Caller, in runtime.CreateSandboxInput) (runtime.SandboxView, error) {
	orgID, botID, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SandboxView{}, err
	}
	return s.port.Create(ctx, orgID, botID, in)
}

func (s *Service) Update(ctx context.Context, caller tool.Caller, sandboxID string, in runtime.UpdateSandboxInput) (runtime.SandboxView, error) {
	orgID, _, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return runtime.SandboxView{}, err
	}
	return s.port.Update(ctx, orgID, sandboxID, in)
}

func (s *Service) Delete(ctx context.Context, caller tool.Caller, sandboxID string) error {
	orgID, _, err := s.callerIdentity(ctx, caller)
	if err != nil {
		return err
	}
	return s.port.Delete(ctx, orgID, sandboxID)
}
