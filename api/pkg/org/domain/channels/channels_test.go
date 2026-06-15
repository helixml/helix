package channels

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

func membersOf(set Set, sid streaming.StreamID) []orgchart.WorkerID {
	var out []orgchart.WorkerID
	for k := range set.Members {
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

// TestRequired_OwnerObservesOwn: the manager-less human owner gets a
// self-observed transcript and (with no reports) no team stream.
// This is the "no special-casing by id" rule.
func TestRequired_OwnerObservesOwn(t *testing.T) {
	set := Required([]orgchart.Worker{human("w-owner")}, nil)

	actStream := activation.TranscriptID("w-owner")
	if _, ok := set.Channels[actStream]; !ok {
		t.Fatalf("owner transcript missing")
	}
	if got := membersOf(set, actStream); !eq(got, []orgchart.WorkerID{"w-owner"}) {
		t.Fatalf("owner activation observers = %v, want [w-owner]", got)
	}
	if _, ok := set.Channels[TeamStreamID("w-owner")]; ok {
		t.Fatalf("owner with no reports must NOT have a team stream")
	}
}

// TestRequired_AIObservedByManagers: an AI worker's transcript is
// subscribed by ALL its managers (many-to-many).
func TestRequired_AIObservedByManagers(t *testing.T) {
	workers := []orgchart.Worker{human("w-owner"), ai("w-jane"), ai("w-bob"), ai("w-li")}
	lines := []orgchart.ReportingLine{
		line("w-owner", "w-jane"),
		line("w-owner", "w-bob"),
		line("w-jane", "w-li"),
		line("w-bob", "w-li"), // w-li reports to two managers
	}
	set := Required(workers, lines)

	// w-li observed by both jane and bob.
	if got := membersOf(set, activation.TranscriptID("w-li")); !eq(got, []orgchart.WorkerID{"w-bob", "w-jane"}) {
		t.Fatalf("w-li activation observers = %v, want [w-bob w-jane]", got)
	}
	// w-li is a member of BOTH team streams.
	if got := membersOf(set, TeamStreamID("w-jane")); !eq(got, []orgchart.WorkerID{"w-jane", "w-li"}) {
		t.Fatalf("s-team-w-jane members = %v, want [w-jane w-li]", got)
	}
	if got := membersOf(set, TeamStreamID("w-bob")); !eq(got, []orgchart.WorkerID{"w-bob", "w-li"}) {
		t.Fatalf("s-team-w-bob members = %v, want [w-bob w-li]", got)
	}
}

// TestRequired_AINoSelfSubscribe: a manager-less AI is never subscribed
// to its own transcript (would re-trigger forever).
func TestRequired_AINoSelfSubscribe(t *testing.T) {
	set := Required([]orgchart.Worker{ai("w-rogue")}, nil)
	if got := membersOf(set, activation.TranscriptID("w-rogue")); len(got) != 0 {
		t.Fatalf("manager-less AI activation observers = %v, want none", got)
	}
}

// TestRequired_HumanWithManagerNoActivation: a human worker that has a
// manager gets NO transcript (only AIs and the root do).
func TestRequired_HumanWithManagerNoActivation(t *testing.T) {
	workers := []orgchart.Worker{human("w-owner"), human("w-renee")}
	set := Required(workers, []orgchart.ReportingLine{line("w-owner", "w-renee")})
	if _, ok := set.Channels[activation.TranscriptID("w-renee")]; ok {
		t.Fatalf("managed human must NOT get an transcript")
	}
	// And the owner now has a team stream containing renee.
	if got := membersOf(set, TeamStreamID("w-owner")); !eq(got, []orgchart.WorkerID{"w-owner", "w-renee"}) {
		t.Fatalf("s-team-w-owner members = %v, want [w-owner w-renee]", got)
	}
}

// TestRequired_DanglingLineIgnored: a reporting line that points at a
// non-existent worker is ignored rather than producing phantom
// subscriptions.
func TestRequired_DanglingLineIgnored(t *testing.T) {
	workers := []orgchart.Worker{human("w-owner")}
	set := Required(workers, []orgchart.ReportingLine{line("w-owner", "w-ghost")})
	if _, ok := set.Channels[TeamStreamID("w-owner")]; ok {
		t.Fatalf("team stream must not exist when the only report is a ghost")
	}
}
