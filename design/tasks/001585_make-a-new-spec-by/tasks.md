# Implementation Tasks

## Types (`api/pkg/types/project.go`)

- [ ] Add `ProjectStartup` struct (`Install`, `Start`)
- [ ] Add `Startup *ProjectStartup` to `ProjectSpec`
- [ ] Add `StartupInstall`, `StartupStart` to `Project` DB model
- [ ] Add `ProjectRepositorySpec` struct (`URL`, `Branch`, `Primary` bool)
- [ ] Add `ProjectKanban` struct with `WIPLimits *ProjectWIPLimits`
- [ ] Add `ProjectWIPLimits` struct (`Planning`, `Implementation`, `Review` int)
- [ ] Add `ProjectTaskSpec` struct (`Title` string, `Description` string)
- [ ] Add `Repository *ProjectRepositorySpec` (singular shorthand) to `ProjectSpec`
- [ ] Add `Repositories []ProjectRepositorySpec` to `ProjectSpec`
- [ ] Add `Kanban *ProjectKanban` to `ProjectSpec`
- [ ] Add `Tasks []ProjectTaskSpec` to `ProjectSpec`
- [ ] Add `ValidateRepositories() error` method on `ProjectSpec`
- [ ] Add `ResolvedRepositories() []ProjectRepositorySpec` method (normalises singular/plural → always returns a slice)

## Store (`api/pkg/store/`)

- [ ] Check if `ListGitRepositories` supports URL filtering; if not, add `GetGitRepositoryByExternalURL(ctx, orgID, url)` helper

## Server (`api/pkg/server/project_handlers.go`)

- [ ] Add `PUT /api/v1/projects/apply` endpoint (idempotent upsert by name+org)
- [ ] Wire `startup` fields through on create and update
- [ ] Call `spec.ValidateRepositories()` in `applyProject` handler — return 400 on validation error
- [ ] For each resolved repository: find-or-create `GitRepository` by `ExternalURL`, attach to project with `AttachRepositoryToProject`
- [ ] Set primary repository with `SetProjectPrimaryRepository` after all repos are attached
- [ ] Map `spec.Kanban.WIPLimits` → `project.Metadata.BoardSettings.WIPLimits` on create and update
- [ ] For each task in `spec.Tasks`: list existing project tasks, create task in Planning column only if title not already present (idempotent by title)

## CLI (`api/pkg/cli/app/apply.go`)

- [ ] Add `kind:` dispatcher routing `Project` to `runApplyProject()`
- [ ] `runApplyProject()` creates/updates agent app and calls `ApplyProject`
- [ ] `ApplyProject` client method
- [ ] Agent idempotency: look up by name, update or create
- [ ] Verify `Repositories` and `Kanban` flow through `ProjectApplyRequest.Spec` automatically (no extra wiring needed beyond types)

## K8s Operator — Bug fixes

- [ ] Fix GPTScript conversion bug
- [ ] Fix Knowledge source conversion
- [ ] Add `HELIX_TLS_SKIP_VERIFY` support
- [ ] Add `OrganizationID` to `AIAppSpec`
- [ ] Add status fields to `AIAppStatus`

## K8s Operator — Project CRD

- [ ] `Project`, `ProjectSpec`, `ProjectStatus`, `ProjectList` types
- [ ] Scheme registration
- [ ] Full reconciler with finalizer, status reporting
- [ ] Registration in `operator/cmd/main.go`
- [ ] Deepcopy methods in `zz_generated.deepcopy.go`
- [ ] Improve `reconcileProjectAgent`: use `Status.AgentAppID` first (fast path), fall back to name search
- [ ] Add `ProjectRepositorySpec`, `ProjectKanban`, `ProjectWIPLimits`, `ProjectTaskSpec` to operator `ProjectSpec` CRD type
- [ ] Map operator `Repositories`, `Kanban`, and `Tasks` through to `ProjectApplyRequest` in reconciler
- [ ] Update deepcopy for new slice/pointer fields
- [ ] Run `make generate manifests` (requires `controller-gen`)

## Examples

- [ ] `examples/project.yaml` with startup block
- [ ] Update `examples/project.yaml` to show multi-repo + kanban + tasks

## Testing setup

- [ ] Install `kind`: `go install sigs.k8s.io/kind@latest`
- [ ] Install `kubectl`: download binary or snap
- [ ] Install `controller-gen`: `go install sigs.k8s.io/controller-tools/cmd/controller-gen@latest`
- [ ] Verify `helix apply` end-to-end: build CLI, apply project YAML against `localhost:8080`, check idempotency
- [ ] Verify `kubectl apply` end-to-end: `kind create cluster`, install CRDs, run operator locally, apply project YAML, check `kubectl get projects` status
