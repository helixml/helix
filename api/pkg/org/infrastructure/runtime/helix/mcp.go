package helix

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/types"
)

// HelixOrgMCPName is the AssistantMCP.Name slot the helix-org MCP entry
// occupies on a Worker's agent app. Upsert is keyed on this string —
// keep it stable across the codebase.
const HelixOrgMCPName = "helix"

// AttachHelixOrgMCP idempotently writes the helix-org MCP entry onto
// the per-Worker agent app's first assistant.
//
// Why this isn't done inside the project-apply path: applyProject
// (api/pkg/server/project_handlers.go) wholesale-replaces
// agentApp.Config.Helix when an agent app already exists, so anything
// attached previously is wiped on every re-apply. Re-attaching after
// the apply lands is the invariant that keeps the helix-org MCP
// present across activations. Both call sites — the Spawner (per AI
// Worker activation) and dynamicProjectApplier (per owner-chat
// ensureWorker call) — invoke this; the function is the single source
// of truth for what the MCP entry looks like.
//
// Upsert is keyed on HelixOrgMCPName: an existing entry with the same
// name is overwritten in place, otherwise appended. MCP configuration is
// persisted on a long-lived app, so it must use the long-lived service key,
// never the activation request's session-scoped bearer. Empty bearer means
// no Authorization header is sent — fine for standalone deployments where
// the MCP route is not auth-gated.
func AttachHelixOrgMCP(
	ctx context.Context,
	svc ProjectService,
	appID string,
	helixOrgURL string,
	workerID orgchart.BotID,
	fallbackBearer string,
) error {
	if svc == nil {
		return errors.New("AttachHelixOrgMCP: ProjectService is nil")
	}
	if appID == "" {
		return errors.New("AttachHelixOrgMCP: appID is empty")
	}
	if helixOrgURL == "" {
		return errors.New("AttachHelixOrgMCP: helixOrgURL is empty")
	}
	if workerID == "" {
		return errors.New("AttachHelixOrgMCP: workerID is empty")
	}

	cfg, err := svc.GetAppConfig(ctx, appID)
	if err != nil {
		return fmt.Errorf("get app config: %w", err)
	}
	if len(cfg.Helix.Assistants) == 0 {
		return errors.New("AttachHelixOrgMCP: app has no assistants")
	}

	var headers map[string]string
	if fallbackBearer != "" {
		headers = map[string]string{"Authorization": "Bearer " + fallbackBearer}
	}
	entry := types.AssistantMCP{
		Name:      HelixOrgMCPName,
		Transport: "http",
		URL:       strings.TrimRight(helixOrgURL, "/") + "/workers/" + string(workerID) + "/mcp",
		Headers:   headers,
	}

	asst := &cfg.Helix.Assistants[0]
	replaced := false
	for i := range asst.MCPs {
		if asst.MCPs[i].Name == entry.Name {
			asst.MCPs[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		asst.MCPs = append(asst.MCPs, entry)
	}
	if err := svc.UpdateAppConfig(ctx, appID, cfg); err != nil {
		return fmt.Errorf("update app config: %w", err)
	}
	return nil
}
