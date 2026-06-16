// Router tests (§9.4). The Router narrows a channel's subscriber set to
// the Worker(s) an inbound message should activate. BroadcastRouter
// (the default) returns everyone; FuzzyRouter scores candidates by
// keyword overlap with their identity/role text and returns the best
// match — but never drops to zero, falling back to all on a tie or low
// confidence. The registry maps the per-org `slack.router` value to an
// implementation, defaulting to broadcast.
package slack_test

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
	slacktransport "github.com/helixml/helix/api/pkg/org/infrastructure/transports/slack"
)

func candidates() []slacktransport.Candidate {
	return []slacktransport.Candidate{
		{WorkerID: "w-billing", Identity: "Handles billing, invoices, payments and refunds."},
		{WorkerID: "w-eng", Identity: "Software engineer. Fixes bugs, writes code, reviews pull requests."},
		{WorkerID: "w-sales", Identity: "Sales rep. Talks to prospects about pricing and demos."},
	}
}

func ids(cs []slacktransport.Candidate) []orgchart.WorkerID {
	out := make([]orgchart.WorkerID, len(cs))
	for i, c := range cs {
		out[i] = c.WorkerID
	}
	return out
}

func contains(set []orgchart.WorkerID, id orgchart.WorkerID) bool {
	for _, s := range set {
		if s == id {
			return true
		}
	}
	return false
}

func TestBroadcastRouter_ReturnsAll(t *testing.T) {
	r := slacktransport.BroadcastRouter{}
	in := slacktransport.Inbound{
		Subscribers: candidates(),
		Message:     streaming.Message{Body: "anything"},
	}
	got, err := r.Route(context.Background(), in)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("BroadcastRouter returned %d, want all 3", len(got))
	}
	for _, want := range ids(candidates()) {
		if !contains(got, want) {
			t.Errorf("missing %q from broadcast result", want)
		}
	}
}

func TestFuzzyRouter_PicksBestMatch(t *testing.T) {
	r := slacktransport.FuzzyRouter{}
	in := slacktransport.Inbound{
		Subscribers: candidates(),
		Message:     streaming.Message{Body: "I need a refund on my invoice, the payment was wrong"},
	}
	got, err := r.Route(context.Background(), in)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(got) != 1 || got[0] != "w-billing" {
		t.Fatalf("FuzzyRouter = %v, want [w-billing]", got)
	}
}

func TestFuzzyRouter_UsesThreadContext(t *testing.T) {
	r := slacktransport.FuzzyRouter{}
	in := slacktransport.Inbound{
		Subscribers: candidates(),
		Message:     streaming.Message{Body: "can you take a look?"},
		Thread: []streaming.Message{
			{Body: "There is a bug in the code, the pull requests are failing"},
		},
	}
	got, err := r.Route(context.Background(), in)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(got) != 1 || got[0] != "w-eng" {
		t.Fatalf("FuzzyRouter with thread = %v, want [w-eng]", got)
	}
}

func TestFuzzyRouter_LowConfidenceReturnsAll(t *testing.T) {
	r := slacktransport.FuzzyRouter{}
	in := slacktransport.Inbound{
		Subscribers: candidates(),
		Message:     streaming.Message{Body: "hello everyone good morning"},
	}
	got, err := r.Route(context.Background(), in)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("FuzzyRouter low-confidence = %v, want all 3 (never drops)", got)
	}
}

func TestFuzzyRouter_TieReturnsAllTied(t *testing.T) {
	r := slacktransport.FuzzyRouter{}
	cs := []slacktransport.Candidate{
		{WorkerID: "w-a", Identity: "refund refund"},
		{WorkerID: "w-b", Identity: "refund refund"},
		{WorkerID: "w-c", Identity: "totally unrelated topic"},
	}
	in := slacktransport.Inbound{
		Subscribers: cs,
		Message:     streaming.Message{Body: "refund please"},
	}
	got, err := r.Route(context.Background(), in)
	if err != nil {
		t.Fatalf("Route: %v", err)
	}
	if len(got) != 2 || !contains(got, "w-a") || !contains(got, "w-b") {
		t.Fatalf("FuzzyRouter tie = %v, want [w-a w-b]", got)
	}
}

func TestRouterRegistry_Resolves(t *testing.T) {
	cases := []struct {
		name string
		want string // type name fragment for the assertion
	}{
		{"broadcast", "broadcast"},
		{"fuzzy", "fuzzy"},
		{"", "broadcast"},
		{"nonsense", "broadcast"},
	}
	for _, tc := range cases {
		r := slacktransport.RouterFor(tc.name)
		if r == nil {
			t.Fatalf("RouterFor(%q) = nil", tc.name)
		}
		// Behavioural check: broadcast returns all; fuzzy narrows.
		in := slacktransport.Inbound{
			Subscribers: candidates(),
			Message:     streaming.Message{Body: "refund invoice payment billing"},
		}
		got, _ := r.Route(context.Background(), in)
		if tc.want == "broadcast" && len(got) != 3 {
			t.Errorf("RouterFor(%q) routed %d, want broadcast(3)", tc.name, len(got))
		}
		if tc.want == "fuzzy" && len(got) != 1 {
			t.Errorf("RouterFor(%q) routed %d, want fuzzy(1)", tc.name, len(got))
		}
	}
}
