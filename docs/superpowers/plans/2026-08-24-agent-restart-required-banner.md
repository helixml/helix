# Agent Restart-Required Banner Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Show a non-blocking banner on an org Bot's config page and chat panel when its running sandbox holds stale tool/instruction config, offering a user-initiated restart.

**Architecture:** A pure domain fingerprint over the restart-sensitive Node fields (`Tools` + `Content`). When a save changes that fingerprint, the org layer stamps the Docker container id of the bot's currently-live sandbox into `NodeRuntimeState`. Reads compare that stamp against the session's current `ContainerID` — a match means the same container is still running pre-edit config. Because Docker never reuses an id, every recreate path (stop/start, idle reap, crash reconcile, full restart) self-clears the flag with no clearing code anywhere, and nothing is written during a GET.

**Tech Stack:** Go 1.x (helix-org hexagonal layers: `domain` / `application` / `infrastructure` / `interfaces`), GORM, React 18 + TypeScript, MUI, react-query, vitest + React Testing Library, generated swagger API client.

**Spec:** `design/2026-08-24-agent-restart-required-banner.md`

## Global Constraints

- **Scope is org Bots only.** Helix Agents (Apps), project settings, and provider endpoints are out of scope.
- **The fingerprint covers exactly `Node.Tools` and `Node.Content`.** Never add `Name`, `PreserveContext`, `ProjectIDs`, runtime/model/provider/effort, secrets, or triggers — each of those already reaches a running sandbox without a restart, and including them turns the banner into noise.
- **No writes during a GET.** The stamp is written only on save.
- **Do not modify `api/pkg/external-agent/hydra_executor.go`.** `Metadata.ContainerID` is consumed as-is.
- **No new fields on `types.SessionMetadata` or any core Helix type.** The only new fields are `BotRuntimeInfo.RestartRequired` and `BotDTO.RestartRequired`.
- **The restart action is the full-session restart** (`restart-agent` → `ResetSession`), never a thread-preserving restart. A preserved transcript re-teaches the stale tool list.
- **Never restart automatically.** The banner offers; a human confirms.
- **Go rules (CLAUDE.md):** fail fast with `fmt.Errorf("...: %w", err)`; no fallbacks; structs not `map[string]interface{}`; comments only where they carry non-obvious reasoning.
- **Frontend rules (CLAUDE.md):** Lucide icons (`lucide-react`), never MUI icons, in new components; use the generated API client, never raw `fetch`; no `setTimeout` for async.
- **Commit format:** `type(scope): description`, subject ≤ 72 chars, imperative, no trailing period. The `commit-msg` hook rejects non-conforming messages.
- **Branch:** `feat/agent-restart-required-banner` (already created; the design doc is already committed on it). Never push to `main`.

---

### Task 1: Restart fingerprint (domain)

The pure function everything else keys off. No dependencies, no I/O.

**Files:**
- Create: `api/pkg/org/domain/orgchart/restart.go`
- Test: `api/pkg/org/domain/orgchart/restart_test.go`

**Interfaces:**
- Consumes: `orgchart.Node` (existing, `api/pkg/org/domain/orgchart/node.go:56`)
- Produces: `func RestartFingerprint(n Node) string` — hex-encoded sha256. Task 2 calls it.

- [ ] **Step 1: Write the failing test**

Create `api/pkg/org/domain/orgchart/restart_test.go`. Use the internal
`package orgchart`, matching the existing `node_test.go` and
`validate_test.go` in this directory:

```go
package orgchart

import (
	"testing"
	"time"

	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/stretchr/testify/require"
)

func fpNode(t *testing.T, content string, tools []tool.Name) Node {
	t.Helper()
	n, err := NewNode("b-fp", content, tools, time.Date(2026, 8, 24, 12, 0, 0, 0, time.UTC), "org-fp")
	require.NoError(t, err)
	return n
}

// Reordering a Bot's tool list is not a config change — the MCP surface
// is a set. Without sorting, dragging a chip in the tool picker would
// nag every operator to restart.
func TestRestartFingerprint_StableAcrossToolReordering(t *testing.T) {
	a := fpNode(t, "# bot", []tool.Name{"chat", "ask_human", "reports"})
	b := fpNode(t, "# bot", []tool.Name{"reports", "chat", "ask_human"})

	require.Equal(t, RestartFingerprint(a), RestartFingerprint(b))
}

func TestRestartFingerprint_ChangesOnToolAdd(t *testing.T) {
	before := fpNode(t, "# bot", []tool.Name{"chat"})
	after := fpNode(t, "# bot", []tool.Name{"chat", "get_secret"})

	require.NotEqual(t, RestartFingerprint(before), RestartFingerprint(after))
}

func TestRestartFingerprint_ChangesOnToolRemove(t *testing.T) {
	before := fpNode(t, "# bot", []tool.Name{"chat", "get_secret"})
	after := fpNode(t, "# bot", []tool.Name{"chat"})

	require.NotEqual(t, RestartFingerprint(before), RestartFingerprint(after))
}

func TestRestartFingerprint_ChangesOnContentEdit(t *testing.T) {
	before := fpNode(t, "# bot", []tool.Name{"chat"})
	after := fpNode(t, "# bot, now with feeling", []tool.Name{"chat"})

	require.NotEqual(t, RestartFingerprint(before), RestartFingerprint(after))
}

// These reach a running sandbox without a restart, so editing them must
// not raise the banner. This test is the guard against the fingerprint
// quietly widening into every Node field.
func TestRestartFingerprint_IgnoresFieldsThatHotApply(t *testing.T) {
	base := fpNode(t, "# bot", []tool.Name{"chat"})

	want := RestartFingerprint(base)
	require.Equal(t, want, RestartFingerprint(base.WithName("Chief of Staff")))
	require.Equal(t, want, RestartFingerprint(base.WithPreserveContext(true)))
	require.Equal(t, want, RestartFingerprint(base.WithProjectIDs([]string{"prj_01", "prj_02"})))
}

// Guards the per-name 0x00 terminator. Concatenated without it, tools
// ["a","b"] and ["ab"] both hash the bytes "ab" — so revoking "b" from a
// bot that also has "a" would look like no change at all.
func TestRestartFingerprint_ToolNamesCannotRunTogether(t *testing.T) {
	a := fpNode(t, "c", []tool.Name{"a", "b"})
	b := fpNode(t, "c", []tool.Name{"ab"})

	require.NotEqual(t, RestartFingerprint(a), RestartFingerprint(b))
}

// Guards the 0x01 domain separator between the tool list and the content.
// Concatenated without it, tools ["ab"] + content "c" and tools ["a"] +
// content "bc" both hash the bytes "abc".
func TestRestartFingerprint_ToolsCannotRunIntoContent(t *testing.T) {
	a := fpNode(t, "c", []tool.Name{"ab"})
	b := fpNode(t, "bc", []tool.Name{"a"})

	require.NotEqual(t, RestartFingerprint(a), RestartFingerprint(b))
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd api && go test ./pkg/org/domain/orgchart/ -run TestRestartFingerprint -v
```

Expected: FAIL — `undefined: orgchart.RestartFingerprint`.

- [ ] **Step 3: Write the implementation**

Create `api/pkg/org/domain/orgchart/restart.go`:

```go
package orgchart

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
)

// RestartFingerprint hashes exactly the Node config a running sandbox
// consumes at startup and that nothing hot-applies afterwards:
//
//   - Tools — the coding agent caches tools/list at MCP client init, and
//     nothing pushes an update (notifications/tools/list_changed is not
//     implemented on either side).
//   - Content — materialized into AGENTS.md/CLAUDE.md before the desktop
//     starts. SyncAgentProfile rewrites them in a live container, but only
//     during an activation, never on save.
//
// Deliberately excluded, because each already reaches a running sandbox
// without a restart: runtime/model/provider/effort (hot-switched through
// settings.json by the switch-agent path), worker secrets (fetched live
// through the get_secret MCP tool), and Name/PreserveContext/ProjectIDs/
// triggers (evaluated server-side per request or dispatch). Widening this
// set makes the banner fire on changes that need nothing, which teaches
// operators to ignore it.
//
// Tools are sorted so reordering is not a change. The 0x00 terminator per
// name and the 0x01 domain separator before Content stop a tool name and
// the instruction text from running together into the same bytes.
func RestartFingerprint(n Node) string {
	names := make([]string, 0, len(n.Tools))
	for _, t := range n.Tools {
		names = append(names, string(t))
	}
	sort.Strings(names)

	h := sha256.New()
	for _, name := range names {
		h.Write([]byte(name))
		h.Write([]byte{0x00})
	}
	h.Write([]byte{0x01})
	h.Write([]byte(n.Content))
	return hex.EncodeToString(h.Sum(nil))
}
```

- [ ] **Step 4: Run the test to verify it passes**

```bash
cd api && go test ./pkg/org/domain/orgchart/ -run TestRestartFingerprint -v
```

Expected: PASS, all seven tests.

- [ ] **Step 5: Commit**

```bash
git add api/pkg/org/domain/orgchart/restart.go api/pkg/org/domain/orgchart/restart_test.go
git commit -m "feat(org): add restart fingerprint over bot tools and content"
```

---

### Task 2: Fire a restart-required hook from the node mutation service

`Nodes` is the single home for node mutations — REST handlers and MCP tools both drive it — so one hook here covers every write path.

**Files:**
- Modify: `api/pkg/org/application/nodes/nodes.go` (struct at :46, `Deps` at :58, `New` at :84, `Update` at :192, `AttachTools` at :256, `DetachTools` at :281)
- Test: `api/pkg/org/application/nodes/restart_required_test.go` (create)

**Interfaces:**
- Consumes: `orgchart.RestartFingerprint(Node) string` from Task 1.
- Produces: `nodes.Deps.OnRestartRequired func(context.Context, string, orgchart.NodeID)` — called with `(ctx, orgID, nodeID)` after a successful write whose fingerprint changed. Task 4 wires it.

- [ ] **Step 1: Write the failing test**

Create `api/pkg/org/application/nodes/restart_required_test.go`:

```go
package nodes_test

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/application/nodes"
	"github.com/helixml/helix/api/pkg/org/domain/orgchart"
	"github.com/helixml/helix/api/pkg/org/domain/store"
	"github.com/helixml/helix/api/pkg/org/domain/tool"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

// restartSpy records every (orgID, nodeID) the service reports as needing
// a sandbox restart.
type restartSpy struct{ calls []orgchart.NodeID }

func (r *restartSpy) hook() func(context.Context, string, orgchart.NodeID) {
	return func(_ context.Context, _ string, id orgchart.NodeID) {
		r.calls = append(r.calls, id)
	}
}

func restartSvc(st *store.Store, spy *restartSpy) *nodes.Nodes {
	return nodes.New(nodes.Deps{
		Nodes:             st.Nodes,
		Now:               at,
		BaseTools:         []tool.Name{"chat"},
		KnownTools:        func() map[tool.Name]bool { return live },
		OnRestartRequired: spy.hook(),
	})
}

func ptrTools(t []tool.Name) *[]tool.Name { return &t }
func ptrStr(s string) *string             { return &s }
func ptrBool(b bool) *bool                { return &b }

func TestUpdate_FiresRestartRequiredOnToolChange(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-one", []tool.Name{"chat"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).Update(context.Background(), org, "b-one", nodes.UpdateParams{
		Tools: ptrTools([]tool.Name{"chat", "get_secret"}),
	})
	require.NoError(t, err)

	require.Equal(t, []orgchart.NodeID{"b-one"}, spy.calls)
}

func TestUpdate_FiresRestartRequiredOnContentChange(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-two", []tool.Name{"chat"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).Update(context.Background(), org, "b-two", nodes.UpdateParams{
		Content: ptrStr("# rewritten instructions"),
	})
	require.NoError(t, err)

	require.Equal(t, []orgchart.NodeID{"b-two"}, spy.calls)
}

// The whole point of the narrow fingerprint: edits that already reach a
// running sandbox must not nag.
func TestUpdate_SilentOnHotApplyingFields(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-three", []tool.Name{"chat"})
	spy := &restartSpy{}
	svc := restartSvc(st, spy)

	_, err := svc.Update(context.Background(), org, "b-three", nodes.UpdateParams{
		Name: ptrStr("Chief of Staff"),
	})
	require.NoError(t, err)

	_, err = svc.Update(context.Background(), org, "b-three", nodes.UpdateParams{
		PreserveContext: ptrBool(true),
	})
	require.NoError(t, err)

	_, err = svc.Update(context.Background(), org, "b-three", nodes.UpdateParams{
		ProjectIDs: &[]string{"prj_01"},
	})
	require.NoError(t, err)

	require.Empty(t, spy.calls)
}

// Re-submitting the same tool list (the UI sends the whole array on every
// save) is not a change.
func TestUpdate_SilentOnNoOpToolResubmit(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-four", []tool.Name{"chat", "get_secret"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).Update(context.Background(), org, "b-four", nodes.UpdateParams{
		Tools: ptrTools([]tool.Name{"get_secret", "chat"}),
	})
	require.NoError(t, err)

	require.Empty(t, spy.calls)
}

func TestAttachTools_FiresRestartRequired(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-five", []tool.Name{"chat"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).AttachTools(context.Background(), org, "b-five", []tool.Name{"get_secret"})
	require.NoError(t, err)

	require.Equal(t, []orgchart.NodeID{"b-five"}, spy.calls)
}

func TestAttachTools_SilentWhenNothingAdded(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-six", []tool.Name{"chat", "get_secret"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).AttachTools(context.Background(), org, "b-six", []tool.Name{"get_secret"})
	require.NoError(t, err)

	require.Empty(t, spy.calls)
}

func TestDetachTools_FiresRestartRequired(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-seven", []tool.Name{"chat", "get_secret"})
	spy := &restartSpy{}

	_, err := restartSvc(st, spy).DetachTools(context.Background(), org, "b-seven", []tool.Name{"get_secret"})
	require.NoError(t, err)

	require.Equal(t, []orgchart.NodeID{"b-seven"}, spy.calls)
}

// A nil hook is the standalone/test wiring. It must not panic.
func TestUpdate_NilHookIsSafe(t *testing.T) {
	st := memory.New()
	seed(t, st, "b-eight", []tool.Name{"chat"})
	svc := nodes.New(nodes.Deps{Nodes: st.Nodes, Now: at, BaseTools: []tool.Name{"chat"}})

	_, err := svc.Update(context.Background(), org, "b-eight", nodes.UpdateParams{
		Tools: ptrTools([]tool.Name{"chat", "get_secret"}),
	})
	require.NoError(t, err)
}
```

Note: `org`, `at`, `seed`, and `live` are package-level helpers already defined in `reconcile_tools_test.go` in this same `nodes_test` package — do not redeclare them.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd api && go test ./pkg/org/application/nodes/ -run 'TestUpdate_|TestAttachTools_|TestDetachTools_' -v
```

Expected: FAIL to compile — `unknown field OnRestartRequired in struct literal of type nodes.Deps`.

- [ ] **Step 3: Write the implementation**

In `api/pkg/org/application/nodes/nodes.go`, add the field to the `Nodes` struct (after `onToolsChanged`):

```go
	onToolsChanged    func(context.Context, string)
	onRestartRequired func(context.Context, string, orgchart.NodeID)
```

Add to `Deps` (after `OnToolsChanged`):

```go
	OnToolsChanged func(context.Context, string)
	// OnRestartRequired fires after a write that changed the Node's
	// restart fingerprint — the config a running sandbox reads once at
	// startup and never re-reads. The composition root uses it to stamp
	// which sandbox container is now stale. nil disables the signal.
	OnRestartRequired func(context.Context, string, orgchart.NodeID)
```

Add to `New`'s returned struct (after `onToolsChanged`):

```go
		onRestartRequired: deps.OnRestartRequired,
```

Add the notifier next to `notifyToolsChanged` (:315):

```go
func (s *Nodes) notifyRestartRequired(ctx context.Context, orgID string, id orgchart.NodeID) {
	if s.onRestartRequired != nil {
		s.onRestartRequired(ctx, orgID, id)
	}
}
```

In `Update` (:192), replace the post-write notify block:

```go
	if err := s.nodes.Update(ctx, updated); err != nil {
		return orgchart.Node{}, err
	}
	if p.Tools != nil && !sameToolList(existing.Tools, updated.Tools) {
		s.notifyToolsChanged(ctx, updated.AgentID)
	}
	if orgchart.RestartFingerprint(existing) != orgchart.RestartFingerprint(updated) {
		s.notifyRestartRequired(ctx, orgID, updated.ID)
	}
	return updated, nil
```

In `AttachTools` (:256) and `DetachTools` (:281), add the same fingerprint check immediately after each existing `s.notifyToolsChanged(ctx, updated.AgentID)` call:

```go
	s.notifyToolsChanged(ctx, updated.AgentID)
	if orgchart.RestartFingerprint(existing) != orgchart.RestartFingerprint(updated) {
		s.notifyRestartRequired(ctx, orgID, updated.ID)
	}
	return updated, nil
```

Both already early-return when nothing changed, so the check is belt-and-braces — but it keeps all three paths reading identically, so a future edit to one cannot silently diverge.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd api && go test ./pkg/org/application/nodes/ -v
```

Expected: PASS, including the pre-existing reconcile and validate tests.

- [ ] **Step 5: Commit**

```bash
git add api/pkg/org/application/nodes/nodes.go api/pkg/org/application/nodes/restart_required_test.go
git commit -m "feat(org): fire restart-required hook when bot fingerprint changes"
```

---

### Task 3: Persist the stale-container stamp

**Files:**
- Modify: `api/pkg/org/infrastructure/runtime/helix/state.go` (`WorkerState` at :31, key consts at :38, `LoadState` at :48)
- Test: `api/pkg/org/infrastructure/runtime/helix/restart_state_test.go` (create)

**Interfaces:**
- Consumes: `store.NodeRuntimeState` (existing: `Get(ctx, orgID, nodeID, backend) (map[string]string, error)`, `Set(ctx, orgID, nodeID, backend, key, value) error`).
- Produces:
  - `WorkerState.RestartRequiredContainer string`
  - `func SaveRestartRequiredContainer(ctx context.Context, st *store.Store, orgID string, workerID orgchart.NodeID, containerID string) error`

  Task 4 uses both.

- [ ] **Step 1: Write the failing test**

Create `api/pkg/org/infrastructure/runtime/helix/restart_state_test.go`:

```go
package helix

import (
	"context"
	"testing"

	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/stretchr/testify/require"
)

func TestSaveRestartRequiredContainer_RoundTrips(t *testing.T) {
	ctx := context.Background()
	st := memory.New()

	require.NoError(t, SaveRestartRequiredContainer(ctx, st, "org-rr", "b-rr", "3f9a1c2b4d5e"))

	state, err := LoadState(ctx, st, "org-rr", "b-rr")
	require.NoError(t, err)
	require.Equal(t, "3f9a1c2b4d5e", state.RestartRequiredContainer)
}

// A bot whose sandbox is stopped has no container id. Persisting the
// empty value is the "no banner" case and must overwrite a previous
// stamp rather than being skipped.
func TestSaveRestartRequiredContainer_EmptyOverwrites(t *testing.T) {
	ctx := context.Background()
	st := memory.New()

	require.NoError(t, SaveRestartRequiredContainer(ctx, st, "org-rr", "b-rr", "3f9a1c2b4d5e"))
	require.NoError(t, SaveRestartRequiredContainer(ctx, st, "org-rr", "b-rr", ""))

	state, err := LoadState(ctx, st, "org-rr", "b-rr")
	require.NoError(t, err)
	require.Empty(t, state.RestartRequiredContainer)
}

func TestLoadState_NoStampIsEmpty(t *testing.T) {
	ctx := context.Background()
	st := memory.New()

	state, err := LoadState(ctx, st, "org-rr", "b-never-stamped")
	require.NoError(t, err)
	require.Empty(t, state.RestartRequiredContainer)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd api && go test ./pkg/org/infrastructure/runtime/helix/ -run TestSaveRestartRequiredContainer -v
```

Expected: FAIL — `undefined: SaveRestartRequiredContainer`.

- [ ] **Step 3: Write the implementation**

In `api/pkg/org/infrastructure/runtime/helix/state.go`, add the field to `WorkerState`:

```go
	SessionID    string
	HiringUserID string
	// RestartRequiredContainer is the Docker container id of the Worker's
	// sandbox at the moment a restart-sensitive config change was saved.
	// Empty when nothing is pending or the sandbox was down at save time.
	RestartRequiredContainer string
```

Add the key const:

```go
	keyHiringUserID = "hiring_user_id"
	// keyRestartContainer stores the sandbox container id that a saved
	// config change made stale. Docker never reuses a container id, so
	// comparing it to the session's live ContainerID is what makes the
	// flag self-clear on every container recreate — there is deliberately
	// no code anywhere that clears this key.
	keyRestartContainer = "restart_required_container"
```

Populate it in `LoadState`'s returned struct:

```go
		HiringUserID:             kv[keyHiringUserID],
		RestartRequiredContainer: kv[keyRestartContainer],
```

Add the setter after `SaveHiringUser`:

```go
// SaveRestartRequiredContainer records which sandbox container was live
// when a restart-sensitive config change was saved. Writing "" (no
// session, or the sandbox is down) is meaningful: it is the no-banner
// case, so this deliberately does not skip empty values the way
// SaveHiringUser does.
func SaveRestartRequiredContainer(ctx context.Context, st *store.Store, orgID string, workerID orgchart.NodeID, containerID string) error {
	if st == nil || st.NodeRuntimeState == nil {
		return errors.New("helix state: store is nil")
	}
	return st.NodeRuntimeState.Set(ctx, orgID, workerID, Backend, keyRestartContainer, containerID)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd api && go test ./pkg/org/infrastructure/runtime/helix/ -run 'TestSaveRestartRequiredContainer|TestLoadState_NoStamp' -v
```

Expected: PASS, all three tests.

- [ ] **Step 5: Commit**

```bash
git add api/pkg/org/infrastructure/runtime/helix/state.go api/pkg/org/infrastructure/runtime/helix/restart_state_test.go
git commit -m "feat(org): persist stale sandbox container stamp on worker state"
```

---

### Task 4: Derive `RestartRequired` and wire the hook

**Files:**
- Modify: `api/pkg/org/interfaces/server/api/api.go` (`BotRuntimeInfo` at :247)
- Modify: `api/pkg/server/helix_org.go` (`orgWorkerRuntime.State` at :178, `buildOrgServices` botsSvc at :292, notifier wiring near :544)
- Modify: `api/pkg/org/interfaces/mcptools/builtins.go` (`Config` at :148, `botsService` at :355)
- Test: `api/pkg/server/org_restart_required_test.go` (create)

**Interfaces:**
- Consumes: `runtimehelix.WorkerState.RestartRequiredContainer` and `runtimehelix.SaveRestartRequiredContainer` from Task 3; `nodes.Deps.OnRestartRequired` from Task 2.
- Produces: `helixorgapi.BotRuntimeInfo.RestartRequired bool`. Task 5 reads it.

- [ ] **Step 1: Write the failing test**

Create `api/pkg/server/org_restart_required_test.go`:

```go
package server

import (
	"context"
	"testing"

	helixorgapi "github.com/helixml/helix/api/pkg/org/interfaces/server/api"
	runtimehelix "github.com/helixml/helix/api/pkg/org/infrastructure/runtime/helix"
	"github.com/helixml/helix/api/pkg/org/infrastructure/persistence/memory"
	"github.com/helixml/helix/api/pkg/types"
	"github.com/stretchr/testify/require"
)

// fakeSessionReader satisfies the anonymous interface orgWorkerRuntime
// holds for the Helix session/app store.
type fakeSessionReader struct{ session *types.Session }

func (f fakeSessionReader) GetSession(_ context.Context, _ string) (*types.Session, error) {
	return f.session, nil
}
func (f fakeSessionReader) GetApp(_ context.Context, _ string) (*types.App, error) {
	return nil, nil
}

func runtimeFor(t *testing.T, stamp, containerID, agentStatus string) helixorgapi.BotRuntimeInfo {
	t.Helper()
	ctx := context.Background()
	st := memory.New()
	require.NoError(t, runtimehelix.SaveProject(ctx, st, "org-rr", "b-rr", "prj_1", "app_1", "repo_1"))
	require.NoError(t, runtimehelix.SaveSession(ctx, st, "org-rr", "b-rr", "ses_1"))
	require.NoError(t, runtimehelix.SaveRestartRequiredContainer(ctx, st, "org-rr", "b-rr", stamp))

	session := &types.Session{ID: "ses_1"}
	session.Metadata.ContainerID = containerID
	session.Metadata.ExternalAgentStatus = agentStatus

	info, err := orgWorkerRuntime{st: st, sessions: fakeSessionReader{session: session}}.
		State(ctx, "org-rr", "b-rr")
	require.NoError(t, err)
	return info
}

// The banner case: the same container that was live at save time is still
// running, so it is still serving the pre-edit tool list.
func TestState_RestartRequiredWhenStampMatchesLiveContainer(t *testing.T) {
	info := runtimeFor(t, "container-a", "container-a", "running")
	require.True(t, info.RestartRequired)
}

// The self-clearing property. Any recreate — stop/start, idle reap,
// crash reconcile, full restart — yields a new Docker id, so the stamp
// stops matching with no clearing code anywhere.
func TestState_RestartNotRequiredAfterContainerReplaced(t *testing.T) {
	info := runtimeFor(t, "container-a", "container-b", "running")
	require.False(t, info.RestartRequired)
}

func TestState_RestartNotRequiredWhenSandboxStopped(t *testing.T) {
	info := runtimeFor(t, "container-a", "", "stopped")
	require.False(t, info.RestartRequired)
}

// Editing config while the sandbox is down stamps "". That must never
// match, including against a session whose ContainerID is also "".
func TestState_EmptyStampNeverMatches(t *testing.T) {
	info := runtimeFor(t, "", "", "running")
	require.False(t, info.RestartRequired)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd api && CGO_ENABLED=1 go test ./pkg/server/ -run TestState_Restart -v
```

Expected: FAIL to compile — `info.RestartRequired undefined`.

If the run fails with a tree-sitter/CGo linker error instead, install the toolchain first: `sudo apt-get update && sudo apt-get install -y gcc libc6-dev`.

- [ ] **Step 3: Write the implementation**

In `api/pkg/org/interfaces/server/api/api.go`, add to `BotRuntimeInfo`:

```go
	AgentStatus string
	// RestartRequired is true when the bot's sandbox is running but is
	// still serving config from before the operator's last save. Only
	// tool and instruction changes raise it — everything else on the bot
	// page already reaches a running sandbox without a restart.
	RestartRequired bool
```

In `api/pkg/server/helix_org.go`, replace the online-ness block in `orgWorkerRuntime.State` (:196-205):

```go
	// Resolve sandbox online-ness from the session metadata the desktop
	// stack already maintains (external_agent_status). Missing session
	// or lookup failure keeps the default "stopped".
	if s.SessionID != "" && o.sessions != nil {
		if sess, err := o.sessions.GetSession(ctx, s.SessionID); err == nil && sess != nil {
			if sess.Metadata.ExternalAgentStatus == "running" {
				info.AgentStatus = "running"
				// RestartRequiredContainer is the container that was live
				// when a restart-sensitive config change was saved. Docker
				// never reuses an id, so a match means this very container
				// is still up with the pre-edit config. Any recreate
				// (stop/start, idle reap, crash reconcile, full restart)
				// changes ContainerID and the flag self-clears — which is
				// why nothing anywhere clears the stamp. An empty stamp
				// means the sandbox was down at save time and must never
				// match, including against an empty ContainerID.
				info.RestartRequired = s.RestartRequiredContainer != "" &&
					s.RestartRequiredContainer == sess.Metadata.ContainerID
			}
		}
	}
```

Still in `helix_org.go`, in `initHelixOrgHandler` next to the existing `deps.ToolChangeNotifier` assignment (~:544):

```go
	deps.ToolChangeNotifier = cfg.APIServer.publishAgentToolChange
	// Stamp which sandbox container a restart-sensitive config change made
	// stale. Writing "" when the bot has no session or its sandbox is down
	// is the no-banner case, so this needs no is-it-running check.
	deps.RestartRequiredNotifier = func(ctx context.Context, orgID string, id orgchart.NodeID) {
		containerID := ""
		if ws, err := runtimehelix.LoadState(ctx, st, orgID, id); err == nil && ws.SessionID != "" {
			if sess, err := cfg.APIServer.Store.GetSession(ctx, ws.SessionID); err == nil && sess != nil {
				containerID = sess.Metadata.ContainerID
			}
		}
		if err := runtimehelix.SaveRestartRequiredContainer(ctx, st, orgID, id, containerID); err != nil {
			log.Warn().Err(err).Str("org_id", orgID).Str("bot", string(id)).Msg("failed to stamp restart-required container")
		}
	}
```

In `buildOrgServices` (:292), add to the `nodes.Deps` literal:

```go
		OnToolsChanged:    deps.ToolChangeNotifier,
		OnRestartRequired: deps.RestartRequiredNotifier,
```

In `api/pkg/org/interfaces/mcptools/builtins.go`, add to `Config` (next to `ToolChangeNotifier` at :154):

```go
	ToolChangeNotifier     func(context.Context, string)
	RestartRequiredNotifier func(context.Context, string, orgchart.NodeID)
```

and to `botsService`'s `nodes.Deps` literal (:356):

```go
		OnToolsChanged:    c.ToolChangeNotifier,
		OnRestartRequired: c.RestartRequiredNotifier,
```

Add the `orgchart` import to `builtins.go` if it is not already there.

Leave the bare `nodes.New` in `api/pkg/server/helix_org_middleware.go:167` unwired. It is a fallback used only for `Reconcile`, which is a base-tool backfill that fires neither notifier today.

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd api && CGO_ENABLED=1 go test ./pkg/server/ -run TestState_Restart -v
cd api && go build ./pkg/server/ ./pkg/org/... ./pkg/store/ ./pkg/types/
```

Expected: PASS, all four tests; build clean.

- [ ] **Step 5: Commit**

```bash
git add api/pkg/org/interfaces/server/api/api.go api/pkg/server/helix_org.go api/pkg/org/interfaces/mcptools/builtins.go api/pkg/server/org_restart_required_test.go
git commit -m "feat(org): derive restart-required from live sandbox container id"
```

---

### Task 5: Expose `restart_required` on the bot API

**Files:**
- Modify: `api/pkg/org/interfaces/server/api/dto.go` (`BotDTO` at :43)
- Modify: `api/pkg/org/interfaces/server/api/bots.go` (`listBots` at :58-77, `getBot` at :194-215)
- Test: `api/pkg/org/interfaces/server/api/restart_required_dto_test.go` (create)

**Interfaces:**
- Consumes: `BotRuntimeInfo.RestartRequired` from Task 4.
- Produces: `BotDTO.RestartRequired bool` → JSON `restart_required` → generated TS `ApiBotDTO.restart_required?: boolean`. Tasks 6 and 7 read it.

- [ ] **Step 1: Write the failing test**

Create `api/pkg/org/interfaces/server/api/restart_required_dto_test.go`:

```go
package api

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// The frontend keys off restart_required; renaming the JSON tag silently
// breaks the banner, so pin it.
func TestBotDTO_RestartRequiredJSONTag(t *testing.T) {
	raw, err := json.Marshal(BotDTO{ID: "b-one", RestartRequired: true})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	require.Equal(t, true, decoded["restart_required"])
}

func TestBotDTO_RestartRequiredOmittedWhenFalse(t *testing.T) {
	raw, err := json.Marshal(BotDTO{ID: "b-one"})
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	_, present := decoded["restart_required"]
	require.False(t, present)
}
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd api && go test ./pkg/org/interfaces/server/api/ -run TestBotDTO_RestartRequired -v
```

Expected: FAIL to compile — `unknown field RestartRequired`.

- [ ] **Step 3: Write the implementation**

In `dto.go`, add to `BotDTO` after `AgentStatus`:

```go
	AgentStatus string `json:"agent_status,omitempty"`
	// RestartRequired is true when the sandbox is running but still holds
	// the tool list and instructions from before the last save. Drives the
	// restart banner on the bot page and the org chat panel.
	RestartRequired bool `json:"restart_required,omitempty"`
```

In `bots.go` `listBots`, inside the existing `if info, err := ...State(...)` block:

```go
				if info.AgentStatus != "" {
					dto.AgentStatus = info.AgentStatus
				}
				dto.RestartRequired = info.RestartRequired
				dto.AgentRuntime = info.Runtime
				dto.AgentModel = info.Model
```

In `bots.go` `getBot`, inside its equivalent block:

```go
			if info.AgentStatus != "" {
				detail.Bot.AgentStatus = info.AgentStatus
			}
			detail.Bot.RestartRequired = info.RestartRequired
			detail.Bot.AgentRuntime = info.Runtime
			detail.Bot.AgentModel = info.Model
```

- [ ] **Step 4: Run the tests, regenerate the client, verify the build**

```bash
cd api && go test ./pkg/org/interfaces/server/api/ -run TestBotDTO_RestartRequired -v
./stack update_openapi
grep -n "restart_required" frontend/src/api/api.ts
cd frontend && yarn build
```

Expected: tests PASS; `grep` shows `restart_required?: boolean;` inside `ApiBotDTO`; frontend build succeeds.

- [ ] **Step 5: Commit**

```bash
git add api/pkg/org/interfaces/server/api/ frontend/src/api/ api/pkg/server/docs.go docs/
git commit -m "feat(org): expose restart_required on bot list and detail"
```

---

### Task 6: The banner component

Self-contained and dumb, so both mount points get identical behaviour and the "never restart without confirmation" guarantee lives in exactly one place.

**Files:**
- Create: `frontend/src/components/helix-org/AgentRestartRequiredBanner.tsx`
- Test: `frontend/src/components/helix-org/AgentRestartRequiredBanner.test.tsx`

**Interfaces:**
- Produces:

```ts
export interface AgentRestartRequiredBannerProps {
  visible: boolean
  working?: boolean   // agent is mid-turn — restart is gated
  busy?: boolean      // a lifecycle mutation is already in flight
  onRestart: () => void
}
```

  Task 7 mounts it with these props.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/helix-org/AgentRestartRequiredBanner.test.tsx`:

```tsx
import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import AgentRestartRequiredBanner from './AgentRestartRequiredBanner'

describe('AgentRestartRequiredBanner', () => {
  it('renders nothing when no restart is pending', () => {
    render(<AgentRestartRequiredBanner visible={false} onRestart={vi.fn()} />)
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()
  })

  it('renders when a restart is pending', () => {
    render(<AgentRestartRequiredBanner visible onRestart={vi.fn()} />)
    expect(screen.getByTestId('agent-restart-required-banner')).toBeInTheDocument()
  })

  // The restart discards the conversation, so it must never fire straight
  // off the banner button.
  it('does not restart until the cost is confirmed', () => {
    const onRestart = vi.fn()
    render(<AgentRestartRequiredBanner visible onRestart={onRestart} />)

    fireEvent.click(screen.getByRole('button', { name: /restart sandbox/i }))
    expect(onRestart).not.toHaveBeenCalled()
    expect(screen.getByText(/current chat history is discarded/i)).toBeInTheDocument()
  })

  it('restarts once the dialog is confirmed', () => {
    const onRestart = vi.fn()
    render(<AgentRestartRequiredBanner visible onRestart={onRestart} />)

    fireEvent.click(screen.getByRole('button', { name: /restart sandbox/i }))
    fireEvent.click(screen.getByTestId('agent-restart-confirm'))
    expect(onRestart).toHaveBeenCalledTimes(1)
  })

  it('cancelling the dialog restarts nothing', () => {
    const onRestart = vi.fn()
    render(<AgentRestartRequiredBanner visible onRestart={onRestart} />)

    fireEvent.click(screen.getByRole('button', { name: /restart sandbox/i }))
    fireEvent.click(screen.getByTestId('agent-restart-cancel'))
    expect(onRestart).not.toHaveBeenCalled()
    expect(screen.getByTestId('agent-restart-required-banner')).toBeInTheDocument()
  })

  it('hides for this tab when dismissed', () => {
    render(<AgentRestartRequiredBanner visible onRestart={vi.fn()} />)

    fireEvent.click(screen.getByRole('button', { name: /not now/i }))
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()
  })

  // Live work in progress is the one thing a restart genuinely destroys.
  it('gates the restart while the agent is mid-turn', () => {
    render(<AgentRestartRequiredBanner visible working onRestart={vi.fn()} />)
    expect(screen.getByRole('button', { name: /restart sandbox/i })).toBeDisabled()
  })

  it('gates the restart while a lifecycle action is in flight', () => {
    render(<AgentRestartRequiredBanner visible busy onRestart={vi.fn()} />)
    expect(screen.getByRole('button', { name: /restart sandbox/i })).toBeDisabled()
  })
})
```

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd frontend && yarn vitest run src/components/helix-org/AgentRestartRequiredBanner.test.tsx
```

Expected: FAIL — cannot resolve `./AgentRestartRequiredBanner`.

- [ ] **Step 3: Write the implementation**

Create `frontend/src/components/helix-org/AgentRestartRequiredBanner.tsx`:

```tsx
// Shown when a bot's sandbox is running but still holds the tool list and
// instructions from before the operator's last save. The MCP tool list is
// fetched once at agent startup and never refreshed, so the only way to
// apply those changes is a restart.
//
// The restart mints a brand-new session and thread on purpose: a preserved
// transcript still contains successful tool calls for tools that no longer
// exist, and the model reads its own history as proof of capability. It is
// never automatic — an in-flight turn is the one thing a restart destroys,
// so the button is gated while the agent is working and the cost is spelled
// out in a confirm dialog.

import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogContentText from '@mui/material/DialogContentText'
import DialogTitle from '@mui/material/DialogTitle'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { RotateCcw } from 'lucide-react'

import { APP_FONT_FAMILY } from '../../styles/typography'

export interface AgentRestartRequiredBannerProps {
  visible: boolean
  working?: boolean
  busy?: boolean
  onRestart: () => void
}

const AgentRestartRequiredBanner: FC<AgentRestartRequiredBannerProps> = ({
  visible,
  working = false,
  busy = false,
  onRestart,
}) => {
  const [dismissed, setDismissed] = useState(false)
  const [confirming, setConfirming] = useState(false)

  if (!visible || dismissed) return null

  const gated = working || busy
  const gateReason = working
    ? 'The agent is working — restart when the current turn finishes'
    : busy
      ? 'Another action is in progress'
      : ''

  const confirm = () => {
    setConfirming(false)
    onRestart()
  }

  return (
    <>
      <Box
        data-testid="agent-restart-required-banner"
        role="status"
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          px: 1.5,
          py: 1,
          mb: 1,
          borderRadius: 1,
          border: '1px solid rgba(255, 167, 38, 0.35)',
          backgroundColor: 'rgba(255, 167, 38, 0.08)',
        }}
      >
        <RotateCcw size={18} strokeWidth={1.8} />
        <Typography
          variant="body2"
          sx={{ flexGrow: 1, fontSize: '0.8rem', fontFamily: APP_FONT_FAMILY }}
        >
          Tool and instruction changes apply after a restart.
        </Typography>
        <Stack direction="row" alignItems="center" spacing={0.75}>
          <Button size="small" onClick={() => setDismissed(true)}>
            Not now
          </Button>
          <Tooltip title={gateReason}>
            <span>
              <Button
                size="small"
                variant="contained"
                color="secondary"
                disabled={gated}
                onClick={() => setConfirming(true)}
              >
                Restart sandbox
              </Button>
            </span>
          </Tooltip>
        </Stack>
      </Box>

      <Dialog open={confirming} onClose={() => setConfirming(false)}>
        <DialogTitle>Restart sandbox?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Restarts the sandbox with a fresh conversation. The workspace and
            committed work are kept; the current chat history is discarded.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button data-testid="agent-restart-cancel" onClick={() => setConfirming(false)}>
            Cancel
          </Button>
          <Button
            data-testid="agent-restart-confirm"
            variant="contained"
            color="secondary"
            onClick={confirm}
          >
            Restart
          </Button>
        </DialogActions>
      </Dialog>
    </>
  )
}

export default AgentRestartRequiredBanner
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd frontend && yarn vitest run src/components/helix-org/AgentRestartRequiredBanner.test.tsx
```

Expected: PASS, all eight tests.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/helix-org/AgentRestartRequiredBanner.tsx frontend/src/components/helix-org/AgentRestartRequiredBanner.test.tsx
git commit -m "feat(frontend): add agent restart-required banner component"
```

---

### Task 7: Mount the banner on both surfaces

**Files:**
- Modify: `frontend/src/pages/HelixOrgBotDetail.tsx` (imports ~:59, header block ~:495)
- Modify: `frontend/src/components/helix-org/HelixOrgChatPanel.tsx` (imports ~:23, above `<AgentChat` at :428)
- Test: `frontend/src/components/helix-org/HelixOrgChatPanel.test.tsx` (extend)

**Interfaces:**
- Consumes: `AgentRestartRequiredBanner` from Task 6; `bot.restart_required` from Task 5; the existing `useRestartBotAgent` mutation and `handleRestartSession` handler.

- [ ] **Step 1: Write the failing test**

Append to `frontend/src/components/helix-org/HelixOrgChatPanel.test.tsx`, inside the existing top-level `describe`:

```tsx
  it('shows the restart banner when the selected bot reports stale config', () => {
    renderPanel({ bot: { id: 'b-one', agent_status: 'running', restart_required: true } })
    expect(screen.getByTestId('agent-restart-required-banner')).toBeInTheDocument()
  })

  it('hides the restart banner when config is current', () => {
    renderPanel({ bot: { id: 'b-one', agent_status: 'running', restart_required: false } })
    expect(screen.queryByTestId('agent-restart-required-banner')).toBeNull()
  })
```

Read the existing file first: it already mocks `../session/AgentChat` (:41). Reuse whatever render/mock helper it defines rather than inventing `renderPanel` — if the file has no such helper, extract the existing setup in the first test into one named `renderPanel({ bot })` that seeds the mocked `useHelixOrgBot` response, and update the existing tests to call it. Keep the mocking style already used in that file.

- [ ] **Step 2: Run the test to verify it fails**

```bash
cd frontend && yarn vitest run src/components/helix-org/HelixOrgChatPanel.test.tsx
```

Expected: FAIL — `agent-restart-required-banner` not found.

- [ ] **Step 3: Write the implementation**

In `HelixOrgChatPanel.tsx`, add the import next to the other local component imports:

```tsx
import AgentRestartRequiredBanner from './AgentRestartRequiredBanner'
```

Immediately above the `<AgentChat` element (:428), add:

```tsx
            <AgentRestartRequiredBanner
              visible={!!selectedBot?.restart_required}
              working={!!chatSessionId && streaming.currentResponses.has(chatSessionId)}
              busy={busy}
              onRestart={() => { void handleRestartSession() }}
            />
```

`streaming` (:58), `busy` (:232), `selectedBot` (:122) and `chatSessionId` are already in scope. If the restart handler in this file is named differently, use the existing one that calls `restartAgent.mutateAsync` — do not add a second restart path.

In `HelixOrgBotDetail.tsx`, add the import next to the other local component imports:

```tsx
import AgentRestartRequiredBanner from '../components/helix-org/AgentRestartRequiredBanner'
```

and, if it is not already imported, the streaming hook:

```tsx
import { useStreaming } from '../contexts/streaming'
```

Inside the component, next to the other hooks:

```tsx
  const streaming = useStreaming()
```

Directly below the closing `</Box>` of the header `Stack` (the block ending just before the `Name` field at ~:495), add:

```tsx
                <AgentRestartRequiredBanner
                  visible={!!bot.restart_required}
                  working={!!chatSessionId && streaming.currentResponses.has(chatSessionId)}
                  busy={activateAgent.isPending || stopAgent.isPending || restartAgent.isPending}
                  onRestart={() => { void handleRestartSession() }}
                />
```

`bot`, `chatSessionId`, `activateAgent`, `stopAgent`, `restartAgent` and `handleRestartSession` are already in scope in that render branch.

- [ ] **Step 4: Run the tests and build**

```bash
cd frontend && yarn vitest run src/components/helix-org/
cd frontend && yarn build
```

Expected: all org component tests PASS; build succeeds.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/pages/HelixOrgBotDetail.tsx frontend/src/components/helix-org/HelixOrgChatPanel.tsx frontend/src/components/helix-org/HelixOrgChatPanel.test.tsx
git commit -m "feat(frontend): surface restart banner on bot detail and chat panel"
```

---

### Task 8: End-to-end verification in the inner Helix

Unit tests prove the mechanism; they do not prove the wired-up feature. Per CLAUDE.md this step is not optional, and the result must be reported as run-or-not-run, never as "covered by unit tests".

**Files:**
- Modify: `design/2026-08-24-agent-restart-required-banner.md` (append a "Verification" section with the observed results)

**Interfaces:**
- Consumes: everything from Tasks 1-7.

- [ ] **Step 1: Bring the stack up and register**

```bash
cd /home/phil/helix && docker compose -f docker-compose.dev.yaml ps
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080
```

Poll both until `helix-api-1`, `helix-frontend-1` and `helix-postgres-1` are `Up` and `8080` returns `200`. Full bring-up takes 5-10 minutes — a `000` or `Restarting` means still starting, not broken. Do not conclude the stack is unavailable before then.

Then register at `http://localhost:8080/login` with `test@helix.ml` / `helixtest` (full name `Test User`), or sign in if the account exists, and complete onboarding.

- [ ] **Step 2: Confirm `ContainerID` is populated for a running org bot**

This is the one assumption the whole mechanism rests on. If it is empty on a running sandbox, the banner silently never fires.

Create an org bot in the UI and start its agent, then:

```bash
docker exec helix-postgres-1 psql -U postgres -d postgres -c \
  "SELECT id, config->>'external_agent_status' AS status, config->>'container_id' AS container \
   FROM sessions WHERE config->>'org_worker_id' <> '' ORDER BY created DESC LIMIT 5;"
```

Expected: the running session shows a non-empty `container` alongside `status = running`. If it is empty, **stop and report it** — the design's core assumption is wrong and the mechanism needs revisiting before shipping.

- [ ] **Step 3: Verify the banner appears on both surfaces**

With the bot's agent running, open its detail page, grant it one extra tool, and save.

Expected: the banner appears under the header without a page reload. Navigate to the org chat panel with that bot selected — the banner appears above the composer too.

Confirm the stamp landed:

```bash
docker exec helix-postgres-1 psql -U postgres -d postgres -c \
  "SELECT node_id, key, value FROM node_runtime_states WHERE key = 'restart_required_container';"
```

Expected: one row whose `value` equals the `container` from Step 2. If the table name differs, find it with `\dt` — the repository is `api/pkg/org/infrastructure/persistence/gorm/`.

- [ ] **Step 4: Verify no banner for hot-applying settings**

Rename the bot and save. Toggle preserve-context and save.

Expected: no banner appears for either. If one does, the fingerprint has widened past `Tools` + `Content`.

- [ ] **Step 5: Verify the restart clears it and applies the change**

Click **Restart sandbox** and confirm the dialog.

Expected: the sandbox restarts; the banner disappears on both surfaces once the new container is up; `restart_required_container` no longer matches the new session's `container_id`. Ask the bot in chat to use the newly granted tool and confirm it succeeds — that is the actual user-visible outcome this whole change exists to deliver.

- [ ] **Step 6: Verify the mid-turn gate**

Send the bot a long-running prompt, and while it is streaming, edit its tools in another tab and return to the chat panel.

Expected: the banner is visible but **Restart sandbox** is disabled with the "agent is working" tooltip. It becomes enabled once the turn finishes.

- [ ] **Step 7: Record the results and commit**

Append a `## Verification` section to `design/2026-08-24-agent-restart-required-banner.md` stating, per step, what was run and what was observed — including anything that could not be exercised and why. Do not write "verified" for a step that was not actually run.

```bash
git add design/2026-08-24-agent-restart-required-banner.md
git commit -m "docs(design): record restart banner end-to-end verification"
```

---

## Self-Review

**Spec coverage:**

| Spec section | Task |
|---|---|
| Narrow fingerprint (`Tools` + `Content`), exclusions | 1 |
| Stamp on save via the existing node-mutation service | 2 |
| `NodeRuntimeState` persistence, no clearing code | 3 |
| Pure read derivation, `ContainerID` comparison, hook wiring | 4 |
| `BotRuntimeInfo` / `BotDTO` surfacing on get + list | 4, 5 |
| Full-session restart, confirm dialog, never automatic, mid-turn gate | 6 |
| Both mount points (bot detail, chat panel) | 7 |
| Known-coupling check that `ContainerID` is populated | 8 (Step 2) |
| End-to-end in the inner Helix | 8 |

`notifications/tools/list_changed` is explicitly out of scope in the spec and correctly has no task.

**Type consistency:** `RestartFingerprint(Node) string` (Task 1) is called in Task 2. `OnRestartRequired func(context.Context, string, orgchart.NodeID)` is defined in Task 2 and wired in Task 4 under the matching `mcptools.Config.RestartRequiredNotifier`. `SaveRestartRequiredContainer` / `WorkerState.RestartRequiredContainer` (Task 3) are consumed in Task 4. `BotRuntimeInfo.RestartRequired` (Task 4) feeds `BotDTO.RestartRequired` (Task 5) feeds `restart_required` in Tasks 6-7. The banner's prop names (`visible`, `working`, `busy`, `onRestart`) are identical in Tasks 6 and 7.
