package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix-org/agent"
	"github.com/helixml/helix-org/broadcast"
	helixorgconfig "github.com/helixml/helix-org/config"
	"github.com/helixml/helix-org/domain"
	"github.com/helixml/helix-org/helix/helixclient"
	helixorgstore "github.com/helixml/helix-org/store"
)

// newEmbeddedSpawner returns an agent.Spawner that activates each AI
// Worker as a fresh Helix `helix_agent` chat session against a
// per-Worker clone of the picked owner agent (chat.app_id).
//
// Why a per-Worker clone instead of reusing the owner's agent: the
// owner's agent has MCP attached at /api/v1/mcp/helix-org/workers/
// w-owner/mcp, so every subscribe/publish call would scope to
// w-owner. Cloning the agent and rewriting that one MCP entry to
// /workers/<workerID>/mcp restores the per-Worker scope helix-org's
// grants enforcement expects.
//
// Clones are created lazily on the first activation and recorded in
// the helix-org config registry under `worker.<id>.app_id`, so a
// re-activation reuses the same app and Helix's app-side state
// (interaction history etc.) accumulates correctly.
//
// The Spawner has no Zed sandbox dependency — every activation is a
// plain LLM call through Helix's existing helix_agent path, so cold
// start latency is one model call's worth, not a Zed boot. This is
// the right trade-off for the alpha; if a Worker eventually needs
// shell tools, swap this Spawner for helix.Spawner (zed_external).
func newEmbeddedSpawner(deps *embeddedSpawnerDeps) agent.Spawner {
	return func(ctx context.Context, workerID domain.WorkerID, _ string, triggers []agent.Trigger) error {
		if len(triggers) == 0 {
			return fmt.Errorf("spawner: no triggers")
		}
		streamID := agent.ActivationStreamID(workerID)
		publish := func(body string) { deps.publishActivation(ctx, workerID, streamID, body) }

		publish(fmt.Sprintf("=== activation: %s ===", agent.DescribeTriggers(triggers)))

		mandate, err := deps.buildMandate(ctx, workerID)
		if err != nil {
			publish(fmt.Sprintf("=== exit: build mandate: %s ===", err.Error()))
			return fmt.Errorf("build mandate for %s: %w", workerID, err)
		}
		prompt := agent.BuildPrompt(workerID, mandate, triggers)

		appID, err := deps.resolveWorkerApp(ctx, workerID)
		if err != nil {
			publish(fmt.Sprintf("=== exit: provision app: %s ===", err.Error()))
			return fmt.Errorf("resolve worker app: %w", err)
		}

		sessionRole, _ := deps.configReg.GetString(ctx, "chat.session_role")
		req := helixclient.StartChatRequest{
			AppID:       appID,
			SessionRole: sessionRole,
			Type:        "text",
			Messages:    []helixclient.SessionChatMessage{helixclient.NewTextMessage("user", prompt)},
		}
		session, _, err := deps.client.StartChatWithStatus(ctx, req)
		if err != nil {
			publish(fmt.Sprintf("=== exit: start chat: %s ===", err.Error()))
			return fmt.Errorf("start chat for %s: %w", workerID, err)
		}
		for _, ix := range session.Interactions {
			if ix == nil {
				continue
			}
			if ix.ResponseMessage != "" {
				publish(ix.ResponseMessage)
			}
			if ix.State == "error" && ix.Error != "" {
				publish("error: " + ix.Error)
			}
		}
		publish("=== exit: ok ===")
		log.Info().Str("worker", string(workerID)).Str("session", session.ID).Str("app", appID).Msg("embedded spawner: activation finished")
		return nil
	}
}

// embeddedSpawnerDeps bundles everything the Spawner closure needs.
// Kept private to this file so server.go doesn't reach in.
type embeddedSpawnerDeps struct {
	orgStore    *helixorgstore.Store
	configReg   *helixorgconfig.Registry
	broadcaster *broadcast.Broadcaster
	client      helixclient.Client
	helixURL    string
	logger      *slog.Logger
	newID       func() string
	now         func() time.Time

	// appsMu serialises per-worker app provisioning so two concurrent
	// activations of the same Worker don't both call CreateApp.
	appsMu sync.Mutex
}

// buildMandate composes the per-Worker policy text the activation
// prompt wraps. Role.Content + IdentityContent + the org-wide agent
// policy. Mirrors what claude.Spawner.projectEnv writes to disk; here
// we just stuff it into the prompt since there is no env directory.
func (d *embeddedSpawnerDeps) buildMandate(ctx context.Context, workerID domain.WorkerID) (string, error) {
	worker, err := d.orgStore.Workers.Get(ctx, workerID)
	if err != nil {
		return "", fmt.Errorf("get worker: %w", err)
	}
	positions := worker.Positions()
	if len(positions) == 0 {
		return "", fmt.Errorf("worker %s has no positions", workerID)
	}
	pos, err := d.orgStore.Positions.Get(ctx, positions[0])
	if err != nil {
		return "", fmt.Errorf("get position: %w", err)
	}
	role, err := d.orgStore.Roles.Get(ctx, pos.RoleID)
	if err != nil {
		return "", fmt.Errorf("get role: %w", err)
	}

	var b strings.Builder
	b.WriteString("# Role\n\n")
	b.WriteString(role.Content)
	b.WriteString("\n\n# Identity\n\n")
	b.WriteString(worker.IdentityContent())
	b.WriteString("\n\n# Policy\n\n")
	b.WriteString(agent.Policy)
	return b.String(), nil
}

// resolveWorkerApp returns a Helix agent app ID dedicated to workerID,
// cloning the operator-picked owner agent (chat.app_id) the first
// time. The clone differs from the original only in its MCP entry
// for "helix-org", which is rewritten to point at
// /api/v1/mcp/helix-org/workers/<workerID>/mcp so the worker's
// subscribe/publish calls scope to the right Worker.
//
// The mapping is persisted in helix-org's config registry under
// `worker.<id>.app_id`. We chose the config store (rather than
// extending the Workers schema) because the alpha is single-tenant
// and the binding is operationally a config concern, not a domain
// invariant — a future refactor can surface it on the Worker record
// if multi-tenant cleanup ever needs it.
func (d *embeddedSpawnerDeps) resolveWorkerApp(ctx context.Context, workerID domain.WorkerID) (string, error) {
	key := fmt.Sprintf("worker.%s.app_id", workerID)
	// Register the spec the first time we see this worker; on
	// subsequent activations the key is already declared and a second
	// Register would panic ("config: key … already registered").
	if _, ok := d.configReg.Spec(key); !ok {
		d.configReg.Register(helixorgconfig.Spec{
			Key:         key,
			Type:        helixorgconfig.TypeString,
			Description: "per-Worker Helix agent app id (clone of chat.app_id with worker-scoped MCP)",
		})
	}
	if existing, _ := d.configReg.GetString(ctx, key); existing != "" {
		return existing, nil
	}
	d.appsMu.Lock()
	defer d.appsMu.Unlock()
	// Re-check after lock acquired.
	if existing, _ := d.configReg.GetString(ctx, key); existing != "" {
		return existing, nil
	}

	ownerAppID, err := d.configReg.GetString(ctx, "chat.app_id")
	if err != nil || ownerAppID == "" {
		return "", fmt.Errorf("chat.app_id is not set — pick an agent under /ui/alpha-agents first")
	}
	src, err := d.client.GetApp(ctx, ownerAppID)
	if err != nil {
		return "", fmt.Errorf("get owner app %s: %w", ownerAppID, err)
	}

	var raw map[string]any
	if len(src.Config) > 0 {
		if err := json.Unmarshal(src.Config, &raw); err != nil {
			return "", fmt.Errorf("decode owner config: %w", err)
		}
	} else {
		raw = map[string]any{}
	}
	helix, _ := raw["helix"].(map[string]any)
	if helix == nil {
		return "", fmt.Errorf("owner app %s has no helix config", ownerAppID)
	}
	// Rename so the clone is identifiable in /orgs/<org>/agents.
	if origName, _ := helix["name"].(string); origName != "" {
		helix["name"] = origName + " — " + string(workerID)
	} else {
		helix["name"] = "helix-org worker " + string(workerID)
	}
	asstsAny, _ := helix["assistants"].([]any)
	if len(asstsAny) == 0 {
		return "", fmt.Errorf("owner app %s has no assistants", ownerAppID)
	}
	asst, _ := asstsAny[0].(map[string]any)
	if asst == nil {
		return "", fmt.Errorf("owner app %s: assistant is not an object", ownerAppID)
	}

	// Rewrite (or add) the helix-org MCP entry to point at the worker.
	newMCPURL := strings.TrimRight(d.helixURL, "/") + "/api/v1/mcp/helix-org/workers/" + string(workerID) + "/mcp"
	mcps, _ := asst["mcps"].([]any)
	if len(mcps) == 0 {
		// Owner agent had no MCP — should never happen since the picker
		// always attaches one, but degrade gracefully by adding it.
		mcps = []any{map[string]any{
			"name":      "helix-org",
			"transport": "http",
			"url":       newMCPURL,
		}}
	} else {
		updated := false
		for _, mAny := range mcps {
			m, ok := mAny.(map[string]any)
			if !ok {
				continue
			}
			if m["name"] == "helix-org" {
				m["url"] = newMCPURL
				updated = true
			}
		}
		if !updated {
			mcps = append(mcps, map[string]any{
				"name":      "helix-org",
				"transport": "http",
				"url":       newMCPURL,
			})
		}
	}
	asst["mcps"] = mcps
	asstsAny[0] = asst
	helix["assistants"] = asstsAny
	raw["helix"] = helix

	body, err := json.Marshal(raw)
	if err != nil {
		return "", fmt.Errorf("encode clone config: %w", err)
	}
	created, err := d.client.CreateApp(ctx, helixclient.AppRequest{Config: body})
	if err != nil {
		return "", fmt.Errorf("create per-worker app: %w", err)
	}
	payload, _ := json.Marshal(created.ID)
	if err := d.configReg.Set(ctx, key, string(payload), domain.WorkerID("w-owner")); err != nil {
		return "", fmt.Errorf("persist per-worker app id: %w", err)
	}
	log.Info().Str("worker", string(workerID)).Str("app", created.ID).Str("mcp_url", newMCPURL).Msg("embedded spawner: provisioned per-worker agent app")
	return created.ID, nil
}

// publishActivation appends one Event to s-activations-<workerID>.
// Errors are logged and swallowed — a transient store hiccup must
// not abort the activation. Mirrors claude.publishActivationEvent.
func (d *embeddedSpawnerDeps) publishActivation(ctx context.Context, workerID domain.WorkerID, streamID domain.StreamID, body string) {
	if body == "" || d.orgStore == nil || d.newID == nil || d.now == nil {
		return
	}
	event, err := domain.NewMessageEvent(
		domain.EventID("e-"+d.newID()),
		streamID,
		workerID,
		domain.Message{From: string(workerID), Body: body},
		d.now(),
	)
	if err != nil {
		d.logger.Warn("activation event: build", "worker", workerID, "err", err)
		return
	}
	if err := d.orgStore.Events.Append(ctx, event); err != nil {
		d.logger.Warn("activation event: append", "worker", workerID, "err", err)
		return
	}
	if d.broadcaster != nil {
		d.broadcaster.Notify(streamID)
	}
}
