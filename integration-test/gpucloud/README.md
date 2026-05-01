# GPU-cloud integration test harness

End-to-end validation of the helix sandbox-absorbs-runner architecture
against the customer's actual deployment-target hardware. Provisions one
cloud instance per matrix entry, applies a runner profile, runs seven
scenarios, tears the instance down. See **Decision 14** in
`helix-specs/design/tasks/001959_we-need-to-replace-all/design.md` for
the full design.

## Why two providers

The customer deployment has both NVIDIA and AMD GPUs, and our sandbox
needs Docker-in-Docker (Hydra spawns inner desktops; compose-manager
spawns inference services). After evaluating six self-serve GPU clouds
in April 2026 (RunPod, Lambda, TensorDock, Crusoe, Vultr, Vast.ai), the
landscape was: datacenter-grade GPUs are container-only or sales-gated;
real VMs only at hyperscalers (slow setup) or two specialists.

So we use both:

| Provider | Used for | Why |
|---|---|---|
| **Hot Aisle** (admin.hotaisle.app) | AMD MI300X | Only self-serve cloud with on-demand MI300X stock as real VMs ($1.99/GPU/hr) |
| **Verda** (api.verda.com, formerly DataCrunch) | NVIDIA L40S, A100 80GB | KVM VMs with full root, cheapest on-demand among self-serve providers ($0.91 L40S, $1.29 A100 per GPU/hr) |

Both ship `Provisioner` implementations behind the
`internal/provision.Multi` dispatcher; the matrix entry's `provider:`
field picks which one provisions it.

## Customer deployment matrix

See `matrix.yaml`. Currently:

| Entry | Hardware | Provider | $/hr |
|---|---|---|---|
| `node1-a100-4x`   | 4× A100 80GB SXM4 | Verda     | $5.16  |
| `node2-l40s-4x`   | 4× L40S           | Verda     | $3.64  |
| `node3-l40s-4x`   | 4× L40S           | Verda     | $3.64  |
| `node4-l40s-4x`   | 4× L40S           | Verda     | $3.64  |
| `node5-mi300x-8x` | 8× MI300X 192GB   | Hot Aisle | $15.92 |
| **Total**         |                    |          | **~$32/hr → ~$16 per 30-min full pass** |

Three identical L40S nodes validate that multiple sandboxes registered
to the same API behave correctly under the inference router's
round-robin dispatch.

## Scenarios per entry

1. `boot_smoke` — sandbox connects to API, heartbeat lands, GPU inventory matches.
2. `compatibility_filter` — `GET compatible-profiles` includes the assigned profile.
3. `assignment_apply` — assign-profile, wait for `running`, all services healthy.
4. `inference_roundtrip` — chat completion + embeddings via the API.
5. `profile_switch` — assign a different compatible profile, confirm clean swap.
6. `clear_profile` — clear-profile, confirm idle state.
7. `incompatible_rejection` — assign a profile that needs a different arch, confirm 422.

## Cost controls

- **30 min soft / 35 min hard** wall-clock per entry. Cloud-init also
  schedules a `shutdown -h now` at +35 min so a stuck harness can't
  leak GPU spend.
- **Result cache** at `.gpucloud-it-cache/` keyed on `(entry-id +
  profile-yaml-sha + harness-build-sha)`. Cache hits report
  green-by-cache without provisioning. 7-day stale cutoff.
- **Parallelism cap** (default 4 concurrent instances, override with
  `--parallel`).
- **Daily $ budget** (default $200, override with `--max-daily-usd`).
  Harness queries each provider's billing API at start; refuses to
  schedule if combined spend exceeds.

## Usage

```bash
# Real run — needs both providers' keys
export HOTAISLE_API_KEY=...
export HOTAISLE_TEAM=helixml
export VERDA_API_KEY=...
export VERDA_SSH_KEY_ID=...
export HELIX_API_URL=https://test.helix.example.com
export RUNNER_TOKEN=...
go run ./integration-test/gpucloud/cmd/gpucloud-it

# Plan only (no provisioning, no creds needed)
go run ./integration-test/gpucloud/cmd/gpucloud-it --dry-run

# Just the AMD entry — only HOTAISLE_* required
go run ./integration-test/gpucloud/cmd/gpucloud-it --only node5-mi300x-8x

# Just the NVIDIA L40S entries — only VERDA_* required
go run ./integration-test/gpucloud/cmd/gpucloud-it --only node2-l40s-4x,node3-l40s-4x,node4-l40s-4x

# Force a re-run, ignoring the cache
go run ./integration-test/gpucloud/cmd/gpucloud-it --no-cache

# Custom parallelism + budget
go run ./integration-test/gpucloud/cmd/gpucloud-it --parallel 2 --max-daily-usd 50
```

## Outputs

- `gpucloud-it.xml` — JUnit XML, one test case per (entry × scenario).
- `gpucloud-it.md` — Markdown summary suitable for a PR comment or Slack.

## CI integration

Triggered nightly on `main` and on-demand by adding `[gpucloud-it]` to a
commit message. Not run on every PR — too expensive. Drone secrets
`HOTAISLE_API_KEY`, `HOTAISLE_TEAM`, `VERDA_API_KEY`, `VERDA_SSH_KEY_ID`
are added when the harness is first wired into CI.

## Why not run on every PR

A single full-matrix run costs ~$16. We run it nightly on `main` to
catch regressions, and by tag on release branches. PR-time validation
relies on the unit-test suite plus the local-GPU `dev-spike-tiny` smoke
that any developer can run on a single-GPU dev machine without paying
for cloud provisioning.

## Skip an entry temporarily

Set `enabled: false` on that entry in `matrix.yaml`.

## Add or change a node spec

Edit `matrix.yaml`. The `instance_type` string must match the chosen
provider's catalogue:

- **Hot Aisle:** Use `hotaisle vm types` (CLI from
  `https://github.com/hotaisle/hotaisle-cli`) or
  `GET /teams/{team}/virtual_machines/available/`.
- **Verda:** Use the Verda console's instance picker or
  `GET https://api.verda.com/v1/instance-types`.
