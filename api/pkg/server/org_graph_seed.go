package server

import (
	"context"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/helixml/helix/api/pkg/org/application/bots"
	"github.com/helixml/helix/api/pkg/org/application/lifecycle"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
	helixstore "github.com/helixml/helix/api/pkg/store"
	"github.com/helixml/helix/api/pkg/types"
)

// chiefOfStaffContent is the seed prompt for the Chief of Staff bot every
// new org gets. Moved here from the frontend (EditOrgWindow.tsx) so org
// bootstrap is server-owned and robust — the FE no longer creates it.
const chiefOfStaffContent = `# Chief of Staff

You are the Chief of Staff for this organization - the owner's right hand, here to support them and the team. When you first meet the owner, ask what this organization is for and what they want to accomplish. Then set things up: bring in assistant bots for the concrete pieces of work, give each a clear purpose, connect who works with whom, and subscribe them to the topics they need. Coordinate and keep things organized, and delegate the hands-on work to the assistants you bring in rather than doing it all yourself.`

const chiefOfStaffBotID orgchart.BotID = "chief-of-staff"

// orgGraphSeeder owns the membership-driven seeding of human nodes and the
// per-org Chief of Staff bot. Humans are never free-created: a human node
// is always the projection of a real org member (HelixUserID is the
// anchor). See design/2026-07-07-humans-in-the-org.md.
//
// It is assembled at the composition root (helix_org.go) with the same
// lifecycle + bots services the REST create path uses, and called from the
// org-lifecycle handlers (org create, membership add/remove).
type orgGraphSeeder struct {
	lifecycle *lifecycle.Service // creates the Chief of Staff bot (runs)
	bots      *bots.Bots         // creates human nodes (never run)
	botStore  store.Bots         // existence checks for idempotency
}

// EnsureHumanNode creates a human placeholder for an org member if one does
// not already exist. Idempotent — a second call for the same (org, user) is
// a no-op. The node id references the Helix user id; the display name is the
// user's name; email seeds the first identity channel. Best-effort at the
// call sites: a failure must not block the membership mutation.
func (s *orgGraphSeeder) EnsureHumanNode(ctx context.Context, orgID string, user *types.User) error {
	if s == nil || user == nil || user.ID == "" {
		return nil
	}
	id := humanNodeID(user.ID)
	if _, err := s.botStore.Get(ctx, orgID, id); err == nil {
		return nil // already represented
	}
	identity := map[string]string{}
	if user.Email != "" {
		identity["email"] = user.Email
	}
	_, err := s.bots.Create(ctx, orgID, bots.CreateParams{
		ID:          string(id),
		Name:        humanDisplayName(user),
		Content:     "Org member.",
		Kind:        orgchart.BotKindHuman,
		HelixUserID: user.ID,
		Identity:    identity,
	})
	if err != nil {
		return fmt.Errorf("ensure human node for user %s: %w", user.ID, err)
	}
	return nil
}

// RemoveHumanNode drops the member's human node when they leave the org.
// A missing node is not an error.
func (s *orgGraphSeeder) RemoveHumanNode(ctx context.Context, orgID, userID string) error {
	if s == nil || userID == "" {
		return nil
	}
	err := s.botStore.Delete(ctx, orgID, humanNodeID(userID))
	if err != nil && err != store.ErrNotFound {
		return fmt.Errorf("remove human node for user %s: %w", userID, err)
	}
	return nil
}

// SeedChiefOfStaff creates the org's Chief of Staff bot as a top-level bot.
// Idempotent on the CoS id. There is deliberately NO reporting line to the
// creator: humans stay entirely out of the reporting graph — CoS reaches
// the owner via the inbox (Stage 2 delivery), not a manager/report edge.
func (s *orgGraphSeeder) SeedChiefOfStaff(ctx context.Context, orgID string) error {
	if s == nil {
		return nil
	}
	if _, err := s.botStore.Get(ctx, orgID, chiefOfStaffBotID); err == nil {
		return nil // already seeded
	}
	_, err := s.lifecycle.Create(ctx, orgID, lifecycle.CreateParams{
		ID:      string(chiefOfStaffBotID),
		Name:    "Chief of Staff",
		Content: chiefOfStaffContent,
		Tools:   mcptools.OwnerBotTools(),
	})
	if err != nil {
		return fmt.Errorf("seed chief of staff: %w", err)
	}
	return nil
}

// ensureOrgHumanNode represents an org member as a human node, loading the
// user by id. Best-effort — logs and returns on any failure so it never
// blocks the membership mutation it hangs off. No-op when helix-org is off.
func (apiServer *HelixAPIServer) ensureOrgHumanNode(ctx context.Context, orgID, userID string) {
	if apiServer.orgSeeder == nil || userID == "" {
		return
	}
	user, err := apiServer.Store.GetUser(ctx, &helixstore.GetUserQuery{ID: userID})
	if err != nil {
		log.Warn().Err(err).Str("org_id", orgID).Str("user_id", userID).Msg("ensure human node: load user failed")
		return
	}
	if err := apiServer.orgSeeder.EnsureHumanNode(ctx, orgID, user); err != nil {
		log.Warn().Err(err).Str("org_id", orgID).Str("user_id", userID).Msg("ensure human node failed")
	}
}

// removeOrgHumanNode drops a member's human node when they leave. Best-effort.
func (apiServer *HelixAPIServer) removeOrgHumanNode(ctx context.Context, orgID, userID string) {
	if apiServer.orgSeeder == nil {
		return
	}
	if err := apiServer.orgSeeder.RemoveHumanNode(ctx, orgID, userID); err != nil {
		log.Warn().Err(err).Str("org_id", orgID).Str("user_id", userID).Msg("remove human node failed")
	}
}

// humanNodeID derives a human node's stable handle from the Helix user id.
// One human node per user per org; the id references the user id so the two
// are trivially correlated.
func humanNodeID(userID string) orgchart.BotID { return orgchart.BotID("h-" + userID) }

// humanDisplayName prefers the user's full name, falling back to username
// then email so a node always has a readable label.
func humanDisplayName(user *types.User) string {
	switch {
	case user.FullName != "":
		return user.FullName
	case user.Username != "":
		return user.Username
	default:
		return user.Email
	}
}
