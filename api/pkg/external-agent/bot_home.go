package external_agent

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/helixml/helix/api/pkg/hydra"
	"github.com/helixml/helix/api/pkg/system"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/rs/zerolog/log"
)

// BotHomeState is the control plane's record of an Org Bot home.
type BotHomeState string

const (
	// BotHomeNone: no home has been created under the bot-owned layout.
	BotHomeNone BotHomeState = ""
	// BotHomePending: a generation and runner were chosen; hydra may or may
	// not have created the home yet.
	BotHomePending BotHomeState = "pending"
	// BotHomeEstablished: hydra confirmed the home on SandboxID.
	BotHomeEstablished BotHomeState = "established"
	// BotHomeErasing: an explicit reset is erasing the home.
	BotHomeErasing BotHomeState = "erasing"
	// BotHomeEraseFailed: the erase did not finish; the data may still exist.
	BotHomeEraseFailed BotHomeState = "erase_failed"
)

// BotHomeRecord is the persisted home record plus the bot's session pointer.
type BotHomeRecord struct {
	State      BotHomeState
	Generation string
	SandboxID  string
	// Detail explains a failed erase, including where data may remain.
	Detail string
	// SessionID is the bot's current main session, if any.
	SessionID string
}

// BotHomeRegistry persists bot home records. It is implemented over the Org
// Bot runtime state, which the bot owns and which bot delete removes.
type BotHomeRegistry interface {
	LoadBotHome(ctx context.Context, orgID, botID string) (BotHomeRecord, error)
	SaveBotHome(ctx context.Context, orgID, botID string, rec BotHomeRecord) error
	// ListLiveBotHomes returns hydra.BotHomeKey values of every existing bot.
	ListLiveBotHomes(ctx context.Context) ([]string, error)
}

// SetBotHomeRegistry enables bot-owned homes for Org Bot main sessions.
func (h *HydraExecutor) SetBotHomeRegistry(registry BotHomeRegistry) {
	h.botHomes = registry
}

// botHomeBinding is a resolved home for one StartDesktop.
type botHomeBinding struct {
	orgID, botID string
	record       BotHomeRecord
	spec         hydra.BotHomeSpec
	relDir       string
}

// resolveBotHome decides whether this start mounts a bot home. Only an Org
// Bot's main session does: not bot instances (their own disk), not golden
// builds, and not another session of the bot while the pointer names a
// different one.
func (h *HydraExecutor) resolveBotHome(ctx context.Context, agent *types.DesktopAgent) (*botHomeBinding, error) {
	if h.botHomes == nil || agent.OrgWorkerID == "" || agent.GoldenBuild {
		return nil, nil
	}
	session, err := h.store.GetSession(ctx, agent.SessionID)
	if err != nil {
		return nil, fmt.Errorf("load session for bot home: %w", err)
	}
	if session.Metadata.BotInstance != nil || session.Metadata.SessionRole != "exploratory" {
		return nil, nil
	}
	orgID := session.OrganizationID
	if orgID == "" {
		return nil, fmt.Errorf("org bot session %s has no organization", agent.SessionID)
	}
	rec, err := h.botHomes.LoadBotHome(ctx, orgID, agent.OrgWorkerID)
	if err != nil {
		return nil, fmt.Errorf("load bot home record: %w", err)
	}
	if rec.SessionID != "" && rec.SessionID != agent.SessionID {
		return nil, nil
	}
	relDir, err := hydra.BotHomeRelDir(orgID, agent.OrgWorkerID)
	if err != nil {
		return nil, err
	}
	b := &botHomeBinding{orgID: orgID, botID: agent.OrgWorkerID, record: rec, relDir: relDir}
	b.spec = hydra.BotHomeSpec{OrganizationID: orgID, BotID: agent.OrgWorkerID, Generation: rec.Generation}
	switch rec.State {
	case BotHomeEstablished:
		b.spec.Expect = hydra.BotHomeExpectExisting
	case BotHomePending:
		b.spec.Expect = hydra.BotHomeExpectNew
	case BotHomeNone:
		b.spec.Expect = hydra.BotHomeExpectNew
		b.spec.Generation = "bh_" + system.GenerateID()
	case BotHomeErasing, BotHomeEraseFailed:
		return nil, fmt.Errorf("bot %s home reset did not complete (%s): %s; retry the reset before starting", agent.OrgWorkerID, rec.State, rec.Detail)
	default:
		return nil, fmt.Errorf("bot %s home has unknown state %q", agent.OrgWorkerID, rec.State)
	}
	if rec.State != BotHomeNone {
		if rec.SandboxID == "" || rec.Generation == "" {
			return nil, fmt.Errorf("bot %s home record is incomplete (state %s)", agent.OrgWorkerID, rec.State)
		}
		// The home lives on one runner. Starting anywhere else would mean an
		// empty home, so pin placement and let an unreachable runner fail.
		if agent.SandboxID != "" && agent.SandboxID != "local" && agent.SandboxID != rec.SandboxID {
			return nil, fmt.Errorf("bot %s home is on runner %s, but session %s asked for runner %s; runner loss recovery is not supported",
				agent.OrgWorkerID, rec.SandboxID, agent.SessionID, agent.SandboxID)
		}
		agent.SandboxID = rec.SandboxID
	}
	return b, nil
}

// reserve persists the chosen runner and generation before hydra is asked to
// create anything, so a crash after creation is resumed, never repeated with
// a new generation.
func (b *botHomeBinding) reserve(ctx context.Context, registry BotHomeRegistry, sandboxID string) error {
	if b.record.State != BotHomeNone {
		if sandboxID != b.record.SandboxID {
			return fmt.Errorf("bot %s home is on runner %s, which is not available (placement chose %q); runner loss recovery is not supported",
				b.botID, b.record.SandboxID, sandboxID)
		}
		return nil
	}
	if sandboxID == "" {
		return fmt.Errorf("bot %s home needs a runner", b.botID)
	}
	rec := BotHomeRecord{State: BotHomePending, Generation: b.spec.Generation, SandboxID: sandboxID}
	if err := registry.SaveBotHome(ctx, b.orgID, b.botID, rec); err != nil {
		return fmt.Errorf("reserve bot home: %w", err)
	}
	b.record = rec
	return nil
}

func (b *botHomeBinding) workspaceDir(base string) string {
	return filepath.Join(base, b.relDir)
}

// establish records hydra's confirmation. If it fails the record stays
// pending with the same generation, and the next start completes it: hydra
// treats a home with that generation as existing.
func (b *botHomeBinding) establish(ctx context.Context, registry BotHomeRegistry, result *hydra.BotHomeResult) {
	if result == nil {
		log.Error().Str("bot_home", hydra.BotHomeKey(b.orgID, b.botID)).Msg("Hydra did not report the bot home binding; it may predate bot homes")
		return
	}
	if b.record.State == BotHomeEstablished {
		return
	}
	rec := b.record
	rec.State = BotHomeEstablished
	if err := registry.SaveBotHome(ctx, b.orgID, b.botID, rec); err != nil {
		log.Error().Err(err).Str("bot_home", hydra.BotHomeKey(b.orgID, b.botID)).
			Msg("Failed to record bot home as established; it stays pending and the next start completes it")
	}
}

// ErrBotHomeUnavailable is returned when bot homes are not wired.
var ErrBotHomeUnavailable = errors.New("bot homes are not configured")

// EraseBotHome erases a bot's home on its runner. The record moves to
// erasing first and to erase_failed (naming where data may remain) on any
// failure, so an interrupted erase is never mistaken for an empty home.
// Callers must stop the bot's desktop first; hydra refuses while it runs.
func (h *HydraExecutor) EraseBotHome(ctx context.Context, orgID, botID string, mode hydra.BotHomeDestroyMode) (*hydra.DestroyBotHomeResponse, error) {
	if h.botHomes == nil {
		return nil, ErrBotHomeUnavailable
	}
	rec, err := h.botHomes.LoadBotHome(ctx, orgID, botID)
	if err != nil {
		return nil, fmt.Errorf("load bot home record: %w", err)
	}
	if rec.State == BotHomeNone {
		return &hydra.DestroyBotHomeResponse{}, nil
	}
	if rec.SandboxID == "" {
		return nil, fmt.Errorf("bot %s home record has no runner", botID)
	}
	if h.connman == nil {
		return nil, fmt.Errorf("connection manager not available")
	}
	erasing := rec
	erasing.State = BotHomeErasing
	erasing.Detail = fmt.Sprintf("%s of generation %s on runner %s started", mode, rec.Generation, rec.SandboxID)
	if err := h.botHomes.SaveBotHome(ctx, orgID, botID, erasing); err != nil {
		return nil, fmt.Errorf("record bot home erase: %w", err)
	}
	client := hydra.NewRevDialClient(h.connman, "hydra-"+rec.SandboxID)
	resp, err := client.DestroyBotHome(ctx, &hydra.DestroyBotHomeRequest{
		OrganizationID: orgID, BotID: botID, Generation: rec.Generation, Mode: mode,
	})
	if err != nil {
		failed := rec
		failed.State = BotHomeEraseFailed
		failed.Detail = fmt.Sprintf("%s of generation %s on runner %s failed; the home may still exist: %v", mode, rec.Generation, rec.SandboxID, err)
		if saveErr := h.botHomes.SaveBotHome(context.WithoutCancel(ctx), orgID, botID, failed); saveErr != nil {
			return nil, fmt.Errorf("erase bot home: %w (and recording the failure failed: %v)", err, saveErr)
		}
		return nil, fmt.Errorf("erase bot home: %w", err)
	}
	if err := h.botHomes.SaveBotHome(ctx, orgID, botID, BotHomeRecord{}); err != nil {
		return resp, fmt.Errorf("bot home erased on runner %s but clearing its record failed: %w", rec.SandboxID, err)
	}
	return resp, nil
}

// withLiveBotHomes adds the authoritative bot live set to a GC request. If
// the set cannot be read, bot homes are left out of this pass entirely.
func (h *HydraExecutor) withLiveBotHomes(ctx context.Context, req *hydra.GCReconcileRequest) *hydra.GCReconcileRequest {
	if h.botHomes == nil {
		return req
	}
	live, err := h.botHomes.ListLiveBotHomes(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("orphan reaper: cannot list live bot homes; skipping bot homes this pass")
		return req
	}
	out := *req
	out.BotHomesAuthoritative = true
	out.LiveBotHomes = live
	return &out
}
