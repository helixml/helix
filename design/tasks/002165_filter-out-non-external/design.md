# Design: Filter Spec Task Agent Switcher to External, Spec-Task-Owned Agents

## Summary

This is a **small, frontend-only** change. The agent switcher on the spec task details
page is populated by a memoized `sortedApps` value that currently *sorts* external agents
first but keeps everything. We change it to *filter* the list down to eligible agents.

## Key Files (from codebase research)

- `frontend/src/components/tasks/SpecTaskDetailContent.tsx`
  - `sortedApps` memo (~lines 238–263) builds the dropdown list from `apps.apps`.
  - `<AgentDropdown ... agents={sortedApps} ... />` (~lines 1451–1461) renders it.
  - `selectedAgent` / `handleAgentChange` (~lines 888–911) persist the choice via
    `helix_app_id`.
- `frontend/src/components/agent/AgentDropdown.tsx` — presentational `Select`; no change needed.
- `frontend/src/contexts/apps.tsx` — loads apps from `GET /api/v1/apps`; no change needed.
- `frontend/src/types.ts`
  - `AGENT_TYPE_ZED_EXTERNAL = 'zed_external'` (line 86) — the "external" marker.
  - `IApp` has `global: boolean`, `owner_type: IOwnerType` where
    `IOwnerType = 'user' | 'system' | 'org'`.

## How to identify each category

**External agent** (keep): an app where any assistant has
`agent_type === AGENT_TYPE_ZED_EXTERNAL`, OR `config.helix.default_agent_type === AGENT_TYPE_ZED_EXTERNAL`.
This is the exact predicate already used to compute `zedExternalApps` today.

**Owned by Helix org** (exclude): there is **no hard-coded Helix org id/slug** in the
codebase. The available, reliable signal for first-party / built-in agents is:
`app.global === true` OR `app.owner_type === 'system'`. Agents created by the
user/org for spec tasks have `global === false` and `owner_type` of `'user'` or `'org'`.
We treat `global || owner_type === 'system'` as "owned by the Helix org" and exclude them.

## Decision: filter, don't sort

Replace the current "split into zedExternal + others, then concatenate" logic with a
single filter:

```ts
const isExternal = (app: IApp) =>
  app.config?.helix?.assistants?.some(a => a.agent_type === AGENT_TYPE_ZED_EXTERNAL) ||
  app.config?.helix?.default_agent_type === AGENT_TYPE_ZED_EXTERNAL;

const isHelixOrgOwned = (app: IApp) =>
  app.global === true || app.owner_type === 'system';

const eligibleApps = useMemo(() => {
  const list = (apps.apps ?? []).filter(
    app => isExternal(app) && !isHelixOrgOwned(app),
  );
  // Keep the currently-assigned agent visible even if it would be filtered out.
  if (selectedAgent && !list.some(a => a.id === selectedAgent)) {
    const current = apps.apps?.find(a => a.id === selectedAgent);
    if (current) list.unshift(current);
  }
  return list;
}, [apps.apps, selectedAgent]);
```

`eligibleApps` then replaces `sortedApps` as the `agents` prop. (Rename or keep the
variable name — either is fine as long as it's consistent.)

## Edge cases & rationale

- **Currently-assigned agent excluded by filter** — re-add it (US-2) so the dropdown
  never renders an empty/invalid selection.
- **No hard-coded Helix org** — research confirmed no `HELIX_ORG_ID` constant exists.
  `global`/`owner_type === 'system'` is the closest correct signal for first-party agents
  and is already exposed on `IApp` to the frontend. If product later defines an explicit
  Helix org id, swap `isHelixOrgOwned` to use it; the predicate is isolated for that reason.
- **Empty result** — `AgentDropdown` already tolerates an empty `agents` array (US-3).

## Testing

- Manual: open a spec task details page; confirm the dropdown lists only external,
  non-global/non-system agents; confirm a previously-assigned (now-filtered) agent still
  shows and stays selected; confirm switching still persists `helix_app_id`.
- If a unit/component test exists for `sortedApps`, update it; otherwise a small test of
  the filter predicates is sufficient.
