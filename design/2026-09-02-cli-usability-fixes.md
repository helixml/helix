# CLI usability fixes

Several CLI workflows had related usability inconsistencies.

- `spectask exec` advertised arbitrary commands but the desktop still enforced a
  benchmark-era command allowlist.
- `spectask files` and `download` treated the primary Git repository as the
  session work root, hiding deliverables written beside the checkout.
- `spectask start`, `stop`, and `send` bypassed task-aware or asynchronous APIs
  that already model the requested behavior.
- artifact and secret commands did not share project-name resolution.
- the generic secret API accepted project IDs without validating the project or
  the caller's access.
- attachment upload locking contradicted the late-upload staging path, which is
  explicitly safe after a task session starts and notifies the running agent.

The implementation should preserve the repository-scoped browser file view,
while allowing the CLI to request the full `/home/retro/work` tree under update
authorization. Project references should resolve across the caller's personal
and organization projects and reject ambiguous names. Project-scoped secret
validation belongs in the API even when the CLI uses the dedicated project
route, because raw API clients must not create orphaned rows.
