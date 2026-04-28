# RunPod-backed integration test harness

End-to-end tests for the sandbox-absorbs-runner architecture across the
GPU form factors Helix supports. Provisions one RunPod pod per matrix
entry, applies a runner profile, runs seven scenarios, tears the pod
down. See **Decision 14** in
`helix-specs/design/tasks/001959_we-need-to-replace-all/design.md` for
the full design.

## Why RunPod

Cheapest on-demand GPU rental with API access that covers the matrix we
care about: consumer Ada through datacenter Hopper / Blackwell, plus AMD
MI300X. Alternative providers (Lambda Labs, Crusoe, AWS spot, Azure NDv5,
GCP A3) are all viable — RunPod is just the fastest to ship against. If
we switch later, only the code under `internal/provision/` changes.

## Form factors

See `matrix.yaml`. Currently:

| Entry        | Hardware                  | Validates |
|--------------|---------------------------|-----------|
| `rtx4090`    | 1× RTX 4090 24 GiB        | "any-nvidia-dev" + dev-spike-tiny |
| `h100-sxm-1x`| 1× H100 80 GiB SXM        | FP8 chat (single Hopper) |
| `h100-sxm-4x`| 4× H100 80 GiB SXM        | tensor-parallel layouts |
| `a100-80gb-1x` | 1× A100 80 GiB PCIe     | Ampere arch (no FP8) |
| `mi300x-1x`  | 1× AMD MI300X 192 GiB     | AMD device passthrough + ROCm |
| `blackwell`  | (deferred — RunPod availability) | Blackwell paths |

## Scenarios per entry

1. `boot_smoke` — sandbox connects to API, heartbeat lands, GPU inventory matches.
2. `compatibility_filter` — `GET compatible-profiles` includes the assigned profile.
3. `assignment_apply` — assign-profile, wait for `running`, all services healthy.
4. `inference_roundtrip` — chat completion + embeddings via the API.
5. `profile_switch` — assign a different compatible profile, confirm clean swap.
6. `clear_profile` — clear-profile, confirm idle state.
7. `incompatible_rejection` — assign a profile that needs a different arch, confirm 422.

## Cost controls

- **30 min soft / 35 min hard** wall-clock per entry. RunPod pods are
  also created with `terminationMinutes: 35` so a stuck harness can't
  leak GPU spend.
- **Result cache** at `.runpod-it-cache/` keyed on `(entry-id +
  profile-yaml-sha + harness-build-sha)`. Cache hits are reported as
  green-by-cache without provisioning. 7-day stale cutoff.
- **Parallelism cap** (default 4 concurrent pods, override with
  `--parallel`).
- **Daily $ budget** (default $200, override with `--max-daily-usd`).
  Harness queries RunPod's billing API at start; refuses to schedule
  if today's spend already exceeds.

## Usage

```bash
# Real run
export RUNPOD_API_KEY=...
export HELIX_API_URL=https://test.helix.example.com
export RUNNER_TOKEN=...
export HELIX_TEST_ADMIN_TOKEN=...
go run ./integration-test/runpod/cmd/runpod-it

# Plan only (no provisioning)
go run ./integration-test/runpod/cmd/runpod-it --dry-run

# Just one entry
go run ./integration-test/runpod/cmd/runpod-it --only rtx4090

# Force a re-run, ignoring the cache
go run ./integration-test/runpod/cmd/runpod-it --no-cache

# Custom parallelism + budget
go run ./integration-test/runpod/cmd/runpod-it --parallel 2 --max-daily-usd 50
```

## Outputs

- `runpod-it.xml` — JUnit XML, one test case per (entry × scenario).
- `runpod-it.md` — Markdown summary suitable for posting to a PR comment
  or Slack.

## CI integration

Triggered nightly on `main` and on-demand by adding `[runpod-it]` to a
commit message. Not run on every PR — too expensive. The Drone secret
`RUNPOD_API_KEY` is added when the harness is first wired into CI.

## Why we don't run this on every PR

A single full-matrix run costs ~$5–20 depending on GPU rental prices
that day. We run it nightly on `main` to catch regressions, and by tag
on release branches. PR-time validation relies on the unit-test suite
plus the `dev-spike-tiny` smoke that any developer can run on a
single-GPU dev machine without RunPod.

## Skip an entry temporarily

Set `enabled: false` on that entry in `matrix.yaml`.

## Add a new form factor

Add an entry to `matrix.yaml`. The `runpod_gpu_type` string must match
RunPod's catalogue exactly (run `curl -H "Authorization: Bearer $RUNPOD_API_KEY"
https://api.runpod.io/v2/gpus` to see).
