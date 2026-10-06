package org

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	orgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
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

func TestSpecDecodesIntoTypedRequests(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "p.md"), []byte("You are a bot."), 0o644); err != nil {
		t.Fatal(err)
	}
	f := filepath.Join(dir, "bot.yaml")
	if err := os.WriteFile(f, []byte("id: b-x\nname: x\ncontent_file: p.md\nmodel: m\npreserve_context: false\n"+
		"instance_profile: {mcp_servers: [chrome-devtools], tools: [], helix_skills: false}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	spec, err := loadSpec(f)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkSpecFields(spec); err != nil {
		t.Fatalf("valid spec rejected: %v", err)
	}
	var create orgapi.CreateBotRequest
	if err := decodeStrict(pick(spec, createFields), &create); err != nil || create.Content != "You are a bot." || create.Model != "m" {
		t.Fatalf("create request: %+v %v", create, err)
	}
	var update orgapi.UpdateBotRequest
	if err := decodeStrict(pick(spec, patchFields), &update); err != nil || update.InstanceProfile == nil || update.Name == nil || *update.Name != "x" {
		t.Fatalf("update request: %+v %v", update, err)
	}
	spec["modle"] = "typo"
	if err := checkSpecFields(spec); err == nil || !strings.Contains(err.Error(), "modle") {
		t.Fatalf("typo not reported: %v", err)
	}
	if err := decodeStrict(map[string]any{"tools": "not-a-list"}, &orgapi.UpdateBotRequest{}); err == nil {
		t.Fatal("wrong type not reported")
	}
	if err := decodeStrict(map[string]any{"mcp_servers": []any{}, "tool": []any{}}, &types.BotInstanceProfile{}); err == nil {
		t.Fatal("unknown instance_profile field not reported")
	}
}
