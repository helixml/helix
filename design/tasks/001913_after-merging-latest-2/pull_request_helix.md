# Document interactive variant of ACP thread entity routing bug

## Summary

Companion design note to `2026-04-24-acp-thread-entity-routing-after-restart.md`. Documents the interactive variant of the same split-brain that was reported after the latest two Zed merges: clicking the currently-open thread in the new agents sidebar caused the panel to detach from the live `Entity<AcpThread>` while Helix kept receiving events. The actual fix lives in the corresponding Zed PR; this note is documentation only.

## Changes

- `design/2026-05-06-acp-thread-entity-routing-on-sidebar-click.md` — root-cause writeup, fix summary, and rebase note covering both fixes (the restart one from `d7be64fad1` and the interactive one in the Zed PR for `001913`).

## Notes

- No code change in this repo. `sandbox-versions.txt`'s `ZED_COMMIT=` will be bumped in a separate follow-up PR after the Zed PR merges (per the helix `CLAUDE.md` "Bumping sandbox-versions.txt" rule).

Release Notes:

- N/A
