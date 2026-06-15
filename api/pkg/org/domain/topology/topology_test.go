package topology

import (
	"sort"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

const orgID = "org-test"

func ai(id orgchart.WorkerID) orgchart.Worker {
	w, err := orgchart.NewAIWorker(id, "r-x", "#", orgID)
	if err != nil {
		panic(err)
	}
	return w
}

func human(id orgchart.WorkerID) orgchart.Worker {
	w, err := orgchart.NewHumanWorker(id, "r-x", "#", orgID)
	if err != nil {
		panic(err)
	}
	return w
}

func line(manager, report orgchart.WorkerID) orgchart.ReportingLine {
	l, err := orgchart.NewReportingLine(orgID, manager, report)
	if err != nil {
		panic(err)
	}
	return l
}

func membersOf(spec Spec, sid streaming.StreamID) []orgchart.WorkerID {
	var out []orgchart.WorkerID
	for k := range spec.Subs {
		if k.StreamID == sid {
			out = append(out, k.WorkerID)
		}
	}
	sort.Strings(out)
	return out
}

func eq(a, b []orgchart.WorkerID) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestDesiredTopology_OwnerObservesOwn: the manager-less human owner
// gets a self-observed activation stream and (with no reports) no team
// stream. This is the "no special-casing by id" rule.
func TestDesiredTopology_OwnerObservesOwn(t *testing.T) {
	spec := DesiredTopology([]orgchart.Worker{human("w-owner")}, nil)

	actStream := activation.StreamID("w-owner")
	if _, ok := spec.Streams[actStream]; !ok {
		t.Fatalf("owner activation stream missing")
	}
	if got := membersOf(spec, actStream); !eq(got, []orgchart.WorkerID{"w-owner"}) {
		t.Fatalf("owner activation observers = %v, want [w-owner]", got)
	}
	if _, ok := spec.Streams[TeamStreamID("w-owner")]; ok {
		t.Fatalf("owner with no reports must NOT have a team stream")
	}
}

// TestDesiredTopology_AIObservedByManagers: an AI worker's activation
// stream is subscribed by ALL its managers (many-to-many).
func TestDesiredTopology_AIObservedByManagers(t *testing.T) {
	workers := []orgchart.Worker{human("w-owner"), ai("w-jane"), ai("w-bob"), ai("w-li")}
	lines := []orgchart.ReportingLine{
		line("w-owner", "w-jane"),
		line("w-owner", "w-bob"),
		line("w-jane", "w-li"),
		line("w-bob", "w-li"), // w-li reports to two managers
	}
	spec := DesiredTopology(workers, lines)

	// w-li observed by both jane and bob.
	if got := membersOf(spec, activation.StreamID("w-li")); !eq(got, []orgchart.WorkerID{"w-bob", "w-jane"}) {
		t.Fatalf("w-li activation observers = %v, want [w-bob w-jane]", got)
	}
	// w-li is a member of BOTH team streams.
	if got := membersOf(spec, TeamStreamID("w-jane")); !eq(got, []orgchart.WorkerID{"w-jane", "w-li"}) {
		t.Fatalf("s-team-w-jane members = %v, want [w-jane w-li]", got)
	}
	if got := membersOf(spec, TeamStreamID("w-bob")); !eq(got, []orgchart.WorkerID{"w-bob", "w-li"}) {
		t.Fatalf("s-team-w-bob members = %v, want [w-bob w-li]", got)
	}
}

// TestDesiredTopology_AINoSelfSubscribe: a manager-less AI is never
// subscribed to its own activation stream (would re-trigger forever).
func TestDesiredTopology_AINoSelfSubscribe(t *testing.T) {
	spec := DesiredTopology([]orgchart.Worker{ai("w-rogue")}, nil)
	if got := membersOf(spec, activation.StreamID("w-rogue")); len(got) != 0 {
		t.Fatalf("manager-less AI activation observers = %v, want none", got)
	}
}

// TestDesiredTopology_HumanWithManagerNoActivation: a human worker that
// has a manager gets NO activation stream (only AIs and the root do).
func TestDesiredTopology_HumanWithManagerNoActivation(t *testing.T) {
	workers := []orgchart.Worker{human("w-owner"), human("w-renee")}
	spec := DesiredTopology(workers, []orgchart.ReportingLine{line("w-owner", "w-renee")})
	if _, ok := spec.Streams[activation.StreamID("w-renee")]; ok {
		t.Fatalf("managed human must NOT get an activation stream")
	}
	// And the owner now has a team stream containing renee.
	if got := membersOf(spec, TeamStreamID("w-owner")); !eq(got, []orgchart.WorkerID{"w-owner", "w-renee"}) {
		t.Fatalf("s-team-w-owner members = %v, want [w-owner w-renee]", got)
	}
}

// TestDesiredTopology_DanglingLineIgnored: a reporting line that points
// at a non-existent worker is ignored rather than producing phantom
// subscriptions.
func TestDesiredTopology_DanglingLineIgnored(t *testing.T) {
	workers := []orgchart.Worker{human("w-owner")}
	spec := DesiredTopology(workers, []orgchart.ReportingLine{line("w-owner", "w-ghost")})
	if _, ok := spec.Streams[TeamStreamID("w-owner")]; ok {
		t.Fatalf("team stream must not exist when the only report is a ghost")
	}
}
