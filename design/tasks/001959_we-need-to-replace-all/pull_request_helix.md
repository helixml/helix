# Compose-based runner: foundation layer

Foundation for replacing the runner infrastructure with operator-defined
Docker Compose profiles per the design at
`helix-specs/design/tasks/001959_we-need-to-replace-all/`.

This PR ships the load-bearing backend pieces that everything else
(runner-binary rewrite, `helix_openai_client.go` repoint, frontend,
deletions) depends on, with full unit-test coverage. It does **not**
yet rewire inference paths, delete the existing scheduler/runner code,
or modify the frontend — those land in subsequent PRs.

## Summary

- New `RunnerProfile` and `RunnerAssignment` GORM types, AutoMigrate-registered.
- New `composeparse` package: extracts model list + GPU count from a runner profile's compose YAML, handling NVIDIA + AMD declaration styles, all five port forms, `--served-model-name` preferred over `--model` basename. Validated against all five committed sample profiles.
- New `gpuarch` package: shared NVIDIA-compute-capability and AMD-`gfx*` → canonical architecture string mapping. Used by both runner (label its GPUs) and API server (validate profile fit).
- New `profile` package: parse-on-save service over the store, plus `Compatibility(req, gpus)` constraint check (count → vendor → architecture → model_match → min_vram, fail-fast in declared order) and `FilterCompatible()` helper for the dropdown filter endpoint.
- New `runnerrouter` package: replaces the scheduler's request-routing role. `PickRunner(model)` round-robins across runners whose active profile contains the model and are `running`. Per-model counters; `NoRunnerError` carries the available-models list for useful 503s; `AvailableModels()` for `/v1/models`.
- Five admin HTTP endpoints for runner-profile CRUD, all going through the parse-on-save service so the YAML-derived metadata invariant holds.
- Five sample profiles in `design/sample-profiles/` covering: 8xH100 production, any-Blackwell-4-GPU, any-NVIDIA-dev, AMD MI300X, and the dev-spike-tiny profile sized for a 16 GiB shared dev GPU.

## Changes

- `api/pkg/types/runner_profile.go` — new types: `GPUVendor`, `ProfileModel`, `ProfileGPURequirement`, `RunnerProfile`, `RunnerAssignment`.
- `api/pkg/store/store.go`, `store_runner_profiles.go`, `store_mocks.go` — store-level CRUD for both new tables.
- `api/pkg/store/postgres.go` — register new types in `AutoMigrate`.
- `api/pkg/system/uuid.go` — `RunnerProfilePrefix=rprof_` and `GenerateRunnerProfileID()`.
- `api/pkg/runner/composeparse/` — parser + 13 unit tests + sample-profile validation test.
- `api/pkg/runner/gpuarch/` — canonical arch mapping + table-driven tests.
- `api/pkg/runner/profile/` — service with `Create`/`Update`/`Get`/`GetByName`/`List`/`Delete` + compatibility check + `FilterCompatible` + 14 unit tests.
- `api/pkg/runnerrouter/` — `Router` + 9 unit tests.
- `api/pkg/server/runner_profile_handlers.go`, `server.go` — admin HTTP routes.
- `design/sample-profiles/` — five YAML profiles + README.

## What this PR doesn't do (deliberate scope cuts)

- Does not delete `api/pkg/scheduler/`, the existing custom runtimes (`vllm_runtime.go` etc.), `memory_estimation_handlers.go`, or anything else from AC8. Deletion is a separate change set after the new code is wired into the inference path.
- Does not modify `api/pkg/openai/helix_openai_client.go` — the repoint to the new router needs the router wired into the `HelixAPIServer` struct first, which involves more surgery.
- Does not implement the runner-binary rewrite (`compose_manager.go`, `proxy.go`, narrowed `controller_nats.go`, etc.). This needs GPU hardware to validate end-to-end.
- Does not implement the assign-profile / clear-profile / compatible-profiles HTTP routes. Those need `RunnerStatus` to carry `vendor` and `architecture` per-GPU, which is wired with the runner-binary rewrite (see AC2 in requirements.md).
- Does not touch the frontend — `RunnerProfilesTable.tsx` and `EditRunnerProfile.tsx` come in a separate PR once these endpoints are exercised against the dev stack.

## Test plan

- [ ] `go build ./api/pkg/types/ ./api/pkg/store/ ./api/pkg/runner/gpuarch/ ./api/pkg/runner/composeparse/ ./api/pkg/runner/profile/ ./api/pkg/runnerrouter/ ./api/pkg/server/` — clean.
- [ ] `go test ./api/pkg/runner/gpuarch/ ./api/pkg/runner/composeparse/ ./api/pkg/runner/profile/ ./api/pkg/runnerrouter/` — all green (36 tests).
- [ ] AutoMigrate creates `runner_profiles` and `runner_assignments` tables on first start (verify in DB).
- [ ] `POST /api/v1/runner-profiles` with one of `design/sample-profiles/dev-spike-tiny.yaml` succeeds and returns a `rprof_*` ID.
- [ ] Same compose YAML, derived `models` and `gpu_requirement.count` match what `composeparse` extracts.
- [ ] `PUT /api/v1/runner-profiles/{id}` with a different YAML re-derives the metadata.
- [ ] `DELETE /api/v1/runner-profiles/{id}` returns 204; subsequent `GET` returns 404.

## Design references

- Requirements: `helix-specs/design/tasks/001959_we-need-to-replace-all/requirements.md`
- Design: `helix-specs/design/tasks/001959_we-need-to-replace-all/design.md` (especially Decisions 1, 2, 6, 9, 11; AC1a + AC9–AC12)
- Tasks: `helix-specs/design/tasks/001959_we-need-to-replace-all/tasks.md`
