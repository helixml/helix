package server

import (
	"context"
	"errors"
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

You are the Chief of Staff for this organization - the owner's right hand, here to support them and the team.

## First, reach the owner
On your first activation you do not yet know what this organization is for. Find the owner and ask them - do NOT guess, and do NOT just write the question into your own transcript (they will not see that).

1. Call ` + "`read_bots`" + ` and find the **person** - the node whose ` + "`kind`" + ` is ` + "`human`" + ` (its id looks like ` + "`h-…`" + `). On a new org there is exactly one: the owner who created it.
2. Use ` + "`ask_human`" + ` with that person's id to deliver the initial message through Helix notifications. Ask them, in one friendly message:
   - what this organization is for and what they want to accomplish,
   - who the key people are and what they are responsible for,
   - whether future messages should arrive in Helix or Slack,
   - and anything else you need to set it up well.

Keep it to a single, concise message - you can follow up once they reply.

If they choose Slack, ask them to install the org's Slack workspace from the Helix Slack integration settings, then ask for their Slack email and, if they prefer a shared channel, its channel name. Do not make them find opaque Slack IDs. Use ` + "`mint_credential`" + ` with provider ` + "`slack`" + `, then call Slack's ` + "`users.lookupByEmail`" + ` and ` + "`conversations.list`" + ` APIs to resolve the canonical user, channel, and team IDs. Use ` + "`set_human_contact`" + ` to set ` + "`preferred_contact=slack`" + `, ` + "`slack_user_id`" + `, and optionally ` + "`slack_channel_id`" + ` and ` + "`slack_team_id`" + `. Ask for IDs only if lookup fails. If they choose Helix, set ` + "`preferred_contact=helix`" + `. Do not claim Slack is ready until the workspace is installed and the contact update succeeds.

## Then set things up
When the owner answers, use what they told you to build the org: bring in assistant bots for the concrete pieces of work, give each a clear purpose, connect who works with whom, and subscribe them to the topics they need. Coordinate and keep things organized, and delegate the hands-on work to the assistants you bring in rather than doing it all yourself. Reach the owner again with ` + "`ask_human`" + ` whenever you need a decision or their input.

## Give bots the code they need
Bots only see git repositories attached to their Helix project. After you create a bot (and it has been activated so its project exists):

1. Call ` + "`list_repositories`" + ` to see every repo in this organization.
2. Call ` + "`attach_repository`" + ` with ` + "`bot_id`" + ` + ` + "`repo_id`" + ` (and ` + "`primary: true`" + ` when it should be their main working repo).
3. To **check** what a bot has attached: call ` + "`list_bot_repositories`" + ` with that ` + "`bot_id`" + `, or ` + "`get_bot`" + ` (the response includes a ` + "`repositories`" + ` array). Do **not** guess from memory of who attached what — the UI or another agent may have attached repos.
4. Use ` + "`detach_repository`" + ` to remove an attachment.

Without attached repos a coding bot has nothing to clone and cannot do real work.

## How to call your tools
Your tools are helix MCP tools (` + "`mcp__helix__…`" + `). They are live as soon as they appear on your bot's tool list — call them **directly** by name (e.g. ` + "`mcp__helix__list_bot_repositories`" + `). Do **not** wait for a "next activation", and do **not** rely on deferred-tool ` + "`ToolSearch`" + ` to find them. If ` + "`tools/list`" + ` / your tool list shows a name, invoke it now.

## Start, stop, and restart bots
Use ` + "`start_bot`" + ` to bring a bot's desktop online (also after create — activation provisions the project). Use ` + "`stop_bot`" + ` to shut the desktop down without losing the transcript. Use ` + "`restart_bot`" + ` when you need a brand-new session (e.g. after changing tools or repo attachments).`

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
	// Only create when the node is genuinely absent. A transient Get error
	// must NOT fall through to Create — that could mint a suffixed duplicate.
	_, err := s.botStore.Get(ctx, orgID, id)
	if err == nil {
		return nil // already represented
	}
	if !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("check human node for user %s: %w", user.ID, err)
	}
	identity := map[string]string{}
	if user.Email != "" {
		identity["email"] = user.Email
	}
	// The id is deterministic (h-<userID>) and used exactly. On a create race
	// the loser gets a conflict, which we treat as success below (the node
	// now exists).
	if _, err := s.bots.Create(ctx, orgID, bots.CreateParams{
		ID:          string(id),
		Name:        humanDisplayName(user),
		Content:     "Org member.",
		Kind:        orgchart.BotKindHuman,
		HelixUserID: user.ID,
		Identity:    identity,
	}); err != nil {
		if _, getErr := s.botStore.Get(ctx, orgID, id); getErr == nil {
			return nil // lost a create race; the node exists — fine
		}
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
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("remove human node for user %s: %w", userID, err)
	}
	return nil
}

// SeedChiefOfStaff creates the org's Chief of Staff bot as a top-level bot.
// Idempotent on the CoS id. There is deliberately NO reporting line to the
// creator: humans stay entirely out of the reporting graph — CoS reaches
// the owner via the inbox (Stage 2 delivery), not a manager/report edge.
//
// When CoS already exists, OwnerBotTools is unioned onto its tool list so
// upgrades (e.g. new repository tools) land without recreating the bot.
func (s *orgGraphSeeder) SeedChiefOfStaff(ctx context.Context, orgID string) error {
	if s == nil {
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
	// DeferActivation: a brand-new org has no bot runtime configured yet.
	// Activating now would provision CoS on the seed-time default
	// (claude_code/subscription/no-model → renders as gpt). The deferred bot
	// shows on the chart and is provisioned correctly once the operator sets
	// the default agent configuration. The id is used
	// exactly (`chief-of-staff`); a collision means already-seeded.
	if _, err := s.lifecycle.Create(ctx, orgID, lifecycle.CreateParams{
		ID:              string(chiefOfStaffBotID),
		Name:            "Chief of Staff",
		Content:         chiefOfStaffContent,
		Tools:           mcptools.OwnerBotTools(),
		DeferActivation: true,
	}); err != nil {
		if _, getErr := s.botStore.Get(ctx, orgID, chiefOfStaffBotID); getErr == nil {
			return nil // lost a seed race; CoS exists — fine
		}
		return fmt.Errorf("seed chief of staff: %w", err)
	}
	return nil
}

// ReconcileHumans makes the org's human nodes match its membership: ensure a
// node for every member, and remove human nodes whose user is no longer a
// member. Idempotent. This is the correctness backstop for the inline
// membership hooks — several membership-granting paths (notably OIDC login /
// domain auto-join, which run in a layer that can't reach the seeder) don't
// hook inline, and existing orgs pre-date the feature. Run on org bootstrap.
func (s *orgGraphSeeder) ReconcileHumans(ctx context.Context, orgID string, members []*types.User) error {
	if s == nil {
		return nil
	}
	want := make(map[orgchart.BotID]bool, len(members))
	for _, u := range members {
		if u == nil || u.ID == "" {
			continue
		}
		want[humanNodeID(u.ID)] = true
		if err := s.EnsureHumanNode(ctx, orgID, u); err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Str("user_id", u.ID).Msg("reconcile: ensure human node failed")
		}
	}
	all, err := s.botStore.List(ctx, orgID)
	if err != nil {
		return fmt.Errorf("reconcile humans: list bots: %w", err)
	}
	for _, b := range all {
		if b.IsHuman() && !want[b.ID] {
			if err := s.botStore.Delete(ctx, orgID, b.ID); err != nil && !errors.Is(err, store.ErrNotFound) {
				log.Warn().Err(err).Str("org_id", orgID).Str("bot", string(b.ID)).Msg("reconcile: remove orphan human node failed")
			}
		}
	}
	return nil
}

// listOrgMemberUsers loads the *types.User for every member of an org — the
// input ReconcileHumans reconciles against.
func listOrgMemberUsers(ctx context.Context, hs helixstore.Store, orgID string) ([]*types.User, error) {
	memberships, err := hs.ListOrganizationMemberships(ctx, &helixstore.ListOrganizationMembershipsQuery{OrganizationID: orgID})
	if err != nil {
		return nil, fmt.Errorf("list org memberships: %w", err)
	}
	users := make([]*types.User, 0, len(memberships))
	for _, m := range memberships {
		u, err := hs.GetUser(ctx, &helixstore.GetUserQuery{ID: m.UserID})
		if err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Str("user_id", m.UserID).Msg("reconcile: load member user failed")
			continue
		}
		users = append(users, u)
	}
	return users, nil
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
