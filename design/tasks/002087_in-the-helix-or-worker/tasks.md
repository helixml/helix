# Implementation Tasks: Worker Detail Right-Rail Links Open in New Tab

- [ ] In `frontend/src/pages/HelixOrgWorkerDetail.tsx`, add imports: `Link` from `@mui/material`, `OpenInNewIcon` from `@mui/icons-material/OpenInNew`, and the default `router` from `../router`.
- [ ] Replace the **Role** `<Button>` (lines ~366–373) with a MUI `<Link>` whose `href` is `router.buildPath('helix_org_role_detail', { org_id: orgSlug, role_id: data.role.id })`, `target="_blank"`, `rel="noopener noreferrer"`, and an inline `<OpenInNewIcon sx={{ fontSize: 14 }} />` appended after the role id text.
- [ ] Replace the **Project** `<Button>` (lines ~379–386) with a MUI `<Link>` whose `href` is `router.buildPath('org_project-specs', { org_id: orgSlug, id: projectID })`, `target="_blank"`, `rel="noopener noreferrer"`, with the trailing `OpenInNewIcon`. Preserve the existing `fontSize: 0.7rem`, monospace, `wordBreak: 'break-all'` styling.
- [ ] Replace the **Agent** `<Button>` (lines ~392–399) with a MUI `<Link>` whose `href` is `router.buildPath('org_agent', { org_id: orgSlug, app_id: agentAppID })`, `target="_blank"`, `rel="noopener noreferrer"`, with the trailing `OpenInNewIcon`. Preserve existing 0.7rem monospace styling.
- [ ] Guard each `<Link>` render with the same condition the old `<Button>` used (`orgSlug && ...`) so links only render when the org slug is available — otherwise fall back to plain text or skip render to avoid generating a broken href.
- [ ] Run `cd frontend && yarn build` and fix any TypeScript or build errors.
- [ ] Test manually in the inner Helix at `http://localhost:8080`: register / log in, navigate to a Worker detail page, verify clicking Role / Project / Agent each opens a new browser tab to the correct page and leaves the worker detail tab intact. Verify the OpenInNew icon is visible next to each value.
- [ ] Verify middle-click and Ctrl/Cmd-click on each link also open in a new tab (native browser behaviour, no JS interception).
- [ ] Commit with a conventional-commit message (e.g. `feat(frontend): open worker right-rail links in new tab`) and push.
