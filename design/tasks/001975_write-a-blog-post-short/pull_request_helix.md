# Draft: blog post on working with the garage door up

## Summary

Adds a short (~730 word) blog post draft for helix.ml/blog about Helix's commitment to building in the open. Riffs on Andy Matuschak's "Work with the garage door up" essay and uses the spec task system itself as the central piece of evidence — every feature ships with a public requirements/design/tasks trio on the `helix-specs` branch.

The post highlights four recent spec tasks (001959, 001972, 001956, 001962) with direct links to their design docs, picked to show different facets of building in public: a mid-implementation architectural pivot, agents proposing their own work breakdowns, an honestly-named "Round 3" retry, and a small UX change with outsized effect.

## Changes

- New file: `design/2026-05-01-blog-garage-door-up.md` — blog post draft following the existing convention in `design/` (same format as `2026-03-15-blog-off-by-one-ai-responses.md` and earlier blog drafts).

## Notes for reviewers

- Voice was calibrated against recent posts on https://helix.ml/blog (notably "Off-by-One Bug…" and "Adaptive Bitrate Hubris") — em dashes, mixed first-person, conversational-but-technical, slightly self-deprecating.
- Spec task descriptions and the GitHub URL pattern were both verified before committing.
- Draft only — actual publishing to helix.ml is owned by another pipeline and was explicitly out of scope.
- Easy to swap any of the four highlighted spec tasks if the reviewer would prefer a different mix.
