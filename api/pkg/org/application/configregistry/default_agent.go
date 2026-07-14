package configregistry

import "context"

const DefaultAgentConfigKey = "agent.default"

type DefaultAgentConfig struct {
	Runtime     string `json:"runtime"`
	Credentials string `json:"credentials"`
	Provider    string `json:"provider"`
	Model       string `json:"model"`
}

func (r *Registry) GetDefaultAgentConfig(ctx context.Context, orgID string) (DefaultAgentConfig, error) {
	var cfg DefaultAgentConfig
	if r.IsConfigured(ctx, orgID, DefaultAgentConfigKey) {
		return cfg, r.GetObject(ctx, orgID, DefaultAgentConfigKey, &cfg)
	}

	cfg.Runtime, _ = r.GetString(ctx, orgID, "worker.runtime")
	cfg.Credentials, _ = r.GetString(ctx, orgID, "worker.credentials")
	cfg.Provider, _ = r.GetString(ctx, orgID, "worker.provider")
	cfg.Model, _ = r.GetString(ctx, orgID, "worker.model")
	return cfg, nil
}

func (r *Registry) IsDefaultAgentConfigured(ctx context.Context, orgID string) bool {
	return r.IsConfigured(ctx, orgID, DefaultAgentConfigKey) || r.IsConfigured(ctx, orgID, "worker.runtime")
}
