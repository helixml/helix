package roles

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func fixedClock() time.Time { return time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC) }

// baseline is a small injected tool baseline so the union behaviour is
// testable without importing the tools package (which imports roles).
var baseline = []tool.Name{"managers", "reports"}

func newService(st *store.Store) *Roles {
	return New(Deps{
		Roles:     st.Roles,
		Now:       fixedClock,
		NewID:     func() string { return "id" },
		BaseTools: baseline,
	})
}

func TestRolesCreate_HappyPath(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()

	got, err := svc.Create(ctx, "org-test", CreateParams{
		ID:      "r-qa",
		Content: "# QA Engineer",
		Tools:   []tool.Name{"publish"},
		Streams: []streaming.StreamID{"s-general"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID != "r-qa" || got.Content != "# QA Engineer" {
		t.Fatalf("unexpected role: %+v", got)
	}
	if got.OrganizationID != "org-test" {
		t.Fatalf("org = %q", got.OrganizationID)
	}
	if !got.CreatedAt.Equal(fixedClock()) {
		t.Fatalf("CreatedAt = %v", got.CreatedAt)
	}
	// Caller's tool is preserved at head, baseline unioned and deduped.
	if len(got.Tools) != 3 || got.Tools[0] != "publish" || got.Tools[1] != "managers" || got.Tools[2] != "reports" {
		t.Fatalf("tools union wrong: %v", got.Tools)
	}
	if len(got.Streams) != 1 || got.Streams[0] != "s-general" {
		t.Fatalf("streams = %v", got.Streams)
	}

	stored, err := st.Roles.Get(ctx, "org-test", "r-qa")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if stored.Content != "# QA Engineer" {
		t.Fatalf("stored content = %q", stored.Content)
	}
}

func TestRolesCreate_EmptyToolsGetsBaseline(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	got, err := svc.Create(context.Background(), "org-test", CreateParams{ID: "r-x", Content: "# X"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	want := map[tool.Name]bool{"managers": true, "reports": true}
	for _, n := range got.Tools {
		delete(want, n)
	}
	if len(want) != 0 {
		t.Fatalf("baseline tools missing: %v (got %v)", want, got.Tools)
	}
}

func TestRolesCreate_AutoID(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	got, err := svc.Create(context.Background(), "org-test", CreateParams{Content: "# c"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.ID != "r-id" {
		t.Fatalf("auto id = %q, want r-id", got.ID)
	}
}

func TestRolesCreate_EmptyContentRejected(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	if _, err := svc.Create(context.Background(), "org-test", CreateParams{ID: "r-x"}); err == nil {
		t.Fatal("Create empty content: want error")
	}
}

// TestRolesUpdate_ContentOnlyPreservesToolsStreams pins the bug the
// shared service fixes: the old MCP update_role rebuilt the Role with
// only Content, wiping Tools and Streams. A content-only update must
// leave both intact.
func TestRolesUpdate_ContentOnlyPreservesToolsStreams(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()

	if _, err := svc.Create(ctx, "org-test", CreateParams{
		ID: "r-1", Content: "# old", Tools: []tool.Name{"publish"}, Streams: []streaming.StreamID{"s-a"},
	}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	newContent := "# new"
	got, err := svc.Update(ctx, "org-test", "r-1", UpdateParams{Content: &newContent})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Content != "# new" {
		t.Fatalf("content = %q", got.Content)
	}
	// Tools (publish + baseline) and streams survive the content-only patch.
	if len(got.Tools) == 0 {
		t.Fatalf("tools wiped: %v", got.Tools)
	}
	var hasPublish bool
	for _, n := range got.Tools {
		if n == "publish" {
			hasPublish = true
		}
	}
	if !hasPublish {
		t.Fatalf("publish tool lost on content update: %v", got.Tools)
	}
	if len(got.Streams) != 1 || got.Streams[0] != "s-a" {
		t.Fatalf("streams lost: %v", got.Streams)
	}
	if !got.UpdatedAt.Equal(fixedClock()) {
		t.Fatalf("UpdatedAt not bumped: %v", got.UpdatedAt)
	}
}

func TestRolesUpdate_PatchToolsAndStreams(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "org-test", CreateParams{ID: "r-1", Content: "# c", Tools: []tool.Name{"publish"}}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	newTools := []tool.Name{"subscribe"}
	newStreams := []streaming.StreamID{"s-x", "s-y"}
	got, err := svc.Update(ctx, "org-test", "r-1", UpdateParams{Tools: &newTools, Streams: &newStreams})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(got.Tools) != 1 || got.Tools[0] != "subscribe" {
		t.Fatalf("tools = %v, want [subscribe]", got.Tools)
	}
	if len(got.Streams) != 2 {
		t.Fatalf("streams = %v", got.Streams)
	}
	// Content untouched.
	if got.Content != "# c" {
		t.Fatalf("content changed: %q", got.Content)
	}
}

func TestRolesUpdate_NotFound(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	c := "# x"
	_, err := svc.Update(context.Background(), "org-test", "r-missing", UpdateParams{Content: &c})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
}

func TestRolesUpdate_OrgScoping(t *testing.T) {
	t.Parallel()
	st := memory.New()
	svc := newService(st)
	ctx := context.Background()
	if _, err := svc.Create(ctx, "org-a", CreateParams{ID: "r-1", Content: "# c"}); err != nil {
		t.Fatalf("Create: %v", err)
	}
	c := "# hacked"
	_, err := svc.Update(ctx, "org-b", "r-1", UpdateParams{Content: &c})
	if !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("cross-org update err = %v, want ErrNotFound", err)
	}
}
