# Implementation Tasks

## Types & Config Processing
- [ ] Add `ProjectConfig` struct to `api/pkg/types/project.go` with fields: name, description, repositories, guidelines, skills, initial_prompts
- [ ] Add `ProjectConfigCRD` struct with apiVersion, kind, metadata, spec pattern
- [ ] Add `ProjectRepository` struct for repository references (url, primary, branch)
- [ ] Add `ProjectPrompt` struct reusing fields from `SampleTaskPrompt` (prompt, priority, labels)
- [ ] Extend `ProcessYAMLConfig` in `api/pkg/config/yaml_processor.go` to detect `kind: Project` and return a generic interface or separate function

## CLI Implementation
- [ ] Create `api/pkg/cli/project/` package with root command
- [ ] Implement `helix project apply -f <file>` command
- [ ] Add `--organization` flag for org-scoped projects
- [ ] Handle create vs update based on project name match
- [ ] Print project ID on success

## API Client
- [ ] Add `CreateProjectFromConfig(ctx, config *ProjectConfig) (*Project, error)` to client interface
- [ ] Add `UpdateProjectFromConfig(ctx, projectID string, config *ProjectConfig) (*Project, error)` to client interface
- [ ] Implement repository cloning flow (check OAuth, clone repos, set primary)

## Server/API
- [ ] Add `POST /api/v1/projects/apply` endpoint (or reuse existing create with config detection)
- [ ] Implement config-to-project conversion logic
- [ ] Handle initial_prompts → optional task creation (flag-controlled)
- [ ] Set guidelines and skills from config

## Testing
- [ ] Unit test for YAML parsing with `kind: Project`
- [ ] Unit test for project config validation
- [ ] Integration test: apply new project from YAML
- [ ] Integration test: apply updates existing project

## Documentation
- [ ] Add example `helix.yaml` to `helix/sample/`
- [ ] Update CLI help text with project apply examples