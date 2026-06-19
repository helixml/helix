package processor_test

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

func newFilterProc(t *testing.T, outputs []processor.Output) processor.Processor {
	t.Helper()
	p, err := processor.NewProcessor(
		"p-filter", "Filter", "s-in", processor.KindFilter,
		nil, outputs, "", time.Now(), "org-1",
	)
	if err != nil {
		t.Fatalf("NewProcessor: %v", err)
	}
	return p
}

func TestFilterOneOutputKeepsMatch(t *testing.T) {
	// Predicate: keep when subject contains "invoice" (case-insensitive via lower).
	p := newFilterProc(t, []processor.Output{
		{TopicID: "s-bill", Match: `{{ if eq (lower .Message.subject) "invoice" }}yes{{ end }}`},
	})
	res, err := p.Process(streaming.Message{Subject: "Invoice", Body: "x"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(res) != 1 || res[0].TopicID != "s-bill" {
		t.Fatalf("want 1 result on s-bill, got %+v", res)
	}
	// Body passes through unchanged.
	if res[0].Message.Body != "x" {
		t.Errorf("body mutated: %q", res[0].Message.Body)
	}
}

func TestFilterOneOutputDropsNonMatch(t *testing.T) {
	p := newFilterProc(t, []processor.Output{
		{TopicID: "s-bill", Match: `{{ if eq (lower .Message.subject) "invoice" }}yes{{ end }}`},
	})
	res, err := p.Process(streaming.Message{Subject: "hello", Body: "x"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("want 0 results (dropped), got %+v", res)
	}
}

func TestFilterRoutesToMatchingOutputs(t *testing.T) {
	// Three outputs: vip (from contains @vip), billing (subject=invoice),
	// and a default (empty predicate) catching everything.
	p := newFilterProc(t, []processor.Output{
		{TopicID: "s-vip", Match: `{{ if hasSuffix "@vip.com" .Message.from }}1{{ end }}`},
		{TopicID: "s-bill", Match: `{{ if eq (lower .Message.subject) "invoice" }}1{{ end }}`},
		{TopicID: "s-gen", Match: ``}, // default / catch-all
	})

	// A VIP invoice → vip + bill + default.
	res, err := p.Process(streaming.Message{From: "boss@vip.com", Subject: "Invoice"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	got := topicSet(res)
	if !got["s-vip"] || !got["s-bill"] || !got["s-gen"] {
		t.Fatalf("VIP invoice routed to %v, want all three", keys(got))
	}

	// A plain message → only the default.
	res, _ = p.Process(streaming.Message{From: "joe@example.com", Subject: "hi"})
	got = topicSet(res)
	if len(got) != 1 || !got["s-gen"] {
		t.Fatalf("plain message routed to %v, want only s-gen", keys(got))
	}
}

func TestFilterEmptyPredicateIsUnconditional(t *testing.T) {
	p := newFilterProc(t, []processor.Output{{TopicID: "s-all", Match: ""}})
	res, _ := p.Process(streaming.Message{Body: "anything"})
	if len(res) != 1 || res[0].TopicID != "s-all" {
		t.Fatalf("empty predicate should always match, got %+v", res)
	}
}

func TestFilterMalformedPredicateRejected(t *testing.T) {
	_, err := processor.NewProcessor(
		"p-bad", "Bad", "s-in", processor.KindFilter,
		nil, []processor.Output{{TopicID: "s-x", Match: "{{ if "}}, "", time.Now(), "org-1",
	)
	if err == nil {
		t.Fatal("want error for malformed predicate, got nil")
	}
}

// A predicate referencing a function outside the fixed FuncMap is
// rejected at validation — the FuncMap is the only surface (no arbitrary
// user funcs in v1).
func TestFilterUnknownFuncRejected(t *testing.T) {
	_, err := processor.NewProcessor(
		"p-fn", "Fn", "s-in", processor.KindFilter,
		nil, []processor.Output{{TopicID: "s-x", Match: `{{ if regexMatch "x" .Message.from }}1{{ end }}`}},
		"", time.Now(), "org-1",
	)
	if err == nil {
		t.Fatal("want error for unknown function regexMatch, got nil")
	}
}

func topicSet(res []processor.Result) map[string]bool {
	m := map[string]bool{}
	for _, r := range res {
		m[string(r.TopicID)] = true
	}
	return m
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
