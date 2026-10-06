package server

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/seedprompts"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

// The Chief of Staff seed prompt lives in domain/seedprompts so this
// bootstrap path and the reset-instructions API share one source of
// truth (pkg/org cannot import pkg/server without a cycle).
const chiefOfStaffContent = seedprompts.ChiefOfStaff

const chiefOfStaffBotID = seedprompts.ChiefOfStaffBotID

// orgGraphSeeder owns the per-org Chief of Staff bot.
type orgGraphSeeder struct {
	lifecycle *lifecycle.Service
	bots      *nodes.Nodes
	botStore  store.Nodes
}

// SeedChiefOfStaff creates the org's Chief of Staff bot as a top-level bot.
// Idempotent on the CoS id. When CoS already exists, OwnerBotTools is unioned
// onto its tool list so
// upgrades (e.g. new repository tools) land without recreating the bot.
func (s *orgGraphSeeder) SeedChiefOfStaff(ctx context.Context, orgID string) error {
	if s == nil || s.lifecycle == nil {
		return nil
	}
	deleted, err := s.lifecycle.ChiefOfStaffDeletionMarked(ctx, orgID)
	if err != nil {
		return err
	}
	if deleted {
		return nil
	}
	existing, err := s.botStore.Get(ctx, orgID, chiefOfStaffBotID)
	if err == nil {
		// Already seeded — backfill any new OwnerBotTools entries.
		if s.bots != nil {
			if _, attErr := s.bots.AttachTools(ctx, orgID, chiefOfStaffBotID, mcptools.OwnerBotTools()); attErr != nil {
				log.Warn().Err(attErr).Str("org_id", orgID).Msg("chief-of-staff tool backfill failed")
			}
		}
		_ = existing
		return nil
	}
	if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("check chief of staff: %w", err)
	}
	// Bootstrap has already provisioned the org service key, but activation
	// waits for the operator's runtime selection so the scaffold App is
	// configured before its first project/session.
	if _, err := s.lifecycle.Create(ctx, orgID, lifecycle.CreateParams{
		ID:              string(chiefOfStaffBotID),
		Name:            "Chief of Staff",
		Content:         chiefOfStaffContent,
		Tools:           mcptools.OwnerBotTools(),
		PreserveContext: true,
		DeferActivation: true,
	}); err != nil {
		if _, getErr := s.botStore.Get(ctx, orgID, chiefOfStaffBotID); getErr == nil {
			return nil // lost a seed race; CoS exists — fine
		}
		return fmt.Errorf("seed chief of staff: %w", err)
	}
	return nil
}
