package helix

import (
	"context"
	"fmt"
)

// HelixOrgMCPName is the AssistantMCP.Name slot the helix-org MCP entry
// occupies on a Worker's agent app. Upsert is keyed on this string —
// keep it stable across the codebase.
const HelixOrgMCPName = "helix"

func RemoveHelixOrgMCP(ctx context.Context, svc ProjectService, appID string) error {
	cfg, err := svc.GetAppConfig(ctx, appID)
	if err != nil {
		return fmt.Errorf("get app config: %w", err)
	}
	if len(cfg.Helix.Assistants) == 0 {
		return nil
	}
	assistant := &cfg.Helix.Assistants[0]
	kept := assistant.MCPs[:0]
	for _, mcp := range assistant.MCPs {
		if mcp.Name != HelixOrgMCPName {
			kept = append(kept, mcp)
		}
	}
	if len(kept) == len(assistant.MCPs) {
		return nil
	}
	assistant.MCPs = kept
	if err := svc.UpdateAppConfig(ctx, appID, cfg); err != nil {
		return fmt.Errorf("update app config: %w", err)
	}
	return nil
}
