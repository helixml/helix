package org

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/helixml/helix/api/pkg/types"
)

func interaction(t *testing.T, entries []entry) *types.Interaction {
	t.Helper()
	raw, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	return &types.Interaction{ResponseEntries: raw}
}

func TestFinalTextJoinsTextAfterLastToolCall(t *testing.T) {
	i := interaction(t, []entry{
		{Type: "text", Content: "<thinking>plan</thinking>Let me look."},
		{Type: "tool_call", ToolName: "chrome-devtools_navigate_page", Content: "**Tool Call: x**\nStatus: Completed\n\npage snapshot"},
		{Type: "text", Content: "<thinking>got it</thinking>OTP sent."},
		{Type: "text", Content: "**Next:** paste the code."},
	})
	got := finalText(i)
	want := "OTP sent.\n\n**Next:** paste the code."
	if got != want {
		t.Fatalf("finalText = %q, want %q", got, want)
	}
	if n := len(toolCalls(i)); n != 1 {
		t.Fatalf("toolCalls = %d, want 1", n)
	}
}

func TestFinalTextEndingOnToolCallFallsBackToLastText(t *testing.T) {
	i := interaction(t, []entry{
		{Type: "text", Content: "Working on it."},
		{Type: "tool_call", ToolName: "bash"},
	})
	if got := finalText(i); got != "Working on it." {
		t.Fatalf("finalText = %q", got)
	}
}

func TestLastSegmentCutsToolBlocks(t *testing.T) {
	raw := "<thinking>x</thinking>Checking.\n**Tool Call: chrome-devtools_take_snapshot**\nStatus: Completed\n\nuid=1 RootWebArea\n\nMarcus Webb manages Lunar Industries."
	if got := lastSegment(raw); got != "Marcus Webb manages Lunar Industries." {
		t.Fatalf("lastSegment = %q", got)
	}
	if got := lastSegment("plain answer"); got != "plain answer" {
		t.Fatalf("lastSegment(plain) = %q", got)
	}
}

func TestGradeTurn(t *testing.T) {
	two := 2
	e := evalExpect{
		Must:         [][]string{{"marcus webb"}, {"re:\\b(1043|#1043)\\b", "lunar"}},
		MustNot:      []string{"sandbox", "re:\\bsession\\b"},
		MaxSeconds:   30,
		MaxToolCalls: &two,
		ToolsRequire: []string{"navigate_page"},
		ToolsForbid:  []string{"bash"},
	}
	calls := []entry{{ToolName: "chrome-devtools_navigate_page"}, {ToolName: "chrome-devtools_take_snapshot"}}
	checks, failed := gradeTurn(e, "Marcus Webb manages Lunar Industries (#1043).", 12, calls)
	for k, v := range checks {
		if !v {
			t.Fatalf("check %s failed unexpectedly (%v)", k, failed)
		}
	}
	checks, failed = gradeTurn(e, "Your session is ready; I used the sandbox.", 45, append(calls, entry{ToolName: "bash"}))
	for _, k := range []string{"must", "must_not", "max_seconds", "max_tool_calls", "avoids:bash"} {
		if checks[k] {
			t.Fatalf("check %s passed, want fail (failed=%v)", k, failed)
		}
	}
}

func TestNormFieldIgnoresAPIZeroValuesAndOrder(t *testing.T) {
	spec := map[string]any{"mcp_servers": []any{"chrome-devtools"}, "tools": []any{}, "helix_skills": false}
	api := map[string]any{"mcp_servers": []any{"chrome-devtools"}, "tools": []any{}}
	if !reflect.DeepEqual(normField("instance_profile", spec), normField("instance_profile", api)) {
		t.Fatal("instance_profile with helix_skills:false should equal the API form that omits it")
	}
	if !reflect.DeepEqual(normField("tools", []any{"b", "a"}), normField("tools", []any{"a", "b"})) {
		t.Fatal("tools should compare order-insensitively")
	}
	if normField("provider", "") != nil {
		t.Fatal("empty string should normalise to nil")
	}
}

func TestLoadSuiteQuestionsArrayAndDefaults(t *testing.T) {
	dir := t.TempDir()
	q := filepath.Join(dir, "questions.json")
	if err := os.WriteFile(q, []byte(`[{"id":"q1","question":"Who?","must":[["marcus"]]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := loadSuite(q)
	if err != nil {
		t.Fatal(err)
	}
	if s.Name != "questions" || len(s.Cases) != 1 || len(s.Cases[0].Turns) != 1 || s.Cases[0].Turns[0].User != "Who?" {
		t.Fatalf("bare questions array not loaded as single-turn cases: %+v", s)
	}
	y := filepath.Join(dir, "suite.yaml")
	if err := os.WriteFile(y, []byte("name: s\nbot: b\ndefaults: {must_not: [sandbox], max_seconds: 60}\ncases:\n  - id: c\n    turns: [{user: hi, expect: {must_not: [secret]}}]\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err = loadSuite(y)
	if err != nil {
		t.Fatal(err)
	}
	e := s.Cases[0].Turns[0].Expect
	if e.MaxSeconds != 60 || !reflect.DeepEqual(e.MustNot, []string{"sandbox", "secret"}) {
		t.Fatalf("defaults not merged: %+v", e)
	}
}
