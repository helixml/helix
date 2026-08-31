package helix

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
)

func bindAgentNode(t *testing.T, st *store.Store, id, orgID, projectID string) {
	t.Helper()
	node, err := orgchart.NewNode(orgchart.NodeID(id), id, nil, time.Now(), orgID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(context.Background(), node); err != nil {
		t.Fatal(err)
	}
	if projectID != "" {
		if err := SaveProject(context.Background(), st, orgID, orgchart.NodeID(id), projectID, "", ""); err != nil {
			t.Fatal(err)
		}
	}
}

func TestBoundAgentForProject(t *testing.T) {
	ctx := context.Background()
	st := memory.New()

	bindAgentNode(t, st, "w-owner", "org-1", "prj-home")
	bindAgentNode(t, st, "w-other", "org-1", "prj-somewhere-else")
	// Managed allowlist membership must NOT count as ownership: a manager bot
	// supervising prj-home gets nothing on prj-home.
	managed, err := orgchart.NewNode("w-manager", "w-manager", []tool.Name{"chat"}, time.Now(), "org-1")
	if err != nil {
		t.Fatal(err)
	}
	managed.ProjectIDs = []string{"prj-home"}
	if err := st.Nodes.Create(ctx, managed); err != nil {
		t.Fatal(err)
	}
	if err := SaveProject(ctx, st, "org-1", "w-manager", "prj-manager-home", "", ""); err != nil {
		t.Fatal(err)
	}
	// A human whose state happens to point at the project is still not an owner.
	human, err := orgchart.NewNode("h-1", "human", nil, time.Now(), "org-1")
	if err == nil {
		human.Kind = orgchart.NodeKindHuman
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, human); err != nil {
		t.Fatal(err)
	}
	if err := SaveProject(ctx, st, "org-1", "h-1", "prj-human", "", ""); err != nil {
		t.Fatal(err)
	}

	got, err := BoundAgentForProject(ctx, st, "org-1", "prj-home")
	if err != nil {
		t.Fatalf("expected owner: %v", err)
	}
	if got != "w-owner" {
		t.Fatalf("got %q", got)
	}

	// Zero owners.
	if _, err := BoundAgentForProject(ctx, st, "org-1", "prj-none"); !errors.Is(err, ErrNoBoundAgent) {
		t.Fatalf("expected ErrNoBoundAgent, got %v", err)
	}
	// Agent with runtime state pointing at another project.
	if _, err := BoundAgentForProject(ctx, st, "org-1", "prj-manager-home"); err != nil {
		t.Fatalf("manager owns its own home: %v", err)
	}
	// Humans never own, even with a matching home project.
	if _, err := BoundAgentForProject(ctx, st, "org-1", "prj-human"); !errors.Is(err, ErrNoBoundAgent) {
		t.Fatalf("humans must not own projects, got %v", err)
	}
	// Ambiguous ownership fails closed like zero owners.
	dupe := memory.New() // fresh store cannot reuse st (no delete helper); seed two owners
	bindAgentNode(t, dupe, "w-a", "org-1", "prj-dupe")
	bindAgentNode(t, dupe, "w-b", "org-1", "prj-dupe")
	if _, err := BoundAgentForProject(ctx, dupe, "org-1", "prj-dupe"); !errors.Is(err, ErrNoBoundAgent) {
		t.Fatalf("ambiguous ownership must fail closed, got %v", err)
	}
	// Empty inputs, nil store.
	if _, err := BoundAgentForProject(ctx, nil, "org-1", "prj-home"); !errors.Is(err, ErrNoBoundAgent) {
		t.Fatalf("nil store: %v", err)
	}
	if _, err := BoundAgentForProject(ctx, st, "org-1", ""); !errors.Is(err, ErrNoBoundAgent) {
		t.Fatalf("empty project: %v", err)
	}
}

func TestAgentToolNames(t *testing.T) {
	ctx := context.Background()
	st := memory.New()
	node, err := orgchart.NewNode("w-1", "w-1", []tool.Name{"chat", "get_secret", "list_secrets"}, time.Now(), "org-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := st.Nodes.Create(ctx, node); err != nil {
		t.Fatal(err)
	}
	names := AgentToolNames(ctx, st, "org-1", "w-1")
	if len(names) != 3 || names[0] != "chat" {
		t.Fatalf("got %v", names)
	}
	// Fired / unreachable node: empty surface, no error.
	if got := AgentToolNames(ctx, st, "org-1", "w-gone"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
	if got := AgentToolNames(ctx, nil, "org-1", "w-1"); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}
