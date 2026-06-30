// Tests for the get_bot_project + configure_bot_project MCP tools.
// These let the org owner read a Bot's helix project configuration
// (startup script today; skills/guidelines later once Helix's
// ProjectUpdateRequest plumbing exposes them via ProjectConfig) and
// write a partial patch.
//
// Pinned via a fake ProjectConfig — the runtime/helix implementation
// owns its own tests and runs against the in-proc Helix server; here we
// just exercise the tool's arg-parsing, error-mapping, and
// roundtrip-to-the-port plumbing.
//
// Regression discipline: every behaviour the README / chat docs promise
// is asserted explicitly. If someone refactors the port in the future,
// these tests break loud before the chat experience silently goes weird.
package mcptools_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	domainTool "github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/runtime"
	"github.com/helixml/helix/api/pkg/org/interfaces/mcptools"
)

// fakeProjectConfig is a recorder + scripter for ProjectConfig calls.
// Every test below threads exactly the responses it needs. The method
// names mirror the runtime.ProjectConfig port (unchanged by the bot
// rename — the runtime contract still says "Worker").
type fakeProjectConfig struct {
	mu     sync.Mutex
	get    func(orgID string, botID orgchart.BotID) (runtime.ProjectConfigSnapshot, error)
	patch  func(orgID string, botID orgchart.BotID, p runtime.ProjectConfigPatch) (runtime.ProjectConfigSnapshot, error)
	getN   int
	patchN int
}

func (f *fakeProjectConfig) GetWorkerProjectConfig(_ context.Context, orgID string, botID orgchart.BotID) (runtime.ProjectConfigSnapshot, error) {
	f.mu.Lock()
	f.getN++
	f.mu.Unlock()
	if f.get == nil {
		return runtime.ProjectConfigSnapshot{}, errors.New("fakeProjectConfig: get not stubbed")
	}
	return f.get(orgID, botID)
}

func (f *fakeProjectConfig) UpdateWorkerProjectConfig(_ context.Context, orgID string, botID orgchart.BotID, p runtime.ProjectConfigPatch) (runtime.ProjectConfigSnapshot, error) {
	f.mu.Lock()
	f.patchN++
	f.mu.Unlock()
	if f.patch == nil {
		return runtime.ProjectConfigSnapshot{}, errors.New("fakeProjectConfig: patch not stubbed")
	}
	return f.patch(orgID, botID, p)
}

// invokeProjectTool drives a tool's Invoke directly (skipping the MCP
// transport layer the broader builtins_test.go uses for end-to-end
// coverage). orgID + caller are set the way the MCP server would set
// them for an owner-attributed call. Named to avoid colliding with the
// package-level invokeTool helper.
func invokeProjectTool(t *testing.T, tl domainTool.Tool, args any) (json.RawMessage, error) {
	t.Helper()
	raw, err := json.Marshal(args)
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	inv := domainTool.Invocation{
		Args:   raw,
		Caller: fakeOwnerCaller{},
	}
	return tl.Invoke(context.Background(), inv)
}

// fakeOwnerCaller satisfies tool.Caller as the org owner — that's the
// bot allowed to read + patch helix project config. The MCP server
// normally produces this via the principal flow; the unit tests fake it
// directly.
type fakeOwnerCaller struct{}

func (fakeOwnerCaller) ID() string             { return "b-owner" }
func (fakeOwnerCaller) OrganizationID() string { return "org-test" }

// ---- get_bot_project -----------------------------------------------------

func TestGetBotProject_HappyPath(t *testing.T) {
	t.Parallel()
	fc := &fakeProjectConfig{
		get: func(orgID string, botID orgchart.BotID) (runtime.ProjectConfigSnapshot, error) {
			if orgID != "org-test" {
				t.Errorf("orgID = %q, want org-test", orgID)
			}
			if botID != "b-alice" {
				t.Errorf("botID = %q, want b-alice", botID)
			}
			return runtime.ProjectConfigSnapshot{
				ProjectID:     "prj_01abc",
				StartupScript: "#!/bin/bash\napt install -y gh",
			}, nil
		},
	}
	tl := mcptools.NewGetBotProject(mcptools.Deps{ProjectConfig: fc})

	out, err := invokeProjectTool(t, tl, map[string]string{"botId": "b-alice"})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	var got runtime.ProjectConfigSnapshot
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("unmarshal: %v (body=%s)", err, out)
	}
	if got.ProjectID != "prj_01abc" {
		t.Errorf("ProjectID = %q", got.ProjectID)
	}
	if !strings.Contains(got.StartupScript, "apt install -y gh") {
		t.Errorf("StartupScript = %q, want apt install line", got.StartupScript)
	}
	if fc.getN != 1 {
		t.Errorf("Get called %d times, want 1", fc.getN)
	}
}

func TestGetBotProject_MissingBotID(t *testing.T) {
	t.Parallel()
	tl := mcptools.NewGetBotProject(mcptools.Deps{ProjectConfig: &fakeProjectConfig{}})
	_, err := invokeProjectTool(t, tl, map[string]string{})
	if err == nil {
		t.Fatal("expected error on missing botId")
	}
	if !strings.Contains(err.Error(), "botId") {
		t.Errorf("err = %v, want mention of botId", err)
	}
}

func TestGetBotProject_UnsupportedRuntimeFriendlyError(t *testing.T) {
	t.Parallel()
	tl := mcptools.NewGetBotProject(mcptools.Deps{ProjectConfig: runtime.NoopProjectConfig{}})
	_, err := invokeProjectTool(t, tl, map[string]string{"botId": "b-alice"})
	if err == nil {
		t.Fatal("expected unsupported error")
	}
	if !errors.Is(err, runtime.ErrProjectConfigUnsupported) {
		t.Errorf("err = %v, want ErrProjectConfigUnsupported", err)
	}
}

// TestGetBotProject_NilDepsHasFriendlyError pins the safety net: callers
// wiring the tool without setting Deps.ProjectConfig (a NoopProjectConfig
// is the safe default — the registry hooks it up via DefaultDeps) get a
// clear error rather than a nil dereference.
func TestGetBotProject_NilDepsHasFriendlyError(t *testing.T) {
	t.Parallel()
	tl := mcptools.NewGetBotProject(mcptools.Deps{})
	_, err := invokeProjectTool(t, tl, map[string]string{"botId": "b-alice"})
	if err == nil {
		t.Fatal("expected error when ProjectConfig is nil")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "not wired") && !errors.Is(err, runtime.ErrProjectConfigUnsupported) {
		t.Errorf("err = %v, want a wiring-related message", err)
	}
}

// ---- configure_bot_project -----------------------------------------------

func TestConfigureBotProject_PatchStartupScript(t *testing.T) {
	t.Parallel()
	var gotPatch runtime.ProjectConfigPatch
	fc := &fakeProjectConfig{
		patch: func(orgID string, botID orgchart.BotID, p runtime.ProjectConfigPatch) (runtime.ProjectConfigSnapshot, error) {
			if orgID != "org-test" {
				t.Errorf("orgID = %q", orgID)
			}
			if botID != "b-alice" {
				t.Errorf("botID = %q", botID)
			}
			gotPatch = p
			return runtime.ProjectConfigSnapshot{
				ProjectID:     "prj_01abc",
				StartupScript: *p.StartupScript,
			}, nil
		},
	}
	tl := mcptools.NewConfigureBotProject(mcptools.Deps{ProjectConfig: fc})

	script := "#!/bin/bash\napt install -y gh\ngh auth status"
	out, err := invokeProjectTool(t, tl, map[string]any{
		"botId":         "b-alice",
		"startupScript": script,
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if gotPatch.StartupScript == nil || *gotPatch.StartupScript != script {
		t.Errorf("patch.StartupScript = %v, want pointer to script", gotPatch.StartupScript)
	}
	var snap runtime.ProjectConfigSnapshot
	if err := json.Unmarshal(out, &snap); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if snap.StartupScript != script {
		t.Errorf("returned StartupScript = %q, want the script we sent", snap.StartupScript)
	}
}

// TestConfigureBotProject_NoFieldsRejected pins that calling the tool
// with NO fields to patch is an error — otherwise an agent that
// mis-formats its args could silently no-op when it thinks it's
// configuring something. Forces the LLM to be explicit.
func TestConfigureBotProject_NoFieldsRejected(t *testing.T) {
	t.Parallel()
	tl := mcptools.NewConfigureBotProject(mcptools.Deps{ProjectConfig: &fakeProjectConfig{}})
	_, err := invokeProjectTool(t, tl, map[string]string{"botId": "b-alice"})
	if err == nil {
		t.Fatal("expected error on empty patch")
	}
	if !strings.Contains(err.Error(), "at least one") {
		t.Errorf("err = %v, want hint to set at least one patch field", err)
	}
}

// TestConfigureBotProject_EmptyStringStartupScriptAccepted pins that
// explicitly clearing the startup script (empty string) is a valid
// intent — distinct from "don't touch this field" (omitted). The tool
// transmits the empty string through the patch as a non-nil pointer so
// the runtime can act on it.
func TestConfigureBotProject_EmptyStringStartupScriptAccepted(t *testing.T) {
	t.Parallel()
	var gotPatch runtime.ProjectConfigPatch
	fc := &fakeProjectConfig{
		patch: func(_ string, _ orgchart.BotID, p runtime.ProjectConfigPatch) (runtime.ProjectConfigSnapshot, error) {
			gotPatch = p
			return runtime.ProjectConfigSnapshot{StartupScript: ""}, nil
		},
	}
	tl := mcptools.NewConfigureBotProject(mcptools.Deps{ProjectConfig: fc})
	_, err := invokeProjectTool(t, tl, map[string]any{
		"botId":         "b-alice",
		"startupScript": "",
	})
	if err != nil {
		t.Fatalf("Invoke: %v", err)
	}
	if gotPatch.StartupScript == nil {
		t.Fatal("expected non-nil pointer when empty string is set explicitly")
	}
	if *gotPatch.StartupScript != "" {
		t.Errorf("StartupScript = %q, want empty string", *gotPatch.StartupScript)
	}
}

// TestConfigureBotProject_PropagatesRuntimeErrors pins that failures
// from the underlying ProjectConfig (network glitch, permission denied,
// etc.) surface to the caller as tool errors rather than being
// swallowed.
func TestConfigureBotProject_PropagatesRuntimeErrors(t *testing.T) {
	t.Parallel()
	boom := errors.New("helix project update returned 500")
	fc := &fakeProjectConfig{
		patch: func(_ string, _ orgchart.BotID, _ runtime.ProjectConfigPatch) (runtime.ProjectConfigSnapshot, error) {
			return runtime.ProjectConfigSnapshot{}, boom
		},
	}
	tl := mcptools.NewConfigureBotProject(mcptools.Deps{ProjectConfig: fc})
	_, err := invokeProjectTool(t, tl, map[string]any{"botId": "b-alice", "startupScript": "x"})
	if !errors.Is(err, boom) {
		t.Errorf("err = %v, want wrapping %v", err, boom)
	}
}

// TestConfigureBotProject_NoFieldsDoesNotCallPort pins the
// short-circuit: when the patch is empty, the tool MUST NOT call the
// underlying ProjectConfig — otherwise a careless impl could update with
// all-zero fields and clobber state.
func TestConfigureBotProject_NoFieldsDoesNotCallPort(t *testing.T) {
	t.Parallel()
	fc := &fakeProjectConfig{
		patch: func(_ string, _ orgchart.BotID, _ runtime.ProjectConfigPatch) (runtime.ProjectConfigSnapshot, error) {
			t.Fatal("port should NOT be called on empty patch")
			return runtime.ProjectConfigSnapshot{}, nil
		},
	}
	tl := mcptools.NewConfigureBotProject(mcptools.Deps{ProjectConfig: fc})
	if _, err := invokeProjectTool(t, tl, map[string]string{"botId": "b-alice"}); err == nil {
		t.Fatal("expected error")
	}
	if fc.patchN != 0 {
		t.Errorf("patch called %d times; want 0", fc.patchN)
	}
}
