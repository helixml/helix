package channels

import (
	"sort"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/activation"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

const orgID = "org-test"

func ai(id orgchart.BotID) orgchart.Worker {
	w, err := orgchart.NewAIWorker(id, "r-x", "#", orgID)
	if err != nil {
		panic(err)
	}
	return w
}

func human(id orgchart.BotID) orgchart.Worker {
	w, err := orgchart.NewHumanWorker(id, "r-x", "#", orgID)
	if err != nil {
		panic(err)
	}
	return w
}

func line(manager, report orgchart.BotID) orgchart.ReportingLine {
	l, err := orgchart.NewReportingLine(orgID, manager, report)
	if err != nil {
		panic(err)
	}
	return l
}

func membersOf(set Set, sid streaming.TopicID) []orgchart.BotID {
	var out []orgchart.BotID
	for k := range set.Members {
		if k.TopicID == sid {
			out = append(out, k.WorkerID)
		}
	}
	sort.Strings(out)
	return out
}

func eq(a, b []orgchart.BotID) bool {
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

// TestRequired_ManagerlessRootHasUnobservedTranscript: a manager-less
// root worker gets a transcript (so its own chat turns have a home) with
// NO observers — never self-subscribed — and, with no reports, no team
// topic.
func TestRequired_ManagerlessRootHasUnobservedTranscript(t *testing.T) {
	set := Required([]orgchart.Worker{human("w-root")}, nil)

	tx := activation.TranscriptID("w-root")
	if _, ok := set.Channels[tx]; !ok {
		t.Fatalf("manager-less root transcript missing")
	}
	if got := membersOf(set, tx); len(got) != 0 {
		t.Fatalf("manager-less root transcript observers = %v, want none (no self-subscribe)", got)
	}
	if _, ok := set.Channels[TeamTopicID("w-root")]; ok {
		t.Fatalf("root with no reports must NOT have a team topic")
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
	if got := membersOf(set, activation.TranscriptID("w-li")); !eq(got, []orgchart.BotID{"w-bob", "w-jane"}) {
		t.Fatalf("w-li activation observers = %v, want [w-bob w-jane]", got)
	}
	// w-li is a member of BOTH team topics.
	if got := membersOf(set, TeamTopicID("w-jane")); !eq(got, []orgchart.BotID{"w-jane", "w-li"}) {
		t.Fatalf("s-team-w-jane members = %v, want [w-jane w-li]", got)
	}
	if got := membersOf(set, TeamTopicID("w-bob")); !eq(got, []orgchart.BotID{"w-bob", "w-li"}) {
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
	// And the owner now has a team topic containing renee.
	if got := membersOf(set, TeamTopicID("w-owner")); !eq(got, []orgchart.BotID{"w-owner", "w-renee"}) {
		t.Fatalf("s-team-w-owner members = %v, want [w-owner w-renee]", got)
	}
}

// TestRequired_DanglingLineIgnored: a reporting line that points at a
// non-existent worker is ignored rather than producing phantom
// subscriptions.
func TestRequired_DanglingLineIgnored(t *testing.T) {
	workers := []orgchart.Worker{human("w-owner")}
	set := Required(workers, []orgchart.ReportingLine{line("w-owner", "w-ghost")})
	if _, ok := set.Channels[TeamTopicID("w-owner")]; ok {
		t.Fatalf("team topic must not exist when the only report is a ghost")
	}
}
