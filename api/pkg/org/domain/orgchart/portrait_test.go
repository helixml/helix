package orgchart

import "testing"

func TestAllocatePortraitIndexResolvesReportedCollision(t *testing.T) {
	if PreferredPortraitIndex("chief-of-staff") != PreferredPortraitIndex("b-joke-writer") {
		t.Fatal("fixture no longer reproduces the reported collision")
	}
	chief := AllocatePortraitIndex("chief-of-staff", nil)
	jokeWriter := AllocatePortraitIndex("b-joke-writer", []int{chief})
	if chief == jokeWriter {
		t.Fatalf("Chief and Joke Writer both received portrait %d", chief)
	}
}

func TestAllocatePortraitIndexUsesAllPortraitsBeforeReuse(t *testing.T) {
	assigned := make([]int, 0, PortraitCount+1)
	for _, id := range []NodeID{"b-one", "b-two", "b-three", "b-four", "b-five", "b-six"} {
		assigned = append(assigned, AllocatePortraitIndex(id, assigned))
	}
	seen := map[int]bool{}
	for _, index := range assigned {
		seen[index] = true
	}
	if len(seen) != PortraitCount {
		t.Fatalf("first six assignments = %v, want six distinct portraits", assigned)
	}
	seventh := AllocatePortraitIndex("b-seven", assigned)
	if seventh < 0 || seventh >= PortraitCount {
		t.Fatalf("seventh assignment = %d", seventh)
	}
}
