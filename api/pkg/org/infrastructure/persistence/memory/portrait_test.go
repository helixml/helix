package memory

import (
	"context"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
)

func TestNodePortraitAllocationIsStable(t *testing.T) {
	ctx := context.Background()
	store := New()
	ids := []orgchart.NodeID{"chief-of-staff", "b-joke-writer", "b-three", "b-four", "b-five", "b-six"}
	assigned := map[orgchart.NodeID]int{}
	for _, id := range ids {
		node, err := orgchart.NewNode(id, "# bot", nil, time.Now().UTC(), "org-test")
		if err != nil {
			t.Fatal(err)
		}
		if err := store.Nodes.Create(ctx, node); err != nil {
			t.Fatal(err)
		}
		created, err := store.Nodes.Get(ctx, "org-test", id)
		if err != nil || created.PortraitIndex == nil {
			t.Fatalf("get %s: portrait=%v err=%v", id, created.PortraitIndex, err)
		}
		assigned[id] = *created.PortraitIndex
	}
	seen := map[int]bool{}
	for _, index := range assigned {
		seen[index] = true
	}
	if len(seen) != orgchart.PortraitCount {
		t.Fatalf("first six assignments = %v", assigned)
	}

	chief, err := store.Nodes.Get(ctx, "org-test", "chief-of-staff")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Nodes.Update(ctx, chief.WithContent("updated")); err != nil {
		t.Fatal(err)
	}
	if err := store.Nodes.Delete(ctx, "org-test", "b-three"); err != nil {
		t.Fatal(err)
	}
	seventh, err := orgchart.NewNode("b-seven", "# seven", nil, time.Now().UTC(), "org-test")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Nodes.Create(ctx, seventh); err != nil {
		t.Fatal(err)
	}

	for _, id := range []orgchart.NodeID{"chief-of-staff", "b-joke-writer", "b-four", "b-five", "b-six"} {
		got, err := store.Nodes.Get(ctx, "org-test", id)
		if err != nil || got.PortraitIndex == nil || *got.PortraitIndex != assigned[id] {
			t.Fatalf("%s assignment changed: got=%v want=%d err=%v", id, got.PortraitIndex, assigned[id], err)
		}
	}
	gotSeventh, err := store.Nodes.Get(ctx, "org-test", "b-seven")
	if err != nil || gotSeventh.PortraitIndex == nil {
		t.Fatalf("seventh portrait=%v err=%v", gotSeventh.PortraitIndex, err)
	}
	if *gotSeventh.PortraitIndex != assigned["b-three"] {
		t.Fatalf("seventh portrait=%d, want freed portrait %d", *gotSeventh.PortraitIndex, assigned["b-three"])
	}
}
