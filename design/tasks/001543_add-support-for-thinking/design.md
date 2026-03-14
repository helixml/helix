# Design: Add Support for Thinking Tags in SpecTask Chat

## Current Architecture

The SpecTask chat rendering path is:

```
SpecTaskDetailContent
  └─ EmbeddedSessionView
       └─ Interaction
            └─ InteractionInference
                 └─ MessageWithToolCalls
                      └─ Markdown (MessageProcessor)
                           └─ ThinkingWidget (when <think> tags found)
```

The `Markdown` component (`frontend/src/components/session/Markdown.tsx`) already handles `<think>...</think>` tags via `processThinkingTags()` and renders them as `ThinkingWidget`. This pipeline is fully wired for SpecTask sessions.

## Root Cause (Confirmed)

Tag name mismatch: Claude Code outputs `<thinking>...</thinking>` but `processThinkingTags()` only matches `<think>` / `</think>`. The tags pass through unprocessed and render as raw XML text in the chat.

## Solution

Extend `processThinkingTags()` in `Markdown.tsx` to normalise `<thinking>` → `<think>` and `</thinking>` → `</think>` before the existing parsing logic runs. This is a one-line (or two-line) change with no architectural impact.

All streaming behaviour (unclosed tag handling, triple-dash delimiter, timer, glow) works unchanged once the tag names match.

## Key Files

| File | Purpose |
|------|---------|
| `frontend/src/components/session/Markdown.tsx` | `processThinkingTags()` — extend to match `<thinking>` |
| `frontend/src/components/session/Markdown.test.tsx` | Add test cases for `<thinking>` variant |
| `frontend/src/components/session/ThinkingWidget.tsx` | No changes needed |

## Codebase Patterns

- `processThinkingTags()` uses string replacement to normalise tags before extraction — adding `<thinking>` support follows the same pattern.
- Tests in `Markdown.test.tsx` cover all edge cases (streaming/unclosed/empty/multiple); add parallel cases for the `<thinking>` variant.
