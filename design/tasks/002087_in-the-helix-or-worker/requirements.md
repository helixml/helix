# Requirements: Worker Detail Right-Rail Links Open in New Tab

## Background

The Worker Detail page in the Helix-OR section
(`/home/retro/work/helix/frontend/src/pages/HelixOrgWorkerDetail.tsx`)
shows a right-hand side rail with metadata about the worker. Three of
those fields — **Role**, **Project**, and **Agent** — are clickable and
currently navigate the user away from the worker view in the *same* tab.

That is friction: the user is usually orienting themselves to the worker
and wants to peek at its role / project / agent without losing their
place. They asked for these to open in a new browser tab, with a small
"open in new" icon (a square with an arrow) next to each link so the
behaviour is discoverable before the click.

## User Stories

1. **As an org admin viewing a worker**, I want to click the Role / Project /
   Agent links and have them open in a new browser tab, so the worker
   detail page stays open in my current tab for reference.

2. **As an org admin**, I want a visible "open in new tab" affordance
   (the standard square-with-arrow icon) next to each of these links so
   I know in advance that clicking them won't navigate me away.

## Scope

In scope — three links in the right rail of `HelixOrgWorkerDetail.tsx`:

- **Role** (line 366–373) — navigates to `helix_org_role_detail`
- **Project** (line 379–386) — navigates to `org_project-specs`
- **Agent** (line 392–399) — navigates to `org_agent`

Out of scope:

- The "Reports to" parent ID field (line 353–360) — it is plain text, not
  a link today, and the user did not ask for it.
- Any other navigable element on this page (back arrow, fire-worker
  button, accordion actions).
- Any other page in the application.

## Acceptance Criteria

- [ ] Clicking the **Role** value on the worker detail page opens the
      role detail page in a new browser tab; the current tab still shows
      the worker detail page.
- [ ] Clicking the **Project** value opens the project specs page in a
      new browser tab; the current tab is unchanged.
- [ ] Clicking the **Agent** value opens the agent page in a new browser
      tab; the current tab is unchanged.
- [ ] Each of the three values is rendered with a small
      `OpenInNew`-style icon (square + arrow) immediately adjacent to the
      ID text, indicating new-tab behaviour.
- [ ] Middle-click and Ctrl/Cmd-click behave like a normal anchor (browser
      handles them natively — we don't intercept).
- [ ] The new icon and link styling match existing usage in
      `CreateProviderEndpointDialog.tsx` (icon at `fontSize: 14`, inline-flex
      with the text) so the visual language is consistent.
- [ ] The label-above-value layout (caption "Role" / "Project" / "Agent"
      on top, monospace ID below) is preserved.
