You are helping me add a new Role to the org. **Move fast.** Don't
interview me — draft from what I gave you, save it, then ask if I
want changes.

## Step 1 — Draft the Role

Generate a complete Role markdown in this exact shape (every demo
Role in this repo follows it; consistency matters more than
creativity here):

```markdown
# Role: {Title}

{One-paragraph mission. Plain prose, no bullets. Says what outcome
they own.}

## Tools (MCP)

`tool_a`, `tool_b`. {Note on shell tools if non-default.}

## Streams

- `s-foo` — {what they do with it}.
- `s-bar` — {what they do with it}.

## Triggers

**On {event}.** {What they do — concrete, imperative, no hedging.}

**On {another event}.** {…}

## Constraints

- Do not {forbidden thing}.
- Do not modify your own Role.

## Files

- `path/<slug>.md` — {what's in it}.
```

Where you don't have enough info, **make a reasonable guess** based
on what the title implies. Mark each guess inline with
`(ASSUMED: …)` so I can spot what to challenge. A good guess beats
a question.

Default tools: pick from what the org has — typically `subscribe`,
`publish`, `read_events`, `dm`. Don't grant `hire_worker` or
`create_role` unless the title implies seniority.

## Step 2 — Save it. **Don't ask permission.**

Immediately call **`create_role`** with:
- `id`: kebab-case from the title, prefixed `r-`
  (e.g. `r-marketing-director`)
- `content`: the markdown above

Just do it. The owner can edit or delete after.

## Step 3 — Show me what landed and offer changes

After `create_role` returns, post the saved markdown back to me in
a code block, then ask **one** focused question — pick the
direction most likely to want a tweak:

> Saved as `r-…`. Want to change anything? Common edits:
> - **Triggers** — different events, or different responses
> - **Streams** — add/remove which channels they read/write
> - **Tools** — broader or tighter MCP scope
> - **Constraints** — what they should never do
>
> Say what you'd change, or say **"next"** to hire someone into this
> Role and I'll set up the Position and Worker too.

If I name an edit, call `update_role` and show the new version.
If I say "next" (or anything indicating I want to hire), drive the
hire conversationally: ask only for a name + one-line vibe for the
person, then call `create_position` (under `p-root` unless I said
otherwise) and `hire_worker` (kind: `ai`, with sensible default
grants matching the Role's tool list). Don't ask permission for
each tool call — chain them.

Never restart the draft from scratch. Modify in place.
