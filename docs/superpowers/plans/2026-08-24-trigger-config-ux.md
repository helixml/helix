# Trigger Configuration UX Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every Trigger's settings, current values, and activation method visible and editable across all three UI surfaces, and fix the two kinds that cannot be created at all.

**Architecture:** Each transport Kind declares its settings in Go via a `Descriptor` returned from the existing `Strategy` interface, so the compiler enforces coverage and there is one source of truth. A new `GET /trigger-kinds` endpoint serves those descriptors; `TriggerDTO` carries a resolved per-Trigger activation recipe. One React `TriggerConfig` component renders descriptors at three densities and backs the create drawer, the org-chart side pane, and the detail page.

**Tech Stack:** Go 1.x (stdlib `net/http` mux, GORM), React 18 + TypeScript, MUI v5, react-query, vitest, Vite.

**Spec:** `design/2026-08-24-trigger-config-ux.md`

## Global Constraints

- **Ids never change.** Internal ids (`tr-…`, `s-…`, `b-…`, `e-…`) stay exactly as they are, including in URLs. No slug column, no backfill, no re-minting. Only their *visual prominence* changes.
- **Config visibility only.** Do not add prerequisite gating of unusable kinds, "send test event" buttons, or liveness probes. Populating a field's dropdown options (e.g. listing Slack workspaces) is field rendering, not readiness gating, and IS in scope.
- **No fallbacks** (CLAUDE.md). One code path per behaviour. Delete dead code as you go.
- **Frontend must use the generated API client** (`api.getApiClient()`), never raw `fetch`/`api.get`/`api.post`. New endpoints need swagger annotations plus `./stack update_openapi`.
- **Comments only when necessary for external documentation** (CLAUDE.md + user memory). Do not narrate code.
- **Lucide icons only** in this UI; no MUI icons. Tables use `SimpleTable`. Copyable snippets use `MarkdownCodeBlock`.
- **Conventional commits**, subject ≤ 72 chars, imperative mood, no trailing period. The `commit-msg` hook rejects non-conforming messages.
- Go build check: `cd api && CGO_ENABLED=0 go build ./...`
- Go tests: `cd api && CGO_ENABLED=0 go test ./pkg/org/... -count=1`
- Frontend tests: `cd frontend && yarn test`
- Frontend build: `cd frontend && yarn build`
- Dev stack is at `http://localhost:8080` (never `api:8080`). Air hot-reloads Go; Vite HMR serves the frontend on 8081 behind 8080.

---

## File Structure

**Created:**
- `api/pkg/org/domain/transport/descriptor.go` — `Descriptor`, `Field`, `Activation`, `SecretRef`, `FieldType`, `Direction`, `DescribeAll()`, `ResolveActivation()`
- `api/pkg/org/domain/transport/descriptor_test.go` — parity + ordering + activation resolution tests
- `frontend/src/services/triggerKindService.ts` — `useTriggerKinds()` hook + `TriggerKindDescriptor` types
- `frontend/src/components/helix-org/trigger/TriggerConfig.tsx` — descriptor-driven renderer, three densities
- `frontend/src/components/helix-org/trigger/TriggerFieldRenderer.tsx` — `FieldType` → control
- `frontend/src/components/helix-org/trigger/TriggerActivationCard.tsx` — "How to fire this"
- `frontend/src/components/helix-org/trigger/TriggerSecretsNote.tsx` — credential locations
- `frontend/src/components/helix-org/trigger/fields/SlackWorkspacePicker.tsx`
- `frontend/src/components/helix-org/trigger/fields/GitLabRepoPicker.tsx`
- `frontend/src/components/helix-org/trigger/fields/GitLabEventsField.tsx`
- `frontend/src/components/helix-org/trigger/triggerConfigModel.ts` — pure helpers (draft init, dirty check, submit payload)
- `frontend/src/components/helix-org/trigger/triggerConfigModel.test.ts`

**Modified:**
- `api/pkg/org/domain/transport/transport.go` — add `Describe()` to `Strategy`; add `DescribeAll()`
- `api/pkg/org/domain/transport/{local,helixevents,webhook,email,cron,slack,github,gitlab}.go` — one `Describe()` each
- `api/pkg/org/interfaces/server/api/triggers.go` — `TriggerDTO.Activation`, `listTriggerKinds` handler
- `api/pkg/org/interfaces/server/api/api.go` — register `GET /trigger-kinds`
- `api/pkg/org/interfaces/server/orgctx.go` — `WithOrgHandle` / `OrgHandleFromContext`
- `api/pkg/server/helix_org_middleware.go` — stash the raw `{org}` URL segment
- `api/pkg/org/interfaces/server/server.go` — register org-less `POST /webhooks/{triggerID}`
- `api/pkg/org/interfaces/server/webhook.go` — resolve org properly
- `api/pkg/org/interfaces/mcptools/triggers.go` — drop `id` from `create_trigger` args (Task 10, optional)
- `frontend/src/components/helix-org/TriggerFormDialog.tsx` — render `TriggerConfig` in create mode
- `frontend/src/components/helix-org/TriggerDetailDrawer.tsx` — render `TriggerConfig` compact/read
- `frontend/src/pages/HelixOrgTriggerDetail.tsx` — name prominence + `TriggerConfig` full
- `frontend/src/pages/HelixOrgTriggers.tsx` — kind labels, id de-emphasis, Edit row action

**Deleted:** none. `CronScheduleFields`, `GitHubRepoPicker`, `GitHubTopicConfigFields`, `TriggerWebhookPanel` are all reused as-is.

---

### Task 1: Descriptor types and per-Kind descriptors

**Files:**
- Create: `api/pkg/org/domain/transport/descriptor.go`
- Create: `api/pkg/org/domain/transport/descriptor_test.go`
- Modify: `api/pkg/org/domain/transport/transport.go` (the `Strategy` interface, ~line 44)
- Modify: `api/pkg/org/domain/transport/local.go`, `helixevents.go`, `webhook.go`, `email.go`, `cron.go`, `slack.go`, `github.go`, `gitlab.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `transport.Descriptor`, `transport.Field`, `transport.FieldType`, `transport.Direction`, `transport.Activation`, `transport.SecretRef`, `transport.DescribeAll() []Descriptor`, and `Describe() Descriptor` on every `Strategy`.

Adding `Describe()` to `Strategy` rather than a separate optional interface makes the compiler reject any new Kind that lacks a descriptor. No package outside `transport` implements `transport.Strategy` (`processor.Strategy` is unrelated), so this is safe.

- [ ] **Step 1: Write the failing test**

Create `api/pkg/org/domain/transport/descriptor_test.go`:

```go
package transport_test

import (
	"encoding/json"
	"testing"

	"github.com/helixml/helix/api/pkg/org/domain/transport"
)

// Every Kind must be described, in canonical order.
func TestDescribeAll_CoversEveryKindInOrder(t *testing.T) {
	t.Parallel()

	kinds := transport.KindValues()
	got := transport.DescribeAll()
	if len(got) != len(kinds) {
		t.Fatalf("DescribeAll returned %d descriptors, want %d", len(got), len(kinds))
	}
	for i, k := range kinds {
		if got[i].Kind != k {
			t.Errorf("descriptor %d is %q, want %q", i, got[i].Kind, k)
		}
		if got[i].Label == "" {
			t.Errorf("kind %q has an empty Label", k)
		}
		if got[i].Summary == "" {
			t.Errorf("kind %q has an empty Summary", k)
		}
	}
}

// Descriptor-vs-validator parity: a field the descriptor marks Required
// must actually be demanded by Validate(). This guards drift between the
// two, which sit in the same file precisely so they stay in step.
func TestDescriptorRequiredFieldsAreEnforcedByValidate(t *testing.T) {
	t.Parallel()

	for _, d := range transport.DescribeAll() {
		for _, f := range d.Fields {
			if !f.Required {
				continue
			}
			// Build a config with every OTHER required field populated,
			// omitting only f — so the error we observe is about f.
			cfg := map[string]any{}
			for _, other := range d.Fields {
				if other.Required && other.Name != f.Name {
					cfg[other.Name] = sampleFor(other)
				}
			}
			raw, err := json.Marshal(cfg)
			if err != nil {
				t.Fatalf("kind %q: marshal config: %v", d.Kind, err)
			}
			tr := transport.Transport{Kind: d.Kind, Config: raw}
			if err := tr.Validate(); err == nil {
				t.Errorf("kind %q: field %q is Required in the descriptor but Validate() accepts a config without it",
					d.Kind, f.Name)
			}
		}
	}
}

// sampleFor returns a value that satisfies the field's own validation, so
// omitting a DIFFERENT field is the only reason Validate can fail.
func sampleFor(f transport.Field) any {
	switch f.Type {
	case transport.FieldGitHubRepo:
		return "helixml/helix"
	case transport.FieldGitHubEvents:
		return []string{"*"}
	case transport.FieldGitLabEvents:
		return []string{"Merge Request Hook"}
	case transport.FieldStringList:
		return []string{"*"}
	case transport.FieldCron:
		return "0 9 * * 1"
	case transport.FieldURL:
		return "https://example.com/hook"
	default:
		return "sample"
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && CGO_ENABLED=0 go test ./pkg/org/domain/transport/ -run 'Describe' -v`
Expected: FAIL — compile error, `undefined: transport.DescribeAll`.

- [ ] **Step 3: Create the descriptor types**

Create `api/pkg/org/domain/transport/descriptor.go`:

```go
package transport

// FieldType tells the UI which control to render for a Field. Types
// beyond the primitives name a Helix-specific picker whose options come
// from a live org-scoped list.
type FieldType string

const (
	FieldString       FieldType = "string"
	FieldURL          FieldType = "url"
	FieldStringList   FieldType = "string_list"
	FieldCron         FieldType = "cron"
	FieldGitHubRepo   FieldType = "github_repo"
	FieldGitHubEvents FieldType = "github_events"
	FieldGitLabRepo   FieldType = "gitlab_repo"
	FieldGitLabEvents FieldType = "gitlab_events"
	FieldSlackWorkspace FieldType = "slack_workspace"
	FieldSlackChannel   FieldType = "slack_channel"
)

// Direction says whether a Field affects data coming IN to the Trigger
// or going OUT of it. It exists because the webhook Kind's only field is
// outbound, which every user reads as inbound.
type Direction string

const (
	Inbound  Direction = "inbound"
	Outbound Direction = "outbound"
)

// Field is one configurable key inside a Transport's Config blob.
type Field struct {
	Name        string    `json:"name"`
	Label       string    `json:"label"`
	Help        string    `json:"help,omitempty"`
	Placeholder string    `json:"placeholder,omitempty"`
	Type        FieldType `json:"type"`
	Required    bool      `json:"required,omitempty"`
	ReadOnly    bool      `json:"read_only,omitempty"`
	Direction   Direction `json:"direction"`
}

// Activation describes how a Trigger of this Kind is fired. Templates use
// {public}, {org}, {id} and {field:<name>} placeholders, resolved per
// Trigger by ResolveActivation.
type Activation struct {
	Summary         string `json:"summary"`
	Verb            string `json:"verb,omitempty"`
	URLTemplate     string `json:"url_template,omitempty"`
	AuthHeader      string `json:"auth_header,omitempty"`
	AddressTemplate string `json:"address_template,omitempty"`
	Note            string `json:"note,omitempty"`
}

// SecretRef names where a Kind's credentials live. Kinds needing no
// credentials return none.
type SecretRef struct {
	Label      string `json:"label"`
	SettingKey string `json:"setting_key,omitempty"`
	Location   string `json:"location"`
}

// Descriptor is everything the UI needs to render, explain, and validate
// one Kind's settings. It is returned by that Kind's Strategy, so it
// lives in the same file as the Config type and its Validate rules.
type Descriptor struct {
	Kind          Kind        `json:"kind"`
	Label         string      `json:"label"`
	Summary       string      `json:"summary"`
	Fields        []Field     `json:"fields,omitempty"`
	Activation    Activation  `json:"activation"`
	Secrets       []SecretRef `json:"secrets,omitempty"`
	SystemManaged bool        `json:"system_managed,omitempty"`
}

// DescribeAll returns every registered Kind's Descriptor in canonical
// display order (see kindOrder).
func DescribeAll() []Descriptor {
	out := make([]Descriptor, 0, len(kindOrder))
	for _, k := range kindOrder {
		out = append(out, strategies[k].Describe())
	}
	return out
}
```

- [ ] **Step 4: Add Describe to the Strategy interface**

In `api/pkg/org/domain/transport/transport.go`, extend the `Strategy` interface:

```go
type Strategy interface {
	ParseConfig(json.RawMessage) (Config, error)
	Describe() Descriptor
}
```

Update the doc comment above `strategies` so "adding a new Kind" also names `Describe()`:

```go
// strategies registers every known Kind's Strategy. Adding a new Kind
// means adding a new file in this package that defines its Kind
// constant, its Config type with Validate(), its Describe() descriptor,
// and its Strategy implementation — plus one entry here AND in
// kindOrder. Validate() itself does not change.
```

- [ ] **Step 5: Add Describe to each Kind**

Append to `local.go`:

```go
func (local) Describe() Descriptor {
	return Descriptor{
		Kind:    KindLocal,
		Label:   "Manual or agent event",
		Summary: "Fires when an agent or the Helix API publishes to it. Nothing external reaches this Trigger.",
		Activation: Activation{
			Summary: "An agent publishes to this Trigger with its publish tool, or Helix publishes to it internally.",
		},
	}
}
```

Append to `helixevents.go`:

```go
func (helixEvents) Describe() Descriptor {
	return Descriptor{
		Kind:          KindHelixEvents,
		Label:         "Helix event bus",
		Summary:       "Org-wide bus carrying Helix's own events. Created and maintained by Helix; one per org.",
		SystemManaged: true,
		Activation: Activation{
			Summary: "Helix publishes to this Trigger automatically when org events occur. There is nothing to configure.",
		},
	}
}
```

Append to `webhook.go`:

```go
func (webhook) Describe() Descriptor {
	return Descriptor{
		Kind:    KindWebhook,
		Label:   "Webhook",
		Summary: "Fires when an external system POSTs to this Trigger's URL.",
		Fields: []Field{{
			Name:        "outbound_url",
			Label:       "Also POST every event to (optional)",
			Help:        "Outbound relay, not the inbound URL. When set, every event on this Trigger is additionally POSTed to this address. Leave empty for inbound-only.",
			Placeholder: "https://example.com/receive",
			Type:        FieldURL,
			Direction:   Outbound,
		}},
		Activation: Activation{
			Summary:     "POST any JSON or text body to this URL. The body becomes the event.",
			Verb:        "POST",
			URLTemplate: "{public}/api/v1/orgs/{org}/webhooks/{id}",
			AuthHeader:  "Authorization: Bearer <your Helix API key>",
			Note:        "Requests are authenticated with a Helix API key, not a per-Trigger signing secret. Bodies are capped at 1 MiB.",
		},
	}
}
```

Append to `email.go`:

```go
func (email) Describe() Descriptor {
	return Descriptor{
		Kind:    KindEmail,
		Label:   "Incoming email",
		Summary: "Fires when mail arrives at this Trigger's address.",
		Fields: []Field{{
			Name:        "alias",
			Label:       "Inbox alias",
			Help:        "Lowercase letters, digits, dash and underscore only. No @, + or dots.",
			Placeholder: "support",
			Type:        FieldString,
			Required:    true,
			Direction:   Inbound,
		}},
		Activation: Activation{
			Summary:         "Send mail to this address. Subject and body become the event.",
			AddressTemplate: "{field:alias}@<your inbound domain>",
			Note:            "The exact address depends on the Postmark setup: <hash>+<alias>@inbound.postmarkapp.com when no domain is configured, or <alias>@yourdomain.com when one is.",
		},
		Secrets: []SecretRef{{
			Label:      "Postmark account (token, inbound address, from address)",
			SettingKey: "transport.postmark",
			Location:   "Organization Settings",
		}},
	}
}
```

Append to `cron.go`:

```go
func (cron) Describe() Descriptor {
	return Descriptor{
		Kind:    KindCron,
		Label:   "Schedule",
		Summary: "Fires on a schedule. Nothing external is involved.",
		Fields: []Field{
			{
				Name:      "schedule",
				Label:     "Schedule",
				Help:      "Standard 5-field cron expression, optionally prefixed with CRON_TZ=<zone>. Consecutive fires must be at least 90 seconds apart.",
				Type:      FieldCron,
				Required:  true,
				Direction: Inbound,
			},
			{
				Name:        "message",
				Label:       "Message (optional)",
				Help:        "Text delivered to the attached agents on every fire. Leave empty for a bare tick.",
				Placeholder: "Check the overnight build",
				Type:        FieldString,
				Direction:   Inbound,
			},
		},
		Activation: Activation{
			Summary: "Helix's scheduler fires this Trigger on the schedule above.",
		},
	}
}
```

Append to `slack.go`:

```go
func (slack) Describe() Descriptor {
	return Descriptor{
		Kind:    KindSlack,
		Label:   "Slack event",
		Summary: "Fires when a message arrives in a connected Slack workspace.",
		Fields: []Field{
			{
				Name:      "service_connection_id",
				Label:     "Slack workspace",
				Help:      "Which connected workspace this Trigger listens to.",
				Type:      FieldSlackWorkspace,
				Required:  true,
				Direction: Inbound,
			},
			{
				Name:        "channel_id",
				Label:       "Channel (optional)",
				Help:        "Limit this Trigger to one channel. Leave empty to receive messages from the whole workspace that no channel-specific Trigger already claims.",
				Placeholder: "C012ABCDEF",
				Type:        FieldSlackChannel,
				Direction:   Inbound,
			},
		},
		Activation: Activation{
			Summary: "Post a message in the connected workspace (or the specific channel above).",
		},
		Secrets: []SecretRef{{
			Label:    "Slack workspace connection (bot token)",
			Location: "Organization Settings, Connected Accounts",
		}},
	}
}
```

Append to `github.go`:

```go
func (github) Describe() Descriptor {
	return Descriptor{
		Kind:    KindGitHub,
		Label:   "GitHub event",
		Summary: "Fires when selected events happen in a GitHub repository.",
		Fields: []Field{
			{
				Name:      "repo",
				Label:     "Repository",
				Help:      "The owner/name whose webhook deliveries land on this Trigger.",
				Type:      FieldGitHubRepo,
				Required:  true,
				Direction: Inbound,
			},
			{
				Name:      "events",
				Label:     "Events",
				Help:      "GitHub event types to accept. Use * for all events.",
				Type:      FieldGitHubEvents,
				Required:  true,
				Direction: Inbound,
			},
			{
				Name:      "branches",
				Label:     "Branches",
				Help:      "Narrows branch-carrying events (push, create, delete). Use * for all, an exact name, or a prefix glob like release/*. Events with no branch are unaffected.",
				Type:      FieldStringList,
				Direction: Inbound,
			},
			{
				Name:      "webhook_id",
				Label:     "GitHub webhook id",
				Help:      "Set by Helix when it installs the webhook on GitHub.",
				Type:      FieldString,
				ReadOnly:  true,
				Direction: Inbound,
			},
			{
				Name:      "webhook_html_url",
				Label:     "Webhook on GitHub",
				Help:      "Set by Helix when it installs the webhook on GitHub.",
				Type:      FieldURL,
				ReadOnly:  true,
				Direction: Inbound,
			},
		},
		Activation: Activation{
			Summary: "GitHub delivers matching events to Helix. Use the Connect to GitHub panel below to install the webhook.",
		},
		Secrets: []SecretRef{{
			Label:      "GitHub token and webhook signing secret",
			SettingKey: "transport.github",
			Location:   "Organization Settings",
		}},
	}
}
```

Append to `gitlab.go`:

```go
func (gitlab) Describe() Descriptor {
	return Descriptor{
		Kind:    KindGitLab,
		Label:   "GitLab event",
		Summary: "Fires when selected events happen in a GitLab project.",
		Fields: []Field{
			{
				Name:      "repository_id",
				Label:     "Project",
				Help:      "The Helix repository record this Trigger listens to.",
				Type:      FieldGitLabRepo,
				Required:  true,
				Direction: Inbound,
			},
			{
				Name:      "events",
				Label:     "Events",
				Help:      "GitLab event types to accept.",
				Type:      FieldGitLabEvents,
				Required:  true,
				Direction: Inbound,
			},
			{
				Name:      "repo",
				Label:     "Project path",
				Help:      "The namespace/project path, recorded when the project is selected.",
				Type:      FieldString,
				ReadOnly:  true,
				Direction: Inbound,
			},
			{
				Name:      "webhook_id",
				Label:     "GitLab webhook id",
				Help:      "Set by Helix when it installs the webhook on GitLab.",
				Type:      FieldString,
				ReadOnly:  true,
				Direction: Inbound,
			},
			{
				Name:      "webhook_html_url",
				Label:     "Webhook on GitLab",
				Help:      "Set by Helix when it installs the webhook on GitLab.",
				Type:      FieldURL,
				ReadOnly:  true,
				Direction: Inbound,
			},
		},
		Activation: Activation{
			Summary: "GitLab delivers matching events to Helix.",
		},
		Secrets: []SecretRef{{
			Label:      "GitLab webhook authentication",
			SettingKey: "transport.gitlab",
			Location:   "Organization Settings",
		}},
	}
}
```

- [ ] **Step 6: Run the tests**

Run: `cd api && CGO_ENABLED=0 go test ./pkg/org/domain/transport/ -count=1 -v`
Expected: PASS, including the pre-existing transport tests.

If `TestDescriptorRequiredFieldsAreEnforcedByValidate` fails for `gitlab`'s `events`, check that `sampleFor` returns a value from GitLab's accepted set (`Merge Request Hook`, `Note Hook`, `Pipeline Hook`, `Push Hook`) — the validator rejects unknown event names.

- [ ] **Step 7: Build**

Run: `cd api && CGO_ENABLED=0 go build ./...`
Expected: success. A compile error naming a missing `Describe` method means a Kind was skipped — that is the interface doing its job.

- [ ] **Step 8: Commit**

```bash
git add api/pkg/org/domain/transport/
git commit -m "feat(org): declare per-kind trigger settings as descriptors"
```

---

### Task 2: Resolve a Descriptor's activation for one Trigger

**Files:**
- Modify: `api/pkg/org/domain/transport/descriptor.go`
- Modify: `api/pkg/org/domain/transport/descriptor_test.go`

**Interfaces:**
- Consumes: `transport.Descriptor`, `transport.Activation` (Task 1).
- Produces: `transport.ResolveActivation(d Descriptor, p ActivationParams) ResolvedActivation` and the `ActivationParams` / `ResolvedActivation` structs.

Keeping template resolution a pure function in the domain means it is table-testable without an HTTP server, and the API handler stays a thin caller.

- [ ] **Step 1: Write the failing test**

Append to `api/pkg/org/domain/transport/descriptor_test.go`:

```go
func TestResolveActivation_FillsTemplates(t *testing.T) {
	t.Parallel()

	var webhookDesc transport.Descriptor
	for _, d := range transport.DescribeAll() {
		if d.Kind == transport.KindWebhook {
			webhookDesc = d
		}
	}

	got := transport.ResolveActivation(webhookDesc, transport.ActivationParams{
		PublicURL: "https://helix.example.com",
		OrgHandle: "acme",
		TriggerID: "tr-123",
	})

	wantURL := "https://helix.example.com/api/v1/orgs/acme/webhooks/tr-123"
	if got.URL != wantURL {
		t.Errorf("URL = %q, want %q", got.URL, wantURL)
	}
	if got.Verb != "POST" {
		t.Errorf("Verb = %q, want POST", got.Verb)
	}
	if got.AuthHeader == "" {
		t.Error("AuthHeader is empty, want the API key header")
	}
}

func TestResolveActivation_FillsFieldPlaceholdersFromConfig(t *testing.T) {
	t.Parallel()

	var emailDesc transport.Descriptor
	for _, d := range transport.DescribeAll() {
		if d.Kind == transport.KindEmail {
			emailDesc = d
		}
	}

	got := transport.ResolveActivation(emailDesc, transport.ActivationParams{
		Config: map[string]any{"alias": "support"},
	})

	if got.Address != "support@<your inbound domain>" {
		t.Errorf("Address = %q, want the alias substituted", got.Address)
	}
}

func TestResolveActivation_TrailingSlashOnPublicURLDoesNotDouble(t *testing.T) {
	t.Parallel()

	var webhookDesc transport.Descriptor
	for _, d := range transport.DescribeAll() {
		if d.Kind == transport.KindWebhook {
			webhookDesc = d
		}
	}

	got := transport.ResolveActivation(webhookDesc, transport.ActivationParams{
		PublicURL: "https://helix.example.com/",
		OrgHandle: "acme",
		TriggerID: "tr-123",
	})

	if strings.Contains(got.URL, "//api/") {
		t.Errorf("URL = %q, want no doubled slash", got.URL)
	}
}

func TestResolveActivation_MissingFieldLeavesReadablePlaceholder(t *testing.T) {
	t.Parallel()

	var emailDesc transport.Descriptor
	for _, d := range transport.DescribeAll() {
		if d.Kind == transport.KindEmail {
			emailDesc = d
		}
	}

	got := transport.ResolveActivation(emailDesc, transport.ActivationParams{})

	if got.Address != "<alias>@<your inbound domain>" {
		t.Errorf("Address = %q, want an unset alias rendered as <alias>", got.Address)
	}
}
```

Add `"strings"` to that file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && CGO_ENABLED=0 go test ./pkg/org/domain/transport/ -run ResolveActivation -v`
Expected: FAIL — `undefined: transport.ResolveActivation`.

- [ ] **Step 3: Implement**

Append to `api/pkg/org/domain/transport/descriptor.go`:

```go
import (
	"fmt"
	"strings"
)

// ActivationParams carries the per-Trigger values that fill an
// Activation's templates.
type ActivationParams struct {
	PublicURL string
	OrgHandle string
	TriggerID string
	Config    map[string]any
}

// ResolvedActivation is an Activation with every template filled in for
// one specific Trigger.
type ResolvedActivation struct {
	Summary    string `json:"summary"`
	Verb       string `json:"verb,omitempty"`
	URL        string `json:"url,omitempty"`
	AuthHeader string `json:"auth_header,omitempty"`
	Address    string `json:"address,omitempty"`
	Note       string `json:"note,omitempty"`
}

// ResolveActivation fills {public}, {org}, {id} and {field:<name>} in the
// Descriptor's Activation templates. An unset field renders as <name> so
// the UI shows a readable gap rather than a raw placeholder.
func ResolveActivation(d Descriptor, p ActivationParams) ResolvedActivation {
	fill := func(tmpl string) string {
		if tmpl == "" {
			return ""
		}
		out := tmpl
		out = strings.ReplaceAll(out, "{public}", strings.TrimRight(p.PublicURL, "/"))
		out = strings.ReplaceAll(out, "{org}", p.OrgHandle)
		out = strings.ReplaceAll(out, "{id}", p.TriggerID)
		for _, f := range d.Fields {
			placeholder := "{field:" + f.Name + "}"
			if !strings.Contains(out, placeholder) {
				continue
			}
			value := "<" + f.Name + ">"
			if raw, ok := p.Config[f.Name]; ok {
				if s := fmt.Sprintf("%v", raw); s != "" {
					value = s
				}
			}
			out = strings.ReplaceAll(out, placeholder, value)
		}
		return out
	}

	return ResolvedActivation{
		Summary:    d.Activation.Summary,
		Verb:       d.Activation.Verb,
		URL:        fill(d.Activation.URLTemplate),
		AuthHeader: d.Activation.AuthHeader,
		Address:    fill(d.Activation.AddressTemplate),
		Note:       d.Activation.Note,
	}
}
```

- [ ] **Step 4: Run tests**

Run: `cd api && CGO_ENABLED=0 go test ./pkg/org/domain/transport/ -count=1 -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add api/pkg/org/domain/transport/
git commit -m "feat(org): resolve trigger activation templates per trigger"
```

---

### Task 3: Fix the webhook inbound route

**Files:**
- Modify: `api/pkg/org/interfaces/server/server.go:121`
- Modify: `api/pkg/org/interfaces/server/webhook.go:33-45`
- Modify: `api/pkg/org/interfaces/server/webhook_test.go`

**Interfaces:**
- Consumes: nothing from earlier tasks.
- Produces: the working public URL shape `POST /api/v1/orgs/{org}/webhooks/{trigger_id}`, which Task 1's webhook `Activation.URLTemplate` already promises.

Today the org mux registers `POST /webhooks/{org}/{triggerID}`. Mounted under the authRouter's `/orgs/{org}/` prefix this produces a doubled org segment, and `PathValue("org")` wins over the already-resolved context org, so passing the org slug 404s. The handler was written for two deployment modes but the empty-org case is unreachable when embedded.

- [ ] **Step 1: Write the failing test**

Append to `api/pkg/org/interfaces/server/webhook_test.go`.

This case drives the handler directly rather than through `newWebhookServer`.
That helper wraps the mux in an `httptest.Server`, and a request that travels
over the wire cannot carry an injected context — which is exactly the thing
being tested. Constructing the handler inline is three lines and is the only way
to exercise the embedded code path.

```go
// Embedded in helix the org is resolved by middleware into the request
// context and the mount prefix supplies the {org} segment, so the public
// URL carries only the Trigger id. Standalone, the org is a path
// segment. Both must reach the same Trigger.
func TestWebhookPostResolvesOrgFromContextWhenPathHasNoOrgSegment(t *testing.T) {
	t.Parallel()
	rd := &recordingRouter{}
	st := orggorm.GetOrgTestDB(t)
	bc := newTopichub(t)
	h := server.NewFromStore(st, mcptools.NewRegistry(), bc, rd, nil).Handler()
	seedTrigger(t, st, "s-inbox", transport.KindWebhook)

	req := httptest.NewRequest(http.MethodPost, "/webhooks/s-inbox", strings.NewReader("hello"))
	req.Header.Set("Content-Type", "text/plain")
	req = req.WithContext(server.WithOrgID(req.Context(), "org-test"))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body %q)", rec.Code, rec.Body.String())
	}
	if got := len(rd.snapshot()); got != 1 {
		t.Fatalf("routed %d events, want 1", got)
	}
}
```

Every symbol here already exists: `recordingRouter` and `seedTrigger` are in this
file, `newTopichub` is beside them, and `server.WithOrgID` is exported from
`orgctx.go`. No new imports are needed.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && CGO_ENABLED=0 go test ./pkg/org/interfaces/server/ -run ResolvesOrgFromContext -v`
Expected: FAIL with status 404 and body `404 page not found` — no route matches `/webhooks/{triggerID}`. A 404 whose body reads `trigger "s-inbox": not found` would mean something different (the route matched but the lookup failed); you should not see that here.

- [ ] **Step 3: Register the org-less route**

In `api/pkg/org/interfaces/server/server.go`, beside the existing registration at line 121:

```go
	mux.Handle("POST /webhooks/{org}/{triggerID}", s.webhookHandler())
	// Embedded in helix the org is already resolved into the request
	// context by the org-scope middleware and the mount prefix supplies
	// the {org} segment, so the public URL carries only the Trigger id.
	mux.Handle("POST /webhooks/{triggerID}", s.webhookHandler())
```

- [ ] **Step 4: Run the test**

Run: `cd api && CGO_ENABLED=0 go test ./pkg/org/interfaces/server/ -run Webhook -count=1 -v`
Expected: PASS, including every pre-existing webhook case (the `{org}` form still works for standalone).

- [ ] **Step 5: Verify against the live stack**

Air rebuilds the API automatically. Confirm the binary picked up the change, then:

```bash
cd /home/phil/helix
K=$(docker exec helix-postgres-1 psql -U postgres -d postgres -tAc "SELECT key FROM api_keys LIMIT 1;")
T=$(curl -s -X POST -H "Authorization: Bearer $K" -H 'Content-Type: application/json' \
     -d '{"name":"route-check","kind":"webhook","config":{}}' \
     http://localhost:8080/api/v1/orgs/test/triggers | python3 -c 'import sys,json;print(json.load(sys.stdin)["id"])')
curl -s -o /dev/null -w 'new URL with org SLUG: %{http_code}\n' -X POST \
  -H "Authorization: Bearer $K" -H 'Content-Type: application/json' -d '{"hi":1}' \
  "http://localhost:8080/api/v1/orgs/test/webhooks/$T"
curl -s -X DELETE -H "Authorization: Bearer $K" "http://localhost:8080/api/v1/orgs/test/triggers/$T"
```

Expected: `200`. Before this change the equivalent single-org URL returned 404.

- [ ] **Step 6: Commit**

```bash
git add api/pkg/org/interfaces/server/
git commit -m "fix(org): accept the org slug on the webhook inbound URL"
```

---

### Task 4: Serve descriptors and per-trigger activation over the API

**Files:**
- Modify: `api/pkg/org/interfaces/server/orgctx.go`
- Modify: `api/pkg/server/helix_org_middleware.go:285`
- Modify: `api/pkg/org/interfaces/server/api/triggers.go`
- Modify: `api/pkg/org/interfaces/server/api/api.go:319-321` (the `Routes` slice)
- Test: `api/pkg/org/interfaces/server/api/triggers_test.go`

**Interfaces:**
- Consumes: `transport.DescribeAll()`, `transport.ResolveActivation`, `transport.ActivationParams` (Tasks 1–2).
- Produces:
  - `server.WithOrgHandle(ctx, handle) context.Context` and `server.OrgHandleFromContext(ctx) string`
  - `GET /api/v1/orgs/{org}/trigger-kinds` returning `{"kinds": [Descriptor…]}`
  - `TriggerDTO.Activation *transport.ResolvedActivation` on every trigger read

**Why the org handle is needed.** `resolveOrgID` returns the canonical
`org_…` id from context, so building the activation URL from it yields
`…/orgs/org_01m0mfs4eg…/webhooks/tr-…`. That URL works — the outer `{org}`
segment goes through `lookupOrg`, which accepts an id or a slug — but it is
what the user copies and pastes, so it should read `…/orgs/acme/webhooks/…`.
The middleware already has the raw URL segment and currently discards it.
Stashing it alongside the org id is three lines and keeps the displayed URL
the one the user recognises.

`TriggerDTO.EffectivePublicURL` is unchanged — `TriggerWebhookPanel` reads it.

- [ ] **Step 1: Write the failing test**

Append to `api/pkg/org/interfaces/server/api/triggers_test.go`. It uses this
package's existing `newDeps` / `newDepsClock`, `orgapi.Handler`, `do` and
`decode` helpers, exactly as the tests already in the file do:

```go
func TestListTriggerKindsReturnsEveryKindWithLabels(t *testing.T) {
	deps, _, _ := newDeps(t)
	h := orgapi.Handler(deps)

	rec := do(t, h, http.MethodGet, "/trigger-kinds", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body)
	}

	var out orgapi.TriggerKindsResponse
	decode(t, rec, &out)

	if len(out.Kinds) != len(transport.KindValues()) {
		t.Fatalf("got %d kinds, want %d", len(out.Kinds), len(transport.KindValues()))
	}
	for i, want := range transport.KindValues() {
		if out.Kinds[i].Kind != want {
			t.Errorf("kind %d = %q, want %q", i, out.Kinds[i].Kind, want)
		}
		if out.Kinds[i].Label == "" {
			t.Errorf("kind %q has no Label", out.Kinds[i].Kind)
		}
	}
}

func TestTriggerDTOCarriesResolvedActivation(t *testing.T) {
	// newDeps pins the id generator to a constant, so two creates in one
	// test would collide on the primary key. Use an incrementing one, as
	// TestTriggerDTOPublicURLOnlyForProviderTriggers does.
	n := 0
	deps, _, _ := newDepsClock(t,
		func() time.Time { return time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC) },
		func() string { n++; return fmt.Sprintf("id-%d", n) },
	)
	deps.PublicServerURL = "https://helix.example.com"
	h := orgapi.Handler(deps)

	var wh orgapi.TriggerDTO
	decode(t, do(t, h, http.MethodPost, "/triggers", map[string]any{
		"name": "Inbox", "kind": "webhook",
	}), &wh)

	if wh.Activation == nil {
		t.Fatal("Activation is nil, want the resolved webhook recipe")
	}
	// `do` injects org "org-test" as the scope; the first minted id is
	// tr-id-1.
	want := "https://helix.example.com/api/v1/orgs/org-test/webhooks/tr-id-1"
	if wh.Activation.URL != want {
		t.Errorf("Activation.URL = %q, want %q", wh.Activation.URL, want)
	}
	if wh.Activation.Verb != "POST" {
		t.Errorf("Activation.Verb = %q, want POST", wh.Activation.Verb)
	}
	if wh.Activation.AuthHeader == "" {
		t.Error("Activation.AuthHeader is empty, want the API key header")
	}

	var loc orgapi.TriggerDTO
	decode(t, do(t, h, http.MethodPost, "/triggers", map[string]any{
		"name": "Chatter", "kind": "local",
	}), &loc)

	if loc.Activation == nil || loc.Activation.Summary == "" {
		t.Fatal("local trigger has no activation summary")
	}
	if loc.Activation.URL != "" {
		t.Errorf("local Activation.URL = %q, want empty", loc.Activation.URL)
	}
}
```

Add `"github.com/helixml/helix/api/pkg/org/domain/transport"` to that file's
imports. `fmt` and `time` are already imported for the existing tests.

The URL asserts `org-test` because `do` injects that as the scope and no org
handle is set in this harness — the handle falls back to the org id, which is
the documented behaviour.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd api && CGO_ENABLED=0 go test ./pkg/org/interfaces/server/api/ -run 'TriggerKinds|ResolvedActivation' -v`
Expected: FAIL — compile error, `undefined: orgapi.TriggerKindsResponse` and `wh.Activation undefined`.

- [ ] **Step 3: Add the org handle to context**

Append to `api/pkg/org/interfaces/server/orgctx.go`:

```go
// orgHandleKey is the unexported context key for the org handle exactly
// as it appeared in the request URL.
type orgHandleKey struct{}

// WithOrgHandle stores the raw `{org}` URL segment — a slug or an id,
// whichever the caller used. Handlers that build a URL for a human to
// copy render this rather than the canonical id, so the address they
// paste matches the one they navigated to. Both forms resolve.
func WithOrgHandle(ctx context.Context, handle string) context.Context {
	return context.WithValue(ctx, orgHandleKey{}, handle)
}

// OrgHandleFromContext returns the handle stored by WithOrgHandle, or
// empty when no middleware has set it. Callers fall back to the org id.
func OrgHandleFromContext(ctx context.Context) string {
	v, _ := ctx.Value(orgHandleKey{}).(string)
	return v
}
```

In `api/pkg/server/helix_org_middleware.go`, at the line that currently reads
`ctx := helixorgserver.WithOrgID(r.Context(), org.ID)`, add the handle beneath it:

```go
		ctx := helixorgserver.WithOrgID(r.Context(), org.ID)
		ctx = helixorgserver.WithOrgHandle(ctx, mux.Vars(r)["org"])
```

`mux` is already imported in that file.

- [ ] **Step 4: Add the DTO field, the response type, and the handler**

In `api/pkg/org/interfaces/server/api/triggers.go`, add to `TriggerDTO`
immediately after `EffectivePublicURL`:

```go
	// Activation is the resolved "how do I fire this" recipe for this
	// Trigger: concrete URL or address, verb, and auth, with every
	// template in the Kind's descriptor filled in.
	Activation *transport.ResolvedActivation `json:"activation,omitempty"`
```

Add the response type and handler in the same file:

```go
type TriggerKindsResponse struct {
	Kinds []transport.Descriptor `json:"kinds"`
}

// @Summary Helix-org: list trigger kinds and their settings
// @Tags HelixOrg
// @Produce json
// @Param org path string true "Organization ID or slug"
// @Success 200 {object} api.TriggerKindsResponse
// @Router /api/v1/orgs/{org}/trigger-kinds [get]
func (a *apiHandler) listTriggerKinds(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, TriggerKindsResponse{Kinds: transport.DescribeAll()})
}
```

In `triggerDTO`, immediately after the `EffectivePublicURL` block, resolve the
activation:

```go
	handle := helixorgserver.OrgHandleFromContext(ctx)
	if handle == "" {
		handle = orgID
	}
	for _, d := range transport.DescribeAll() {
		if d.Kind != t.Kind {
			continue
		}
		resolved := transport.ResolveActivation(d, transport.ActivationParams{
			PublicURL: strings.TrimSpace(a.deps.PublicServerURL),
			OrgHandle: handle,
			TriggerID: t.ID,
			Config:    config,
		})
		dto.Activation = &resolved
		break
	}
```

`triggerDTO` already takes `ctx` and `orgID`, and already imports `strings` and
`transport`. Add the `helixorgserver` import if the file does not have it — check
first with `grep -n helixorgserver api/pkg/org/interfaces/server/api/triggers.go`;
`api.go` imports it for `resolveOrgID`, so use the same alias.

- [ ] **Step 5: Register the route**

In `api/pkg/org/interfaces/server/api/api.go`, beside the other trigger routes:

```go
		{Pattern: "GET /trigger-kinds", Handler: http.HandlerFunc(a.listTriggerKinds)},
```

- [ ] **Step 6: Run tests and build**

Run: `cd api && CGO_ENABLED=0 go test ./pkg/org/... -count=1`
Expected: PASS.

Run: `cd api && CGO_ENABLED=0 go build ./...`
Expected: success.

- [ ] **Step 7: Regenerate the API client**

Run: `./stack update_openapi`

Confirm the generated types landed:

```bash
grep -n "TriggerKinds\|TransportDescriptor\|TransportResolvedActivation\|TransportField" frontend/src/api/api.ts | head
```

Expected: an `ApiTriggerKindsResponse`, the `TransportDescriptor` / `TransportField` /
`TransportResolvedActivation` interfaces, and a client method for the new route.
Record the exact generated names — Task 5 imports them.

- [ ] **Step 8: Verify live**

```bash
K=$(docker exec helix-postgres-1 psql -U postgres -d postgres -tAc "SELECT key FROM api_keys LIMIT 1;")
curl -s -H "Authorization: Bearer $K" http://localhost:8080/api/v1/orgs/test/trigger-kinds \
  | python3 -c 'import sys,json;[print(k["kind"],"|",k["label"],"|",len(k.get("fields",[])),"fields") for k in json.load(sys.stdin)["kinds"]]'
```

Expected: 8 rows in canonical order (`local`, `webhook`, `email`, `github`,
`gitlab`, `cron`, `slack`, `helix_events`) with human labels.

Then confirm the activation URL uses the **slug** you browsed with, not the
canonical id:

```bash
curl -s -H "Authorization: Bearer $K" http://localhost:8080/api/v1/orgs/test/triggers \
  | python3 -c 'import sys,json;[print(t["kind"],"->",t.get("activation",{}).get("url") or t.get("activation",{}).get("address") or t.get("activation",{}).get("summary","")[:60]) for t in json.load(sys.stdin)["triggers"]]'
```

Expected: webhook rows show `.../orgs/test/webhooks/tr-…`, containing `test` and
not `org_01m0…`.

- [ ] **Step 9: Commit**

```bash
git add api/pkg/org/interfaces/server/ api/pkg/server/helix_org_middleware.go frontend/src/api/
git commit -m "feat(api): expose trigger kind descriptors and activation"
```

---

### Task 5: Frontend descriptor service and config model helpers

**Files:**
- Create: `frontend/src/services/triggerKindService.ts`
- Create: `frontend/src/components/helix-org/trigger/triggerConfigModel.ts`
- Create: `frontend/src/components/helix-org/trigger/triggerConfigModel.test.ts`

**Interfaces:**
- Consumes: the generated `v1OrgsTriggerKindsDetail` client method (Task 4).
- Produces:
  - `useTriggerKinds(): UseQueryResult<TriggerKindDescriptor[]>`
  - `useTriggerKind(kind: string | undefined): TriggerKindDescriptor | undefined`
  - `type TriggerKindDescriptor = ApiTransportDescriptor`, `type TriggerField = ApiTransportField`
  - `initialDraft(desc, config): Record<string, unknown>`
  - `draftToConfig(desc, draft, existing): Record<string, unknown>`
  - `missingRequired(desc, draft): string[]`

Pure helpers live in `triggerConfigModel.ts` so they are unit-testable without rendering MUI.

- [ ] **Step 1: Write the failing test**

Create `frontend/src/components/helix-org/trigger/triggerConfigModel.test.ts`:

```ts
import { describe, expect, it } from 'vitest'
import { draftToConfig, initialDraft, missingRequired } from './triggerConfigModel'
import type { TriggerKindDescriptor } from '../../../services/triggerKindService'

const emailDesc: TriggerKindDescriptor = {
  kind: 'email',
  label: 'Incoming email',
  summary: 'Fires when mail arrives.',
  fields: [{ name: 'alias', label: 'Inbox alias', type: 'string', required: true, direction: 'inbound' }],
  activation: { summary: 'Send mail.' },
}

const githubDesc: TriggerKindDescriptor = {
  kind: 'github',
  label: 'GitHub event',
  summary: 'Fires on repo events.',
  fields: [
    { name: 'repo', label: 'Repository', type: 'github_repo', required: true, direction: 'inbound' },
    { name: 'events', label: 'Events', type: 'github_events', required: true, direction: 'inbound' },
    { name: 'webhook_id', label: 'Webhook id', type: 'string', read_only: true, direction: 'inbound' },
  ],
  activation: { summary: 'GitHub delivers events.' },
}

describe('initialDraft', () => {
  it('seeds every field from the saved config', () => {
    expect(initialDraft(emailDesc, { alias: 'support' })).toEqual({ alias: 'support' })
  })

  it('seeds list fields as arrays even when the config is empty', () => {
    expect(initialDraft(githubDesc, {})).toEqual({ repo: '', events: [], webhook_id: '' })
  })
})

describe('missingRequired', () => {
  it('names a required field left empty', () => {
    expect(missingRequired(emailDesc, { alias: '' })).toEqual(['alias'])
  })

  it('treats an empty list as missing', () => {
    expect(missingRequired(githubDesc, { repo: 'a/b', events: [] })).toEqual(['events'])
  })

  it('returns nothing when every required field is set', () => {
    expect(missingRequired(githubDesc, { repo: 'a/b', events: ['*'] })).toEqual([])
  })

  it('never demands a read-only field', () => {
    expect(missingRequired(githubDesc, { repo: 'a/b', events: ['*'], webhook_id: '' })).toEqual([])
  })
})

describe('draftToConfig', () => {
  it('drops empty optional values rather than writing empty strings', () => {
    const desc: TriggerKindDescriptor = {
      ...emailDesc,
      fields: [
        ...emailDesc.fields!,
        { name: 'note', label: 'Note', type: 'string', direction: 'inbound' },
      ],
    }
    expect(draftToConfig(desc, { alias: 'support', note: '' }, {})).toEqual({ alias: 'support' })
  })

  it('preserves server-managed keys the form never edits', () => {
    const existing = { repo: 'a/b', events: ['*'], webhook_id: 42, webhook_html_url: 'https://x' }
    const out = draftToConfig(githubDesc, { repo: 'a/c', events: ['push'], webhook_id: '42' }, existing)
    expect(out.webhook_id).toBe(42)
    expect(out.webhook_html_url).toBe('https://x')
    expect(out.repo).toBe('a/c')
  })

  it('keeps unknown keys the descriptor does not model', () => {
    const out = draftToConfig(emailDesc, { alias: 'support' }, { legacy_key: 'keep me' })
    expect(out.legacy_key).toBe('keep me')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend && yarn vitest run src/components/helix-org/trigger/triggerConfigModel.test.ts`
Expected: FAIL — cannot resolve `./triggerConfigModel`.

- [ ] **Step 3: Create the service**

Create `frontend/src/services/triggerKindService.ts`:

```ts
import { useQuery } from '@tanstack/react-query'
import { ApiTransportDescriptor, ApiTransportField } from '../api/api'
import useApi from '../hooks/useApi'
import { useHelixOrgBase } from './helixOrgService'

export type TriggerKindDescriptor = ApiTransportDescriptor
export type TriggerField = ApiTransportField

export const TRIGGER_KIND_QUERY_KEYS = {
  all: (orgID: string) => ['helix-org', orgID, 'trigger-kinds'] as const,
}

export function useTriggerKinds(options?: { enabled?: boolean }) {
  const api = useApi()
  const { orgID } = useHelixOrgBase()
  return useQuery({
    queryKey: TRIGGER_KIND_QUERY_KEYS.all(orgID),
    queryFn: async () => {
      const res = await api.getApiClient().v1OrgsTriggerKindsDetail(orgID)
      return (res.data?.kinds ?? []) as TriggerKindDescriptor[]
    },
    enabled: !!orgID && (options?.enabled ?? true),
    staleTime: 5 * 60 * 1000,
  })
}

export function useTriggerKind(kind: string | undefined) {
  const { data } = useTriggerKinds()
  return data?.find((d) => d.kind === kind)
}
```

If `./stack update_openapi` named the generated types differently, use the generated names and keep the local aliases. Confirm with `grep -n "Descriptor" frontend/src/api/api.ts | head`.

- [ ] **Step 4: Create the model helpers**

Create `frontend/src/components/helix-org/trigger/triggerConfigModel.ts`:

```ts
import type { TriggerField, TriggerKindDescriptor } from '../../../services/triggerKindService'

const LIST_TYPES = new Set(['string_list', 'github_events', 'gitlab_events'])

export function isListField(field: TriggerField): boolean {
  return LIST_TYPES.has(field.type ?? '')
}

export function initialDraft(
  desc: TriggerKindDescriptor | undefined,
  config: Record<string, unknown> | undefined,
): Record<string, unknown> {
  const draft: Record<string, unknown> = {}
  const saved = config ?? {}
  for (const field of desc?.fields ?? []) {
    const name = field.name ?? ''
    if (!name) continue
    const value = saved[name]
    if (isListField(field)) {
      draft[name] = Array.isArray(value) ? value : []
    } else {
      draft[name] = value === undefined || value === null ? '' : String(value)
    }
  }
  return draft
}

export function missingRequired(
  desc: TriggerKindDescriptor | undefined,
  draft: Record<string, unknown>,
): string[] {
  const missing: string[] = []
  for (const field of desc?.fields ?? []) {
    const name = field.name ?? ''
    if (!name || !field.required || field.read_only) continue
    const value = draft[name]
    if (Array.isArray(value) ? value.length === 0 : !String(value ?? '').trim()) {
      missing.push(name)
    }
  }
  return missing
}

export function draftToConfig(
  desc: TriggerKindDescriptor | undefined,
  draft: Record<string, unknown>,
  existing: Record<string, unknown> | undefined,
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...(existing ?? {}) }
  for (const field of desc?.fields ?? []) {
    const name = field.name ?? ''
    if (!name || field.read_only) continue
    const value = draft[name]
    if (Array.isArray(value)) {
      const cleaned = value.map((entry) => String(entry).trim()).filter(Boolean)
      if (cleaned.length) out[name] = cleaned
      else delete out[name]
      continue
    }
    const text = String(value ?? '').trim()
    if (text) out[name] = text
    else delete out[name]
  }
  return out
}
```

Read-only fields are skipped on write, so `webhook_id` and `webhook_html_url` survive from `existing` untouched.

- [ ] **Step 5: Run tests**

Run: `cd frontend && yarn vitest run src/components/helix-org/trigger/triggerConfigModel.test.ts`
Expected: PASS, 9 tests.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/services/triggerKindService.ts frontend/src/components/helix-org/trigger/
git commit -m "feat(frontend): add trigger descriptor service and config model"
```

---

### Task 6: Field renderer with the rich pickers

**Files:**
- Create: `frontend/src/components/helix-org/trigger/TriggerFieldRenderer.tsx`
- Create: `frontend/src/components/helix-org/trigger/fields/SlackWorkspacePicker.tsx`
- Create: `frontend/src/components/helix-org/trigger/fields/GitLabRepoPicker.tsx`
- Create: `frontend/src/components/helix-org/trigger/fields/GitLabEventsField.tsx`

**Interfaces:**
- Consumes: `TriggerField`, `isListField` (Task 5); the existing `CronScheduleFields`, `GitHubRepoPicker`, `GitHubEventsField`, `GitHubBranchesField`.
- Produces: `<TriggerFieldRenderer field value onChange readOnly />` handling every `FieldType` from Task 1.

- [ ] **Step 1: Build the three new pickers**

`fields/SlackWorkspacePicker.tsx` — a `Select` whose options come from `GET /orgs/{org}/slack/workspaces` via the generated client. Reuse the hook in `frontend/src/components/helix-org/SlackIntegrationsPanel.tsx` if one already lists workspaces; grep it first and do not duplicate. Render each option's team/workspace name with its id as secondary text. When the list is empty, render the Select disabled with helper text "No Slack workspaces are connected to this organization." — that is an empty-options state, not readiness gating, so it stays inside the field.

`fields/GitLabRepoPicker.tsx` — a `Select` over `useGitRepositories({ organizationId })` filtered to `TypesExternalRepositoryType.ExternalRepositoryTypeGitLab`, exactly as `TriggerFormDialog.tsx` does today. Value is the repository id. On change, also surface the chosen repo's `external_url` so the caller can store `repo` alongside `repository_id`; expose that via an `onRepoPathChange?: (path: string) => void` prop.

`fields/GitLabEventsField.tsx` — a multi-select over the four values the backend accepts, taken verbatim from `gitlab.go`'s validator: `Merge Request Hook`, `Note Hook`, `Pipeline Hook`, `Push Hook`. Render values in monospace as `GitHubEventsField` does.

- [ ] **Step 2: Build the renderer**

Create `TriggerFieldRenderer.tsx`. It switches on `field.type` and returns:

- `string` → `TextField` (size small, label, placeholder, helper text from `field.help`)
- `url` → `TextField` with `type="url"`
- `string_list` → the existing `GitHubBranchesField` shape (a free-solo `Autocomplete`); for a generic list field use the same control with no curated options
- `cron` → `CronScheduleFields` (it owns both `schedule` and `message`; see the note below)
- `github_repo` → `GitHubRepoPicker`
- `github_events` → `GitHubEventsField`
- `gitlab_repo` → `GitLabRepoPicker`
- `gitlab_events` → `GitLabEventsField`
- `slack_workspace` → `SlackWorkspacePicker`
- `slack_channel` → `TextField` with the placeholder `C012ABCDEF`

When `readOnly` is true, or `field.read_only` is set, render a label plus the value as a `Typography` caption in monospace with a `CopyButtonWithCheck` — never a disabled input. Empty read-only values render as an em dash.

**Cron is the one field that does not map 1:1.** `CronScheduleFields` takes `value`/`onChange` for the schedule and `message`/`onMessageChange` for the message, so it renders two descriptor fields at once. Handle it by having the renderer return `null` for the `message` field when the descriptor's kind is `cron`, and let the `cron` field render the whole `CronScheduleFields` block. Document that in one comment — it is a genuine external constraint, not narration.

Never render a `Direction: "outbound"` field without its direction being visible: prefix the helper text with a small `Chip` reading "Outbound".

- [ ] **Step 3: Verify it compiles**

Run: `cd frontend && yarn build`
Expected: success.

- [ ] **Step 4: Commit**

```bash
git add frontend/src/components/helix-org/trigger/
git commit -m "feat(frontend): render trigger fields from their descriptor"
```

---

### Task 7: Activation card, secrets note, and the TriggerConfig shell

**Files:**
- Create: `frontend/src/components/helix-org/trigger/TriggerActivationCard.tsx`
- Create: `frontend/src/components/helix-org/trigger/TriggerSecretsNote.tsx`
- Create: `frontend/src/components/helix-org/trigger/TriggerConfig.tsx`

**Interfaces:**
- Consumes: everything from Tasks 5–6, plus `TriggerDTO.activation` (Task 4).
- Produces: `<TriggerConfig trigger? draft? kind density mode onChange onSubmit />` with `density: 'compact' | 'full'` and `mode: 'read' | 'edit' | 'create'`.

- [ ] **Step 1: Build TriggerActivationCard**

Props: `{ activation: ApiTransportResolvedActivation; density: 'compact' | 'full' }`.

Renders a `Paper variant="outlined"` headed "How to fire this", containing:
- `activation.summary` as body text.
- When `activation.url` is set: the URL in a `MarkdownCodeBlock` with language `text`, plus `activation.auth_header` beneath it. At `density="full"` also render a ready-to-paste `curl` in a `MarkdownCodeBlock` with language `bash`:

```
curl -X POST '<url>' \
  -H 'Authorization: Bearer $HELIX_API_KEY' \
  -H 'Content-Type: application/json' \
  -d '{"hello":"world"}'
```

- When `activation.address` is set: the address in a `MarkdownCodeBlock` with language `text`.
- `activation.note` as a caption in `text.secondary`.

At `density="compact"` show only the summary and the URL or address — no curl, no note.

This card is documentation. Do not add a button that sends a request.

- [ ] **Step 2: Build TriggerSecretsNote**

Props: `{ secrets: ApiTransportSecretRef[]; orgID: string }`.

Renders nothing when `secrets` is empty. Otherwise a caption listing each `label`, with `location` and `setting_key` shown as secondary text, and a button navigating via `router.navigate('org_general', { org_id: orgID })` — the same route `TriggerWebhookPanel`'s `SettingsLink` uses.

- [ ] **Step 3: Build TriggerConfig**

Layout, top to bottom:

1. In `create` mode: the kind `Select`, sourced from `useTriggerKinds()`, excluding any descriptor with `system_managed`. Show the selected descriptor's `summary` beneath it.
2. In `edit` mode: the kind rendered as a read-only `Chip` with the descriptor's `label`, and the caption "A Trigger's source cannot be changed after it is created. Create a new Trigger to use a different source."
3. Name and description `TextField`s (create and edit modes only).
4. One `TriggerFieldRenderer` per descriptor field, editable fields before read-only ones.
5. When the descriptor has no fields: the caption "This Trigger type has no settings."
6. `TriggerActivationCard`.
7. `TriggerSecretsNote`.
8. In `edit`/`create` modes, a collapsed `Accordion` titled "Advanced (raw JSON)" containing a monospace `TextField` bound to the full config. Its helper text: "Edit keys the form above does not cover. Changes here are merged with the fields above." Editing it takes precedence for keys the descriptor does not model.

In `read` mode render values, not inputs, using the same read-only treatment as Task 6 step 2.

Validation on submit uses `missingRequired`; show one `Alert severity="error"` naming the missing field labels. Build the payload with `draftToConfig`.

- [ ] **Step 4: Verify it compiles**

Run: `cd frontend && yarn build`
Expected: success.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/helix-org/trigger/
git commit -m "feat(frontend): add TriggerConfig with activation and secrets"
```

---

### Task 8: Detail page — name prominence and TriggerConfig

**Files:**
- Modify: `frontend/src/pages/HelixOrgTriggerDetail.tsx`

**Interfaces:**
- Consumes: `TriggerConfig` (Task 7), `useTriggerKind` (Task 5), `useListHelixOrgBots` from `helixOrgService`.
- Produces: nothing downstream.

The header today makes the id the `h5` in monospace and demotes the name to a grey `body2` beneath it. That inversion is the defect.

- [ ] **Step 1: Rewrite the header**

Replace the header `Box` so it reads, in order:

- `<Typography variant="h5">{trigger.name}</Typography>` with a `Chip` beside it carrying the **descriptor's label** (`useTriggerKind(trigger.kind)?.label`), falling back to `trigger.kind` while the descriptor query is loading.
- `trigger.description` as `body2` beneath, when set.
- A metadata row in `text.secondary` `caption`: `ID <mono>{trigger.id}</mono>` with `CopyButtonWithCheck`, then the created timestamp.

- [ ] **Step 2: Replace TriggerConfiguration with TriggerConfig**

Delete the local `TriggerConfiguration` component entirely, including its per-kind `useState` ladder, `currentConfig`, `savedGuidedConfig`, and `reset`. Render `<TriggerConfig trigger={trigger} density="full" mode="edit" />` instead, wired to `useUpdateTrigger`. Keep `errorMessage` — it maps `stale_resource` and `provider_connection_required` to readable copy.

This deletes roughly 90 lines. Do not leave any of it behind (CLAUDE.md: clean up dead code).

- [ ] **Step 3: Show agent display names**

The "Agents started" section renders `attached_workers` ids as raw chips. Look each id up in `useListHelixOrgBots()` and label the chip with the bot's `name`, falling back to the id when `name` is empty (the generated type documents exactly that fallback). Put the id in a `Tooltip`.

- [ ] **Step 4: Keep the GitHub panel**

`TriggerWebhookPanel` stays exactly as it is, still rendered only for `trigger.kind === 'github'`. Do not generalise it to GitLab — that is explicitly out of scope in the spec.

- [ ] **Step 5: Verify**

Run: `cd frontend && yarn build`
Expected: success.

Then in the browser at `http://localhost:8080/orgs/test/triggers/tr-…` confirm: the **name** is the headline, the kind chip reads "Webhook" not `webhook`, the id sits in the metadata row and copies, the settings show `outbound_url` marked Outbound, and the activation card shows the URL from Task 3.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/pages/HelixOrgTriggerDetail.tsx
git commit -m "feat(frontend): lead the trigger detail page with the name"
```

---

### Task 9: Create drawer, side pane, and list page

**Files:**
- Modify: `frontend/src/components/helix-org/TriggerFormDialog.tsx`
- Modify: `frontend/src/components/helix-org/TriggerDetailDrawer.tsx`
- Modify: `frontend/src/pages/HelixOrgTriggers.tsx`

**Interfaces:**
- Consumes: `TriggerConfig` (Task 7), `useTriggerKind`/`useTriggerKinds` (Task 5).
- Produces: nothing downstream.

- [ ] **Step 1: Rewrite TriggerFormDialog**

Delete every per-kind `useState` (`ghRepo`, `ghEvents`, `ghBranches`, `glRepositoryID`, `cronSchedule`, `cronMessage`, `emailAddress`, `config`) and the whole per-kind `submit` ladder. The body becomes `<TriggerConfig density="full" mode="create" onSubmit={onSubmit} />`.

Keep the `GitHubAppConnect` gate: when `kind === 'github'` and the app is not installed, render it above the fields. It is a connect affordance that already exists, not new readiness gating.

This is what fixes email and slack: the form now renders `alias` and `service_connection_id` because the descriptor says so, instead of a hand-written field list that disagreed with the backend.

- [ ] **Step 2: Enrich TriggerDetailDrawer**

Keep `HelixOrgOverviewCard` for the header, but stop passing the name twice — the drawer `title` is already the name, so pass the descriptor `label` as the card `title` and keep `id` as the card's small caption (the card already renders it at `caption`/0.78 opacity, which is the right weight).

Below the card add `<TriggerConfig trigger={trigger} density="compact" mode="read" />`, then keep the existing "Recent events" list.

- [ ] **Step 3: Update the list page**

Three changes to `HelixOrgTriggers.tsx`:

- The `kind` cell renders the descriptor's `label` instead of the raw enum string. Load descriptors once via `useTriggerKinds()` and build a `Map` from kind to label in a `useMemo`; fall back to the raw kind while loading.
- In the `name` cell, keep the id but drop it to `0.65rem` at `text.disabled` so it reads as secondary metadata rather than a competing identity.
- Add an **Edit** `MenuItem` above Delete in the row `Menu`, with a `Pencil` icon from `lucide-react` at `size={20}`, opening `TriggerFormDialog` in edit mode for `currentTrigger`.

`TriggerFormDialog` already accepts an optional `trigger` prop for edit mode; wire the existing `create`/`useUpdateTrigger` mutations to the right branch.

- [ ] **Step 4: Verify**

Run: `cd frontend && yarn build && yarn test`
Expected: build succeeds; existing vitest suites pass.

- [ ] **Step 5: Commit**

```bash
git add frontend/src/components/helix-org/ frontend/src/pages/HelixOrgTriggers.tsx
git commit -m "feat(frontend): render trigger drawer, pane, and list from descriptors"
```

---

### Task 10: Stop the MCP tool advertising `s-` ids (optional)

**Files:**
- Modify: `api/pkg/org/interfaces/mcptools/triggers.go:60-77`

**Interfaces:**
- Consumes: nothing.
- Produces: nothing downstream.

The spec marks this optional. It changes no existing row; it stops *new* agent-created Triggers getting `s-` ids while UI-created ones get `tr-`. Skip this task entirely if the reviewer prefers not to touch the tool surface.

- [ ] **Step 1: Remove the id argument**

In `createTriggerArgs`, delete the `ID` field. In `CreateTrigger.Description()`, delete the sentence "Pass `id` as a short readable handle (e.g. `s-releases`) or omit it to have one minted." Leave `triggers.CreateParams.ID` alone — the application layer still supports an explicit id for the system reconcilers that need deterministic ids.

- [ ] **Step 2: Run the tests**

Run: `cd api && CGO_ENABLED=0 go test ./pkg/org/... -count=1`
Expected: PASS. If a test pins the tool description verbatim, update its expected string.

- [ ] **Step 3: Commit**

```bash
git add api/pkg/org/interfaces/mcptools/triggers.go
git commit -m "refactor(org): stop create_trigger advertising s- id handles"
```

---

### Task 11: End-to-end verification in the browser

**Files:** none modified. This task produces evidence, not code, plus one
verification record at `design/2026-08-24-trigger-config-ux-verification.md`.

**Interfaces:**
- Consumes: everything Tasks 1–10 built.
- Produces: nothing downstream.

CLAUDE.md is explicit: a change is not done until it has been exercised end-to-end, and a unit test asserting a state change is not evidence the feature works. Every claim below needs observed output.

- [ ] **Step 1: Confirm the stack is healthy**

```bash
docker compose -f docker-compose.dev.yaml ps
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080
```

Expected: `helix-api-1`, `helix-frontend-1`, `helix-postgres-1` all `Up`, and `200`. If containers are still starting, poll for several minutes rather than concluding the stack is broken.

- [ ] **Step 2: Prove the two broken kinds now create from the UI**

In the browser, open `http://localhost:8080/orgs/test/triggers` and click **New Trigger**.

- Choose **Incoming email**. Confirm the form shows an **Inbox alias** field, not "Incoming email address". Enter `uxprobe` and create. Expected: success, not `alias is required`.
- Choose **Slack event**. Confirm the form shows a **Slack workspace** dropdown listing the connected workspace. Select it and create. Expected: success, not `service_connection_id is required`.

Confirm in the database:

```bash
docker exec helix-postgres-1 psql -U postgres -d postgres -c \
  "SELECT id, name, kind, config FROM org_triggers WHERE name LIKE 'uxprobe%' OR kind IN ('email','slack') ORDER BY created_at DESC LIMIT 5;"
```

Expected: the email row's config contains `alias`, the slack row's contains `service_connection_id`.

- [ ] **Step 3: Walk every kind across all three surfaces**

For each of `local`, `webhook`, `cron`, `email`, `slack`, `github`, `gitlab`, open the list page, the org-chart side pane, and the detail page. For each, confirm: the name leads, the kind reads as a human label, the settings that exist are shown with their current values, and the activation card answers "how do I fire this".

Record any kind where a surface still shows a bare `{}` textarea as the only settings UI — that is a regression against the spec's goal.

- [ ] **Step 4: Fire the webhook using only what the UI shows**

Copy the payload URL from the webhook Trigger's detail page and POST to it with an API key. Do not hand-edit the URL.

```bash
K=$(docker exec helix-postgres-1 psql -U postgres -d postgres -tAc "SELECT key FROM api_keys LIMIT 1;")
curl -s -X POST -H "Authorization: Bearer $K" -H 'Content-Type: application/json' \
  -d '{"hello":"from the copied url"}' '<PASTE THE URL THE UI SHOWED>'
```

Expected: `200` with an event id. Then reload the detail page and confirm the event appears under Event history. This is the single most important check in the plan: it proves the URL the product displays is the URL that works.

- [ ] **Step 5: Confirm read-only config survives a save**

On the GitHub Trigger's detail page (which has `webhook_id` and `webhook_html_url` set once its webhook is installed), change the branches list and save. Then:

```bash
docker exec helix-postgres-1 psql -U postgres -d postgres -c \
  "SELECT config FROM org_triggers WHERE kind='github' LIMIT 1;"
```

Expected: `webhook_id` and `webhook_html_url` are still present and unchanged. This is the regression `draftToConfig` exists to prevent.

- [ ] **Step 6: Clean up the probes**

Delete the `uxprobe` Triggers created in step 2 through the UI, and confirm the list returns to its prior contents.

- [ ] **Step 7: Record the results**

Write what was actually observed to `design/2026-08-24-trigger-config-ux-verification.md`: which kinds were exercised on which surfaces, the webhook round-trip output, and anything that did not work. State plainly what was **not** tested — for example, GitLab cannot be exercised without a connected GitLab repository, and email delivery cannot be exercised without a configured Postmark account. Do not write "covered by unit tests" for anything only a unit test touched.

- [ ] **Step 8: Commit**

```bash
git add design/2026-08-24-trigger-config-ux-verification.md
git commit -m "docs: record trigger config UX end-to-end verification"
```

---

## Notes for the executor

- **The email and slack fixes are not separate tasks.** They fall out of Task 9 step 1: once the create form renders fields from the descriptor, it necessarily sends `alias` and `service_connection_id`. If they still fail after Task 9, the descriptor and the validator disagree — go back to Task 1 and check the parity test.
- **`TriggerWebhookPanel` is untouched.** It already does its job well. Resist generalising it to GitLab; the spec puts that out of scope because it carries a live status check.
- **Do not add a test-event button** anywhere, however tempting the webhook endpoint makes it. The `curl` in the activation card is documentation and is the whole of what was agreed.
- **If a task reveals the descriptor is missing a field a kind actually accepts**, add it to that kind's `Describe()` in the same commit and note it — the descriptor being complete is what everything else rests on.
