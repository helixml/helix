# Design: Declarative Project YAML

## Architecture Overview

A single `Project` kind replaces both the earlier `Project` and `ProjectTemplate` concepts. The file is self-contained: it describes the project and its default agent inline. The same YAML works for `helix apply -f` and `kubectl apply -f`.

---

## Current State

### `helix apply`
- `api/pkg/cli/app/apply.go` — applies AIApp only; no `kind:` dispatch
- `api/pkg/cli/model/apply.go` — separate command for `Model` kind
- `api/pkg/config/yaml_processor.go` — CRD detection but always returns `AppHelixConfig`

### Agent types (`api/pkg/types/`)
- `AppHelixConfig` — top-level app: name, description, assistants
- `AssistantConfig` — the agent: model, provider, system_prompt, knowledge, APIs, MCPs, built-in tools, etc.
- The internal `AssistantConfig` is large and exposes many fields that are internal or advanced. We don't expose it directly in the YAML.

### Sample projects
- Hardcoded slice `SAMPLE_PROJECTS` in `api/pkg/server/sample_projects_handlers.go`
- `SampleSpecProject` type in `api/pkg/types/simple_spec_task.go` — has `TaskPrompts`, good foundation

### K8s Operator
- Only `AIApp` CRD in `operator/api/v1alpha1/`
- Known bugs in `aiapp_controller.go`: GPTScript loop, Knowledge not converted
- No status reporting; no org scoping

---

## Design Decisions

### Decision 1: Single `Project` kind, agent is inline

No separate `ProjectTemplate` kind. A project YAML is just a project YAML. Sharing it as a template is a separate `--template` flag concern, not a different file format.

The `agent` block inside `spec` maps to a Helix App (with one assistant). On apply, we:
1. Create/update the project
2. Create/update the app (keyed by agent name)
3. Set the app as the project's `DefaultHelixAppID`

**Rationale:** Users shouldn't need to know internal IDs. The YAML is self-contained.

### Decision 2: Custom simplified `ProjectAgentSpec` type — not `AssistantConfig`

We define a new type `ProjectAgentSpec` specifically for use in project YAMLs. It exposes only the fields a user would set when creating an agent:

```go
type ProjectAgentSpec struct {
    Name        string            `yaml:"name"`
    Description string            `yaml:"description"`
    Model       string            `yaml:"model"`       // required
    Provider    string            `yaml:"provider"`    // optional, auto-detected
    SystemPrompt string           `yaml:"system_prompt"`
    Tools       ProjectAgentTools `yaml:"tools"`
    MCPs        []AssistantMCP    `yaml:"mcps"`        // reuse existing type - already simple
    Knowledge   []AssistantKnowledge `yaml:"knowledge"` // reuse existing type
}

type ProjectAgentTools struct {
    WebSearch  bool `yaml:"web_search"`
    Browser    bool `yaml:"browser"`
    Calculator bool `yaml:"calculator"`
}
```

A conversion function `ProjectAgentSpec → AppHelixConfig` handles the mapping to internal types. This keeps the YAML user-friendly while the internal types stay unchanged.

**Why reuse `AssistantMCP` and `AssistantKnowledge` directly?** These are already well-structured and user-facing. The MCP config is inherently detailed (transport, command, args, env) and can't be simplified further. Knowledge source config is similarly concrete.

**What's intentionally omitted from `ProjectAgentSpec`:** temperature, topP, max tokens, reasoning/generation model splitting, code agent runtime, Zapier, AzureDevOps, email, project manager tool, tests. These can be added later if needed.

### Decision 3: `--template` flag for registering org sample projects

Adding `--template` to `helix apply` stores the project as an org-scoped sample project entry (using the existing `SampleSpecProject` / sample projects DB table). Without the flag, it creates a live project. This is cleaner than a separate `ProjectTemplate` kind.

### Decision 4: Idempotency keys

| Resource | Key |
|---|---|
| Project | `(metadata.name, organization_id)` |
| Agent (App) | `(spec.agent.name, organization_id)` |
| K8s Project | `k8s.<namespace>.<name>` |

### Decision 5: Operator `Project` CRD mirrors CLI YAML schema

The operator CRD `spec` matches the YAML `spec` one-to-one. This means:
- The same `ProjectSpec` Go type can be used for both CLI parsing and the operator CRD (or at least kept in sync)
- The operator reconciler calls the same Helix API endpoints as `helix apply`

---

## File-by-File Changes

### `api/` changes

| File | Change |
|---|---|
| `api/pkg/types/project.go` | Add `ProjectCRD`, `ProjectSpec`, `ProjectAgentSpec`, `ProjectAgentTools` types |
| `api/pkg/cli/app/apply.go` | Add `kind:` dispatcher; add `applyProject()` function |
| `api/pkg/cli/app/apply.go` | `applyProject()`: parse project YAML → create/update project → create/update agent app → link |
| `api/pkg/client/project.go` | New: `CreateProject`, `UpdateProject`, `GetProjectByName` client methods |
| `api/pkg/client/client.go` | Add project methods to `Client` interface |
| `api/pkg/server/sample_projects_handlers.go` | Add `PUT /api/v1/sample-projects/simple` upsert endpoint for `--template` flag |

### `operator/` changes

| File | Change |
|---|---|
| `operator/api/v1alpha1/project_types.go` | New: `Project`, `ProjectSpec`, `ProjectStatus`, `ProjectList` CRD types |
| `operator/api/v1alpha1/aiapp_types.go` | Add `OrganizationID` to `AIAppSpec`; add fields to `AIAppStatus` |
| `operator/internal/controller/aiapp_controller.go` | Fix GPTScript bug; fix Knowledge conversion; add status updates; add TLS skip verify; add org scoping |
| `operator/internal/controller/project_controller.go` | New: reconciler — upsert project + agent via Helix API |
| `operator/cmd/main.go` | Register `ProjectReconciler` |
| `operator/config/crd/bases/` | Regenerated CRD manifests (`make manifests`) |
| `operator/api/v1alpha1/zz_generated.deepcopy.go` | Regenerated (`make generate`) |

---

## YAML Schema

```yaml
apiVersion: helix.ml/v1alpha1
kind: Project
metadata:
  name: my-project              # required; idempotency key within org
spec:
  description: "..."            # optional
  github_repo_url: "..."        # optional
  default_branch: main          # optional, defaults to "main"
  technologies: []              # optional
  guidelines: |                 # optional; AI agent guidelines
    ...
  agent:                        # optional; omit to create project without an agent
    name: "Project Assistant"   # optional; defaults to "<project-name> Assistant"
    description: "..."          # optional
    model: claude-sonnet-4-6    # required if agent block present
    provider: anthropic         # optional; auto-detected from model name
    system_prompt: |            # optional
      ...
    tools:                      # optional; all default to false
      web_search: false
      browser: false
      calculator: false
    mcps:                       # optional; reuses AssistantMCP type directly
      - name: github
        transport: stdio
        command: npx
        args: ["-y", "@modelcontextprotocol/server-github"]
        env:
          GITHUB_TOKEN: "${GITHUB_TOKEN}"
    knowledge:                  # optional; reuses AssistantKnowledge type
      - name: project-docs
        source:
          web:
            urls: ["https://docs.example.com"]
```

---

## Conversion: `ProjectAgentSpec` → `AppHelixConfig`

```
ProjectAgentSpec.Name        → AppHelixConfig.Name
ProjectAgentSpec.Description → AppHelixConfig.Description
ProjectAgentSpec             → AppHelixConfig.Assistants[0]:
  .Model                     → AssistantConfig.Model
  .Provider                  → AssistantConfig.Provider
  .SystemPrompt              → AssistantConfig.SystemPrompt
  .Tools.WebSearch           → AssistantConfig.WebSearch.Enabled
  .Tools.Browser             → AssistantConfig.Browser.Enabled
  .Tools.Calculator          → AssistantConfig.Calculator.Enabled
  .MCPs                      → AssistantConfig.MCPs (direct, same type)
  .Knowledge                 → AssistantConfig.Knowledge (direct, same type)
```

---

## Operator Bug Fix Details

**GPTScript bug** (current code):
```go
for _, zapier := range assistant.Zapier {  // BUG: should be assistant.GPTScripts
    helixAssistant.Zapier = append(...)    // BUG: appending to wrong field
}
```

**Fix:**
```go
for _, script := range assistant.GPTScripts {
    helixAssistant.GPTScripts = append(helixAssistant.GPTScripts, types.AssistantGPTScript{
        Name: script.Name, File: script.File, Content: script.Content,
    })
}
for _, k := range assistant.Knowledge {
    helixAssistant.Knowledge = append(helixAssistant.Knowledge, types.AssistantKnowledge{
        Name: k.Name, /* ... map remaining fields */
    })
}
```

**Status fields to add to `AIAppStatus`:**
```go
type AIAppStatus struct {
    Ready      bool   `json:"ready"`
    AppID      string `json:"appID,omitempty"`
    LastSynced string `json:"lastSynced,omitempty"`
    Message    string `json:"message,omitempty"`
}
```

---

## Patterns Found in Codebase

- `helix apply` for AIApps uses app name as idempotency key — mirror this for Project (name + org)
- K8s operator names resources as `k8s.<namespace>.<name>` — Project CRD follows same convention
- `AssistantMCP` is already clean and reusable; no need to re-wrap it for YAML
- Built-in sample projects hardcoded in server — `--template` flag adds DB-backed org templates alongside them
- `AppHelixConfig.Assistants` is always a slice; a project agent maps to index `[0]` with one assistant
