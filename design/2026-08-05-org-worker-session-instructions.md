# Org-worker session instructions

## Failure

Org-worker runtime instructions were written to the worker repository's
`helix-specs` branch, while desktop startup read them from the project's primary
repository. A worker project with an attached user repository therefore exited
workspace setup before launching Zed. Project-scoped `HELIX_WORKER_ID` also made
ordinary SpecTask sessions enter the org-worker startup path.

## Boundary

Worker identity and resolved instructions are ephemeral session runtime state,
not repository content or project configuration.

- The spawner stores `OrgWorkerID` and `RuntimeInstructions` on only the worker's
  exploratory session.
- Hydra sends `AGENTS.md` and `CLAUDE.md` as pre-start workspace files and writes
  them into the session bind mount before starting the desktop container.
- Warm activations update the same session state and live files before clearing
  the ACP thread.
- Activation prompts contain only the trigger plus a short instruction-file
  reload hint.
- SpecTask sessions have no worker bootstrap state and receive no worker files.
- The legacy project secret is filtered during startup and removed on the next
  worker-project reconciliation.

No runtime instruction content is committed to Git.

## Verification

- Unit tests cover fresh and warm spawner propagation, org-worker versus
  SpecTask scoping, legacy secret filtering, atomic workspace materialization,
  and session metadata refresh.
- Live verification must start one org worker and one SpecTask in the same
  multi-repository project. The worker must launch Zed with matching native
  instruction files; the SpecTask must launch without those files or
  `HELIX_WORKER_ID`.

Live verification on 2026-08-05 used b-alex's multi-repository project:

- Org-worker session `ses_01kz8gccz50110gva8r2t535kx` started with matching
  root instruction files and connected Zed thread
  `019fd106-b943-79e2-b096-3628696a11c3`.
- SpecTask `spt_01kz8ggzahr17sh4tpm2a78851` started session
  `ses_01kz8ghr0x66ymgysskcpywh65` without the files or worker environment and
  connected Zed thread `019fd109-71a6-75c2-8b2e-cb902f377d6c`.
