# Design: Project YAML Import

## Summary
Extend `helix apply -f` to support a new `kind: Project` YAML format, allowing users to define and share Helix projects as code.

## YAML Format

```yaml
apiVersion: helix.ml/v1alpha1
kind: Project
metadata:
  name: my-awesome-project
spec:
  description: "Full-stack todo app with React and Node.js"
  
  # Primary repository - startup script at .helix/startup.sh
  repositories:
    - url: github.com/myorg/my-repo
      primary: true
      branch: main
    - url: github.com/myorg/shared-libs
      branch: main
  
  # Project-level AI guidelines
  guidelines: |
    Use TypeScript for all new code.
    Follow existing patterns in the codebase.
  
  # Project-level skills (MCP servers, etc.)
  skills:
    mcp:
      drone-ci:
        url: http://localhost:8080/mcp
  
  # Initial prompts shown in Launchpad / can create tasks
  initial_prompts:
    - prompt: "Add dark mode toggle with localStorage persistence"
      priority: medium
      labels: [frontend, ui]
    - prompt: "Fix the bug where deleted items reappear after refresh"
      priority: high
      labels: [backend, bug]
```

## Architecture

### Processing Flow
```
YAML file → ProcessYAMLConfig → kind check → ProjectConfig → API client → Project created
```

### Key Components

1. **Types** (`api/pkg/types/project.go`):
   - Add `ProjectConfig` struct (mirrors AppHelixConfig pattern)
   - Add `ProjectConfigCRD` with apiVersion, kind, metadata, spec

2. **Config Processor** (`api/pkg/config/yaml_processor.go`):
   - Extend `ProcessYAMLConfig` to detect `kind: Project`
   - Return appropriate typed config based on kind

3. **CLI** (`api/pkg/cli/project/apply.go`):
   - New `helix project apply -f` subcommand
   - OR extend existing `helix apply -f` to auto-detect kind

4. **API Client** (`api/pkg/client/project.go`):
   - `CreateProjectFromConfig(config *ProjectConfig)` method
   - Handles repo cloning, guideline setting, skill configuration

### Reuse from SimpleSampleProject

The existing `SimpleSampleProject` struct already has most fields we need:
- `Skills *types.AssistantSkills`
- `Guidelines string`
- `RequiredGitHubRepos []RequiredGitHubRepo`
- `TaskPrompts []SampleTaskPrompt`

We can reuse these types directly in the new `ProjectConfig` spec.

## Key Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| YAML format | CRD-style with kind | Consistent with agent YAML, extensible |
| CLI command | `helix project apply -f` | Clear separation, matches `helix model apply` pattern |
| Self-referential repos | Allowed | Common use case: "import this repo as a project" |
| Initial prompts | Optional, not auto-created | Keep YAML declarative; let user decide to create tasks |

## API Changes

### New Endpoint
`POST /api/v1/projects/apply` - Create or update project from config

Request body: `ProjectConfig` (the spec portion of the YAML)

### Existing Endpoint Changes
None required - uses existing project create/update endpoints internally.

## Migration Path
No migration needed. This is additive functionality.