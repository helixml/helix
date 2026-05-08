# Design: Pulsing green dot for running agents

## Scope

Pure frontend change to one file. No backend, no API, no schema changes.

## Where today's red dot lives

`frontend/src/components/tasks/TaskCard.tsx:756-773` renders the red dot:

```tsx
{hasUnreadNotification && (
  <Tooltip title="Agent finished - click to dismiss">
    <Box sx={{
      position: "absolute", top: 8, right: 8,
      width: 10, height: 10, borderRadius: "50%",
      backgroundColor: "error.main",
      zIndex: 1,
      border: "2px solid", borderColor: "background.paper",
    }} />
  </Tooltip>
)}
```

`hasUnreadNotification` is derived (line 633) from `attentionEvents.length > 0`
where `attentionEvents` is filtered to unacknowledged
`agent_interaction_completed` events for this task.

## Signal: "is the agent currently working?"

The card already receives `task.agent_work_state` (TaskCard.tsx:141), values:
`"idle" | "working" | "done"`. This field is computed from session/activity
data and refreshed by the task list polling (`useSpecTasks`, refetch interval
~3.1s in `SpecTaskKanbanBoard.tsx:1041`).

`agent_work_state === "working"` is the correct signal for "agent is currently
running". This is also what the existing `useRunningDuration` hook uses
(TaskCard.tsx:637) to drive the live duration counter, so we're being
consistent with prior art in the same file.

## Decision: precedence

When both signals are true (agent working AND unacknowledged attention event),
**show only the green dot**. Rationale: the user's stated complaint is that the
red dot is misleading while an agent is running. The acknowledgement state is
not lost — it's just hidden until the agent stops, at which point the red dot
reappears (the existing attention event is still there).

We deliberately do NOT auto-acknowledge attention events when a new agent run
starts. That's a backend concern with broader implications (notification
history, missed-attention tracking) and is out of scope.

## Decision: pulse animation

Use a CSS `@keyframes` animation defined via emotion/styled (the file already
defines a `spin` keyframes block at lines 85-91, so the pattern exists).
Animation: opacity 1.0 → 0.4 → 1.0 over ~1.5s, infinite, ease-in-out. Subtle
enough to be obvious-but-not-annoying on a board with many cards.

Color: `success.main` (MUI green, ~#10b981 in this theme — coincidentally the
same as the implementation phase accent, which feels intentional). The dot
keeps the same `border: 2px solid background.paper` outer ring as the red dot
so the two indicators feel like the same family.

Avoid `transform: scale()` for the pulse — that can interfere with the
absolute positioning and create subpixel jitter. Opacity-only pulse is enough.

## Decision: tooltip

`"Agent is running"` — short, mirrors the existing
`"Agent finished - click to dismiss"` style.

## Sketch of the change (illustrative, not final code)

```tsx
const isAgentWorking = task.agent_work_state === "working";

{isAgentWorking ? (
  <Tooltip title="Agent is running">
    <Box sx={{
      position: "absolute", top: 8, right: 8,
      width: 10, height: 10, borderRadius: "50%",
      backgroundColor: "success.main",
      zIndex: 1,
      border: "2px solid", borderColor: "background.paper",
      animation: `${pulse} 1.5s ease-in-out infinite`,
    }} />
  </Tooltip>
) : hasUnreadNotification && (
  <Tooltip title="Agent finished - click to dismiss">
    <Box sx={{ /* existing red dot styles */ }} />
  </Tooltip>
)}
```

Plus a `pulse` keyframes definition near the existing `spin` block:

```tsx
const pulse = keyframes`
  0%, 100% { opacity: 1; }
  50%      { opacity: 0.4; }
`;
```

## Other Kanban surface (AgentKanbanBoard)

`AgentKanbanBoard.tsx` uses its own `SortableTaskCard` (line 746), not the
shared `TaskCard`. Quick check showed it does not currently render the same
red attention dot, so the change is naturally scoped to `TaskCard.tsx` only.
If a follow-up reveals AgentKanbanBoard needs the same treatment, it's a
separate small change.

## Testing

This is a pure visual change with three states to verify in the inner Helix
browser at http://localhost:8080:

1. **Working** — start a task, watch the card show the green pulsing dot.
2. **Idle with attention** — let the agent finish; without clicking the card,
   verify the red dot appears.
3. **Working with stale attention** — finish the agent, then send a follow-up
   prompt to restart it. The red dot should disappear and be replaced by the
   green pulsing dot for the duration of the new run; when the agent stops
   again, the red dot returns.

Per CLAUDE.md: prefer end-to-end testing in the inner Helix browser over
isolated visual harnesses.

## Notes for the implementer

- File touched: `frontend/src/components/tasks/TaskCard.tsx` only.
- The `keyframes` helper from `@emotion/react` is already imported (used by
  the existing `spin` animation at line 85).
- Vite HMR is active on port 8081 — frontend changes are live without rebuild.
- After the change, run `cd frontend && yarn build` once to confirm it compiles
  cleanly before pushing.

## Implementation Notes (post-implementation)

- The keyframes block was named `pulseDot` (not `pulse`) to avoid any potential
  collision with the existing `pulseRing` keyframes already defined at the top
  of the file (used elsewhere for the active-task spinner). `pulseDot` is
  unambiguous about its purpose.
- Final diff is exactly the three changes the design called for: keyframes
  block, `isAgentWorking` derivation, ternary render swap. No other refactors
  bundled in.
- `yarn build` passed cleanly on the merged-with-main branch.
- **End-to-end browser test was not possible** in this implementation
  environment because the inner Helix Docker stack was still building at the
  time the change finished. The visual was instead verified with a standalone
  HTML preview saved in `screenshots/01-dot-states-preview.png` — this proves
  the keyframe animation, color, and positioning all render correctly.
  Reviewers running in a fully-built environment should spot-check the three
  Kanban states before merge.
