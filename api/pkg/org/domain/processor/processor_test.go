package processor_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

func cfg(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	return b
}

func out(topic string) []processor.Output {
	return []processor.Output{{TopicID: streaming.TopicID(topic)}}
}

func newTemplateProc(t *testing.T, tmpl string) processor.Processor {
	t.Helper()
	p, err := processor.NewProcessor(
		"p-fmt", "Formatter", "s-in", processor.KindTemplate,
		cfg(t, map[string]string{"template": tmpl}), out("s-out"),
		"w-owner", time.Now(), "org-1",
	)
	if err != nil {
		t.Fatalf("NewProcessor: %v", err)
	}
	return p
}

func TestTemplateRendersBody(t *testing.T) {
	p := newTemplateProc(t, "BODY: {{ .Message.body }}")
	res, err := p.Process(streaming.Message{Body: "hello world"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("want 1 result, got %d", len(res))
	}
	if got := res[0].Message.Body; got != "BODY: hello world" {
		t.Errorf("body = %q, want %q", got, "BODY: hello world")
	}
	if res[0].TopicID != "s-out" {
		t.Errorf("topic = %q, want s-out", res[0].TopicID)
	}
	if res[0].Message.BodyContentType != "text/plain" {
		t.Errorf("content type = %q, want text/plain", res[0].Message.BodyContentType)
	}
}

func TestTemplateRendersFromAndSubject(t *testing.T) {
	p := newTemplateProc(t, "From {{ .Message.from }}: {{ .Message.subject }}")
	res, err := p.Process(streaming.Message{From: "alice@x.com", Subject: "Invoice #7"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := res[0].Message.Body; got != "From alice@x.com: Invoice #7" {
		t.Errorf("body = %q", got)
	}
}

func TestTemplateUnknownKeyRendersPlaceholder(t *testing.T) {
	// A genuinely-unknown field renders the standard Go placeholder
	// rather than erroring — surfacing a typo without dropping the
	// message. (Set-but-empty *known* fields render "", see below.)
	p := newTemplateProc(t, "[{{ .Message.nonexistent }}]")
	res, err := p.Process(streaming.Message{Body: "x"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := res[0].Message.Body; got != "[<no value>]" {
		t.Errorf("body = %q, want [<no value>]", got)
	}
}

func TestTemplateSetButEmptyFieldRendersEmpty(t *testing.T) {
	// An omitempty field that is unset is still a present key (renders "").
	p := newTemplateProc(t, "s=[{{ .Message.subject }}]")
	res, err := p.Process(streaming.Message{Body: "x"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := res[0].Message.Body; got != "s=[]" {
		t.Errorf("body = %q, want s=[]", got)
	}
}

func TestTemplateFuncMap(t *testing.T) {
	p := newTemplateProc(t, `{{ upper .Message.from }}|{{ default "anon" .Message.subject }}|{{ trunc 3 .Message.body }}`)
	res, err := p.Process(streaming.Message{From: "bob", Body: "abcdef"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if got := res[0].Message.Body; got != "BOB|anon|abc" {
		t.Errorf("body = %q, want BOB|anon|abc", got)
	}
}

func TestTemplateMalformedRejectedAtValidation(t *testing.T) {
	_, err := processor.NewProcessor(
		"p-bad", "Bad", "s-in", processor.KindTemplate,
		cfg(t, map[string]string{"template": "{{ .Message.body "}), out("s-out"),
		"", time.Now(), "org-1",
	)
	if err == nil {
		t.Fatal("want error for malformed template, got nil")
	}
}

func TestTemplateEmptyRejected(t *testing.T) {
	_, err := processor.NewProcessor(
		"p-empty", "Empty", "s-in", processor.KindTemplate,
		cfg(t, map[string]string{"template": "   "}), out("s-out"),
		"", time.Now(), "org-1",
	)
	if err == nil {
		t.Fatal("want error for empty template, got nil")
	}
}

func TestTemplateRequiresSingleOutput(t *testing.T) {
	_, err := processor.NewProcessor(
		"p-multi", "Multi", "s-in", processor.KindTemplate,
		cfg(t, map[string]string{"template": "x"}),
		[]processor.Output{{TopicID: "s-a"}, {TopicID: "s-b"}},
		"", time.Now(), "org-1",
	)
	if err == nil {
		t.Fatal("want error for >1 output on a transform, got nil")
	}
}

func TestUnknownKindRejected(t *testing.T) {
	_, err := processor.NewProcessor(
		"p-x", "X", "s-in", processor.Kind("nope"),
		nil, out("s-out"), "", time.Now(), "org-1",
	)
	if err == nil {
		t.Fatal("want error for unknown kind, got nil")
	}
}

func TestValidateRequiredFields(t *testing.T) {
	base := func() processor.Processor {
		return processor.Processor{
			ID: "p-1", OrganizationID: "org-1", Name: "n",
			InputTopicID: "s-in", Kind: processor.KindTemplate,
			Config:  cfg(t, map[string]string{"template": "x"}),
			Outputs: out("s-out"), CreatedAt: time.Now(),
		}
	}
	cases := map[string]func(p *processor.Processor){
		"empty id":     func(p *processor.Processor) { p.ID = "" },
		"empty org":    func(p *processor.Processor) { p.OrganizationID = "" },
		"empty name":   func(p *processor.Processor) { p.Name = "" },
		"empty input":  func(p *processor.Processor) { p.InputTopicID = "" },
		"no outputs":   func(p *processor.Processor) { p.Outputs = nil },
		"empty output": func(p *processor.Processor) { p.Outputs = []processor.Output{{TopicID: ""}} },
		"empty kind":   func(p *processor.Processor) { p.Kind = "" },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			p := base()
			mut(&p)
			if err := p.Validate(); err == nil {
				t.Errorf("%s: want error, got nil", name)
			}
		})
	}
}

func TestKindValuesCanonicalOrder(t *testing.T) {
	got := processor.KindValues()
	want := []processor.Kind{processor.KindTemplate, processor.KindTruncate, processor.KindFilter}
	if len(got) != len(want) {
		t.Fatalf("KindValues len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("KindValues[%d] = %q, want %q", i, got[i], want[i])
		}
	}
	// Returned slice is a copy — mutating it must not affect the canon.
	got[0] = "mutated"
	if processor.KindValues()[0] != processor.KindTemplate {
		t.Error("KindValues did not return a defensive copy")
	}
}
