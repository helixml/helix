// Tests for topicsRepo.Update on the in-memory store. Pins:
//   - Mutable fields (name, description, transport.kind/config)
//     are overwritten by Update.
//   - Immutable fields (ID, OrganizationID, CreatedBy, CreatedAt)
//     are preserved across Update — even when the caller passes a
//     domain.Topic that happens to have different values for them.
//   - (org_id, name) uniqueness is enforced on rename — same
//     constraint the gorm idx_topic_org_name index enforces.
//   - Update on a missing (orgID, id) returns store.ErrNotFound.
package memory_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/transport"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func newGithubTopic(t *testing.T, id, repo string, events []string) streaming.Topic {
	t.Helper()
	cfg, _ := json.Marshal(map[string]any{"repo": repo, "events": events})
	s, err := streaming.NewTopic(streaming.TopicID(id), id, "initial description", "w-owner", time.Now().UTC(),
		transport.Transport{Kind: transport.KindGitHub, Config: cfg}, "org-test")
	if err != nil {
		t.Fatalf("new topic: %v", err)
	}
	return s
}

// TestTopicsRepoUpdate_OverwritesMutableFields pins the happy
// path: Update replaces Name, Description and the entire Transport
// (kind + config). Used by the per-topic Edit form (PUT
// /topics/{id}) on the helix-org Topics detail page.
func TestTopicsRepoUpdate_OverwritesMutableFields(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ctx := context.Background()

	orig := newGithubTopic(t, "s-edit-me", "helixml/helix", []string{"issues"})
	if err := st.Topics.Create(ctx, orig); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Build the "updated" domain topic — same id + orgID + createdBy
	// (these are required by NewTopic's validator) but a new name,
	// description, and a beefed-up events list.
	newCfg, _ := json.Marshal(map[string]any{"repo": "helixml/helix", "events": []string{"issues", "pull_request"}})
	updated, err := streaming.NewTopic(
		orig.ID, "renamed", "edited description",
		orig.CreatedBy, orig.CreatedAt,
		transport.Transport{Kind: transport.KindGitHub, Config: newCfg}, orig.OrganizationID,
	)
	if err != nil {
		t.Fatalf("new updated topic: %v", err)
	}
	if err := st.Topics.Update(ctx, updated); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := st.Topics.Get(ctx, "org-test", orig.ID)
	if err != nil {
		t.Fatalf("get after update: %v", err)
	}
	if got.Name != "renamed" {
		t.Errorf("Name = %q, want renamed", got.Name)
	}
	if got.Description != "edited description" {
		t.Errorf("Description = %q", got.Description)
	}
	if string(got.Transport.Config) != string(newCfg) {
		t.Errorf("Config = %s, want %s", got.Transport.Config, newCfg)
	}
}

// TestTopicsRepoUpdate_PreservesImmutables pins that ID,
// OrganizationID, CreatedBy and CreatedAt CANNOT be changed via
// Update even if the caller passes a topic with different values
// for them. The handler is supposed to feed us a topic built from
// the existing row's immutables, but a future refactor could
// accidentally pass an attacker-controlled value; lock down the
// behaviour at the repo level.
func TestTopicsRepoUpdate_PreservesImmutables(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ctx := context.Background()

	origCreatedAt := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	cfg, _ := json.Marshal(map[string]any{"repo": "helixml/helix", "events": []string{"issues"}})
	orig, _ := streaming.NewTopic(
		streaming.TopicID("s-immutable"), "orig", "",
		"w-orig-author", origCreatedAt,
		transport.Transport{Kind: transport.KindGitHub, Config: cfg}, "org-test",
	)
	if err := st.Topics.Create(ctx, orig); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Build an "updated" topic that LIES about CreatedBy /
	// CreatedAt. The memory repo's Update only touches Name +
	// Description + Transport — ID/OrgID/CreatedBy/CreatedAt on the
	// existing row are preserved.
	tamperedCreatedAt := time.Date(2099, 12, 31, 23, 59, 59, 0, time.UTC)
	updated, _ := streaming.NewTopic(
		orig.ID, "renamed", "",
		"w-attacker", tamperedCreatedAt,
		transport.Transport{Kind: transport.KindGitHub, Config: cfg}, "org-test",
	)
	if err := st.Topics.Update(ctx, updated); err != nil {
		t.Fatalf("update: %v", err)
	}

	got, err := st.Topics.Get(ctx, "org-test", orig.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.CreatedBy != "w-orig-author" {
		t.Errorf("CreatedBy = %q, want unchanged %q", got.CreatedBy, "w-orig-author")
	}
	if !got.CreatedAt.Equal(origCreatedAt) {
		t.Errorf("CreatedAt = %v, want unchanged %v", got.CreatedAt, origCreatedAt)
	}
}

// TestTopicsRepoUpdate_ReturnsNotFoundOnMissingRow pins the
// error contract: trying to update a (orgID, id) that doesn't
// exist returns store.ErrNotFound, which the API handler maps to
// HTTP 404.
func TestTopicsRepoUpdate_ReturnsNotFoundOnMissingRow(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ctx := context.Background()

	s := newGithubTopic(t, "s-ghost", "x/y", []string{"issues"})
	// NB: we never Create this topic. Update should fail with
	// ErrNotFound.
	err := st.Topics.Update(ctx, s)
	if err == nil {
		t.Fatal("expected ErrNotFound, got nil")
	}
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("err = %v, want wrapping store.ErrNotFound", err)
	}
}

// TestTopicsRepoUpdate_EnforcesNameUniqueness pins the
// (org_id, name) uniqueness constraint on rename — same one the
// gorm idx_topic_org_name index enforces server-side. If an
// operator renames topic A to a name already in use by topic B
// in the same org, the Update must refuse.
func TestTopicsRepoUpdate_EnforcesNameUniqueness(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ctx := context.Background()

	a := newGithubTopic(t, "s-a", "x/y", []string{"issues"})
	a.Name = "topic-a"
	if err := st.Topics.Create(ctx, a); err != nil {
		t.Fatalf("create a: %v", err)
	}
	b := newGithubTopic(t, "s-b", "x/y", []string{"issues"})
	b.Name = "topic-b"
	if err := st.Topics.Create(ctx, b); err != nil {
		t.Fatalf("create b: %v", err)
	}

	// Try to rename topic-b to topic-a's name.
	cfg, _ := json.Marshal(map[string]any{"repo": "x/y", "events": []string{"issues"}})
	collide, _ := streaming.NewTopic(b.ID, "topic-a", "", b.CreatedBy, b.CreatedAt,
		transport.Transport{Kind: transport.KindGitHub, Config: cfg}, "org-test")
	if err := st.Topics.Update(ctx, collide); err == nil {
		t.Fatal("expected name uniqueness error, got nil")
	}

	// Confirm topic-b is unchanged after the failed update.
	stillB, err := st.Topics.Get(ctx, "org-test", b.ID)
	if err != nil {
		t.Fatalf("get b: %v", err)
	}
	if stillB.Name != "topic-b" {
		t.Errorf("topic-b name = %q after failed rename, want topic-b unchanged", stillB.Name)
	}
}

// TestTopicsRepoUpdate_AllowsRenamingToOwnCurrentName pins that
// updating a topic WITHOUT changing its name doesn't trip the
// uniqueness check on itself. (The naive implementation would
// loop over all rows in the org and match the current row's own
// name, treating Update with no rename as a duplicate. Pin that
// the implementation excludes the current row.)
func TestTopicsRepoUpdate_AllowsRenamingToOwnCurrentName(t *testing.T) {
	t.Parallel()
	st := memory.New()
	ctx := context.Background()

	s := newGithubTopic(t, "s-same-name", "x/y", []string{"issues"})
	s.Name = "stable"
	if err := st.Topics.Create(ctx, s); err != nil {
		t.Fatalf("create: %v", err)
	}

	cfg, _ := json.Marshal(map[string]any{"repo": "x/y", "events": []string{"issues", "pull_request"}})
	updated, _ := streaming.NewTopic(
		s.ID, "stable", "tweaked description",
		s.CreatedBy, s.CreatedAt,
		transport.Transport{Kind: transport.KindGitHub, Config: cfg}, "org-test",
	)
	if err := st.Topics.Update(ctx, updated); err != nil {
		t.Fatalf("update self-name: %v", err)
	}
}
