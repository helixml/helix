package processor_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/processor"
	"github.com/helixml/helix/api/pkg/org/domain/streaming"
)

func newJSProc(t *testing.T, code string, outputs []processor.Output) processor.Processor {
	t.Helper()
	if outputs == nil {
		outputs = out("s-out")
	}
	p, err := processor.NewProcessor(
		"p-js", "JS", "s-in", processor.KindJS,
		cfg(t, map[string]string{"code": code}), outputs,
		"w-owner", time.Now(), "org-1",
	)
	if err != nil {
		t.Fatalf("NewProcessor: %v", err)
	}
	return p
}

func TestJSTransformBody(t *testing.T) {
	code := `function process(event) {
  event.body = "X:" + event.body;
  return event;
}`
	p := newJSProc(t, code, nil)
	res, err := p.Process(context.Background(), streaming.Message{Body: "hi", From: "a@b.com"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("want 1 result, got %d", len(res))
	}
	if res[0].Message.Body != "X:hi" {
		t.Errorf("body = %q", res[0].Message.Body)
	}
	if res[0].Message.From != "a@b.com" {
		t.Errorf("from should be preserved, got %q", res[0].Message.From)
	}
	if res[0].TopicID != "s-out" {
		t.Errorf("topic = %q", res[0].TopicID)
	}
}

func TestJSDrop(t *testing.T) {
	code := `function process(event) { return null; }`
	p := newJSProc(t, code, nil)
	res, err := p.Process(context.Background(), streaming.Message{Body: "x"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(res) != 0 {
		t.Fatalf("want drop (0 results), got %d", len(res))
	}
}

func TestJSRouteByLabel(t *testing.T) {
	code := `function process(event) {
  if (event.from.indexOf("vip") >= 0) return { out: "vip", event: event };
  return { out: "default", event: event };
}`
	outs := []processor.Output{
		{TopicID: "s-vip", Label: "vip"},
		{TopicID: "s-def", Label: "default"},
	}
	p := newJSProc(t, code, outs)

	res, err := p.Process(context.Background(), streaming.Message{From: "boss@vip.com", Body: "x"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(res) != 1 || res[0].TopicID != "s-vip" {
		t.Fatalf("vip route: %+v", res)
	}

	res, err = p.Process(context.Background(), streaming.Message{From: "joe@ex.com", Body: "x"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(res) != 1 || res[0].TopicID != "s-def" {
		t.Fatalf("default route: %+v", res)
	}
}

func TestJSFanOut(t *testing.T) {
	code := `function process(event) {
  return [
    { out: "a", event: { body: "A:" + event.body } },
    { out: "b", event: { body: "B:" + event.body } },
  ];
}`
	outs := []processor.Output{
		{TopicID: "s-a", Label: "a"},
		{TopicID: "s-b", Label: "b"},
	}
	p := newJSProc(t, code, outs)
	res, err := p.Process(context.Background(), streaming.Message{Body: "z"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(res) != 2 {
		t.Fatalf("want 2 results, got %d", len(res))
	}
	if res[0].TopicID != "s-a" || res[0].Message.Body != "A:z" {
		t.Errorf("first = %+v", res[0])
	}
	if res[1].TopicID != "s-b" || res[1].Message.Body != "B:z" {
		t.Errorf("second = %+v", res[1])
	}
}

func TestJSHTTPGetAndJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/lookup" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("email") != "a@b.com" {
			t.Errorf("query email = %q", r.URL.Query().Get("email"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"tier":"gold"}`)
	}))
	defer srv.Close()

	code := `function process(event) {
  const res = http.get(` + "`" + srv.URL + `/lookup` + "`" + `, {
    query: { email: event.from },
  });
  if (!res.ok) return null;
  const data = res.json();
  event.subject = "[" + data.tier + "] " + (event.subject || "");
  event.extra = { tier: data.tier };
  return event;
}`
	p := newJSProc(t, code, nil)
	res, err := p.Process(context.Background(), streaming.Message{From: "a@b.com", Subject: "hello"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if len(res) != 1 {
		t.Fatalf("want 1, got %d", len(res))
	}
	if res[0].Message.Subject != "[gold] hello" {
		t.Errorf("subject = %q", res[0].Message.Subject)
	}
	var extra map[string]any
	if err := json.Unmarshal(res[0].Message.Extra, &extra); err != nil {
		t.Fatalf("extra: %v", err)
	}
	if extra["tier"] != "gold" {
		t.Errorf("extra = %v", extra)
	}
}

func TestJSHTTPPostJSON(t *testing.T) {
	var gotBody string
	var gotCT string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCT = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"id":"1"}`)
	}))
	defer srv.Close()

	code := `function process(event) {
  const res = http.post(` + "`" + srv.URL + "`" + `, {
    json: { from: event.from, body: event.body },
  });
  event.body = "status=" + res.status + " ok=" + res.ok;
  return event;
}`
	p := newJSProc(t, code, nil)
	res, err := p.Process(context.Background(), streaming.Message{From: "x", Body: "y"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if !strings.Contains(gotCT, "application/json") {
		t.Errorf("content-type = %q", gotCT)
	}
	if !strings.Contains(gotBody, `"from":"x"`) {
		t.Errorf("body = %q", gotBody)
	}
	if res[0].Message.Body != "status=201 ok=true" {
		t.Errorf("result body = %q", res[0].Message.Body)
	}
}

func TestJSHTTPDelete(t *testing.T) {
	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	code := `function process(event) {
  const res = http.delete(` + "`" + srv.URL + "/item/1`" + `);
  event.body = String(res.status);
  return event;
}`
	p := newJSProc(t, code, nil)
	res, err := p.Process(context.Background(), streaming.Message{Body: "x"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if method != http.MethodDelete {
		t.Errorf("method = %q", method)
	}
	if res[0].Message.Body != "204" {
		t.Errorf("body = %q", res[0].Message.Body)
	}
}

func TestJSRejectsEmptyCode(t *testing.T) {
	_, err := processor.NewProcessor(
		"p-js", "JS", "s-in", processor.KindJS,
		cfg(t, map[string]string{"code": "   "}), out("s-out"),
		"", time.Now(), "org-1",
	)
	if err == nil {
		t.Fatal("want error for empty code")
	}
}

func TestJSRejectsMissingProcess(t *testing.T) {
	_, err := processor.NewProcessor(
		"p-js", "JS", "s-in", processor.KindJS,
		cfg(t, map[string]string{"code": "var x = 1;"}), out("s-out"),
		"", time.Now(), "org-1",
	)
	if err == nil {
		t.Fatal("want error when process is missing")
	}
}

func TestJSRejectsSyntaxError(t *testing.T) {
	_, err := processor.NewProcessor(
		"p-js", "JS", "s-in", processor.KindJS,
		cfg(t, map[string]string{"code": "function process( {"}), out("s-out"),
		"", time.Now(), "org-1",
	)
	if err == nil {
		t.Fatal("want error for bad syntax")
	}
}

func TestJSUsesCtxHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "pong")
	}))
	defer srv.Close()

	code := `function process(event, ctx) {
  const res = ctx.http.get(` + "`" + srv.URL + "`" + `);
  event.body = res.body;
  return event;
}`
	p := newJSProc(t, code, nil)
	res, err := p.Process(context.Background(), streaming.Message{Body: "x"})
	if err != nil {
		t.Fatalf("Process: %v", err)
	}
	if res[0].Message.Body != "pong" {
		t.Errorf("body = %q", res[0].Message.Body)
	}
}

func TestKindValuesIncludesJS(t *testing.T) {
	got := processor.KindValues()
	want := []processor.Kind{processor.KindTemplate, processor.KindTruncate, processor.KindFilter, processor.KindJS}
	if len(got) != len(want) {
		t.Fatalf("KindValues len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("KindValues[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
