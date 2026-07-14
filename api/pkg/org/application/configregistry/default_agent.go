package configregistry

import (
	"context"

	"github.com/helixml/helix/api/pkg/types"
)

const DefaultAgentConfigKey = "agent.default"

func (r *Registry) GetDefaultAgentConfig(ctx context.Context, orgID string) (types.AssistantConfig, error) {
	var cfg types.AssistantConfig
	if r.IsConfigured(ctx, orgID, DefaultAgentConfigKey) {
		return cfg, r.GetObject(ctx, orgID, DefaultAgentConfigKey, &cfg)
	}

	runtime, _ := r.GetString(ctx, orgID, "worker.runtime")
	credentials, _ := r.GetString(ctx, orgID, "worker.credentials")
	cfg.CodeAgentRuntime = types.CodeAgentRuntime(runtime)
	cfg.CodeAgentCredentialType = types.CodeAgentCredentialType(credentials)
	cfg.Provider, _ = r.GetString(ctx, orgID, "worker.provider")
	cfg.Model, _ = r.GetString(ctx, orgID, "worker.model")
	return cfg, nil
}

func (r *Registry) IsDefaultAgentConfigured(ctx context.Context, orgID string) bool {
	return r.IsConfigured(ctx, orgID, DefaultAgentConfigKey) || r.IsConfigured(ctx, orgID, "worker.runtime")
}
