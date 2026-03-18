# Implementation Tasks

## Types (`api/pkg/types/project.go`)

- [x] Add `ProjectStartup` struct (`Install`, `Start`)
- [x] Add `Startup *ProjectStartup` to `ProjectSpec`
- [x] Add `StartupInstall`, `StartupStart` to `Project` DB model
- [ ] Add `ProjectRepositorySpec` struct (`URL`, `Branch`, `Primary` bool)
- [ ] Add `ProjectKanban` struct with `WIPLimits *ProjectWIPLimits`
- [ ] Add `ProjectWIPLimits` struct (`Planning`, `Implementation`, `Review` int)
- [ ] Add `Repository *ProjectRepositorySpec` (singular shorthand) to `ProjectSpec`
- [ ] Add `Repositories []ProjectRepositorySpec` to `ProjectSpec`
- [ ] Add `Kanban *ProjectKanban` to `ProjectSpec`
- [ ] Add `ValidateRepositories() error` method on `ProjectSpec`
- [ ] Add `ResolvedRepositories() []ProjectRepositorySpec` method (normalises singular/plural → always returns a slice)

## Store (`api/pkg/store/`)

- [ ] Check if `ListGitRepositories` supports URL filtering; if not, add `GetGitRepositoryByExternalURL(ctx, orgID, url)` helper

## Server (`api/pkg/server/project_handlers.go`)

- [x] Add `PUT /api/v1/projects/apply` endpoint (idempotent upsert by name+org)
- [x] Wire `startup` fields through on create and update
- [ ] Call `spec.ValidateRepositories()` in `applyProject` handler — return 400 on validation error
- [ ] For each resolved repository: find-or-create `GitRepository` by `ExternalURL`, attach to project with `AttachRepositoryToProject`
- [ ] Set primary repository with `SetProjectPrimaryRepository` after all repos are attached
- [ ] Map `spec.Kanban.WIPLimits` → `project.Metadata.BoardSettings.WIPLimits` on create and update

## CLI (`api/pkg/cli/app/apply.go`)

- [x] Add `kind:` dispatcher routing `Project` to `runApplyProject()`
- [x] `runApplyProject()` creates/updates agent app and calls `ApplyProject`
- [ ] Verify `Repositories` and `Kanban` flow through `ProjectApplyRequest.Spec` automatically (no extra wiring needed beyond types)

## `helix apply` — Project kind (completed)

- [x] Kind dispatcher + `runApplyProject()`
- [x] `ApplyProject` client method
- [x] Agent idempotency: look up by name, update or create

## K8s Operator — Bug fixes (completed)

- [x] Fix GPTScript conversion bug
- [x] Fix Knowledge source conversion
- [x] Add `HELIX_TLS_SKIP_VERIFY` support
- [x] Add `OrganizationID` to `AIAppSpec`
- [x] Add status fields to `AIAppStatus`

## K8s Operator — Project CRD

- [x] `Project`, `ProjectSpec`, `ProjectStatus`, `ProjectList` types
- [x] Scheme registration
- [x] Full reconciler with finalizer, status reporting
- [x] Registration in `operator/cmd/main.go`
- [x] Deepcopy methods in `zz_generated.deepcopy.go`
- [x] Improve `reconcileProjectAgent`: use `Status.AgentAppID` first (fast path), fall back to name search
- [ ] Add `ProjectRepositorySpec`, `ProjectKanban`, `ProjectWIPLimits` to operator `ProjectSpec` CRD type
- [ ] Map operator `Repositories` and `Kanban` through to `ProjectApplyRequest` in reconciler
- [ ] Update deepcopy for new slice/pointer fields
- [ ] Run `make generate manifests` (requires `controller-gen`)

## Examples

- [x] `examples/project.yaml` with startup block
- [ ] Update `examples/project.yaml` to show multi-repo + kanban

## Testing setup

- [ ] Install `kind`: `go install sigs.k8s.io/kind@latest`
- [ ] Install `kubectl`: download binary or snap
- [ ] Install `controller-gen`: `go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest`
- [ ] Verify `helix apply` end-to-end: build CLI, apply project YAML against `localhost:8080`, check idempotency
- [ ] Verify `kubectl apply` end-to-end: `kind create cluster`, install CRDs, run operator locally, apply project YAML, check `kubectl get projects` status
