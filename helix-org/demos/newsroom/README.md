# Newsroom — philwinder.com

A seven-Worker editorial team that pitches, researches, drafts, edits
and opens PRs against the philwinder.com Hugo repo. Phil is the
Owner; he authors the Roles, hires Maya (EIC) and Renée (recruiter),
reads streams when he wants, and merges PRs.

## Role vs Worker

A **Role** is the *job* — streams, triggers, tools, duties,
constraints. Owner-only, slow-moving, edited manually. The system
stores it as a markdown blob and propagates updates to every Worker
running it via `update_role`.

A **Worker** is the *person* in a Position that runs a Role — name,
voice, stance, personality refusals. Variable per hire. The Worker
never modifies the Role.

[`roles/`](roles): job descriptions Phil maintains. They become the
content of the system's Role records:
[editor-in-chief](roles/editor-in-chief.md),
[news-scout](roles/news-scout.md),
[researcher](roles/researcher.md),
[journalist](roles/journalist.md),
[seo-strategist](roles/seo-strategist.md),
[fact-checker](roles/fact-checker.md),
[recruiter](roles/recruiter.md).

[`workers/`](workers): the only Worker identities Phil authors —
[Maya](workers/maya.md), [Renée](workers/renee.md). Everyone else's
identity is sourced live by Renée at cast time.

## Prerequisites

- `helix-org` and `claude` on PATH.
- Each Worker Environment is provisioned with bash + standard Unix
  tools, plus `gh` and `git` scoped to `philwinder/philwinder.com`.
  No bespoke MCP tool for publishing — Maya's Role tells her how to
  clone, branch, commit, push, and open a PR; she runs the commands
  herself.

## 1. Start the server (terminal 1)

```bash
cd demos/newsroom
helix-org serve --db /tmp/newsroom.db --envs-dir /tmp/newsroom-envs
```

## 2. Bootstrap and open a chat (terminal 2)

```bash
cd demos/newsroom
helix-org bootstrap --db /tmp/newsroom.db --envs-dir /tmp/newsroom-envs
helix-org chat
```

Everything below is typed into this chat as `w-owner`.

## 3. Phil scaffolds the team

> Set up the newsroom from this directory:
>
> 1. For each `.md` file under `./roles/`, call `create_role` with
>    `id='r-' +` the file's basename and `content` equal to the
>    file body.
>
> 2. Create Position `p-eic` under `p-root` with `roleId
>    r-editor-in-chief`. Create Position `p-recruiter` under
>    `p-root` with `roleId r-recruiter`.
>
> 3. Hire two AI workers:
>    - `id=w-maya` into `p-eic`, `identityContent` from
>      `./workers/maya.md`. Grants for the tools her role lists in
>      its `Tools (MCP)` section (read `role.md` to find them).
>    - `id=w-renee` into `p-recruiter`, `identityContent` from
>      `./workers/renee.md`. Same approach.
>
> Confirm what you did when finished.

Seven `create_role` calls, two `create_position`, two `hire_worker`.
~30 sec.

## 4. The team casts itself

From those two hires, the team builds itself. Maya's hire activation
reads `role.md` and her "On first hire" trigger fires: she creates
the streams, then hires the rest of the team one at a time *via*
Renée. For each opening she posts a brief to `recruiting`; Renée
sources three identity candidates inline; Maya picks one by handle
and calls `hire_worker` with that candidate's content as
`identityContent`. ~2 min.

When `Newsroom is up` lands on `editorial`, the team is live. To
follow along while it casts:

> Subscribe me to `s-recruiting` and `read_events` with `wait=60`,
> summarising each event as it lands. Don't stop until I interrupt.

Press Ctrl-C in the chat to stop the loop.

## 5. Push a brief

> Publish to `s-editorial`: `"Mistral released Foo this morning, see
> if there's a piece in it."` Then subscribe me to `s-bullpen` and
> `s-published` and `read_events` with `wait=60` until I interrupt.

Felix (news-scout) pitches → Maya picks → researcher researches →
journalist drafts → journalist and SEO strategist argue in `s-bullpen`
about the title → fact-checker blocks one number → researcher
re-verifies → Maya ships. PR URL lands on `s-published`.

## 6. DM a Worker

The `dm` tool sends a private message to one Worker. The first call
between any two Workers creates the per-pair Stream; subsequent DMs
reuse it, so the back-and-forth stays in one ordered place:

> DM the fact-checker: "any pattern in the blocks this week —
> sourcing, numbers, framing?" Then `read_events` on the returned
> streamId with `wait=30` until they reply.

`dm` is sugar over `create_stream` + `invite_workers` + `publish` —
useful when you know who you want to talk to. For group threads or
named channels, reach for the underlying tools directly.

## 7. Watch a Worker work

The `worker_log` tool tails one Worker's activation transcript —
their assistant text, tool calls, tool results — without you having
to know the stream-naming convention:

> `worker_log` on `w-fact-checker` with `wait=60`. Show me the
> next activation as it lands.

You'll see `=== activation: event ... ===`, `--- session start ---`,
each `tool_use` they fire, the matching `tool_result`, and finally
`=== exit: ok ===`. `worker_log` is sugar over `subscribe` +
`read_events` scoped to that Worker's activation Stream
(`s-activations-w-fact-checker`).

## 8. Live-edit a Role

Edit `roles/journalist.md` however you like, then in the chat:

> Update the `r-journalist` role: replace its content with the
> current contents of `./roles/journalist.md`.

Every journalist's `role.md` rewrites in place. Their next
activation reads the new content; behaviour shifts org-wide.

## 9. Stop

Ctrl-C terminal 1.

## What to point at during the demo

- **`s-recruiting` during cast time.** Renée sources three identities
  per opening *live*. They did not exist five seconds ago. Maya picks
  one. The team is *cast*, not authored.
- **`s-bullpen` during a story.** Journalist vs SEO strategist, voice
  vs findability. They disagree on something specific.
- **`worker_log w-fact-checker` during a block.** The exact tool call
  and reasoning, live. Inspection without leaving the chat.
- **`update_role` while the team is running.** A one-file edit shifts
  org-wide behaviour on the next activation.
- `ls /tmp/newsroom-envs/w-renee/candidates/researcher/` — three
  drafts on disk, including the two not picked.
- `gh pr view` on the published PR — real Hugo content, real branch.

## Friction map (designed-in clashes)

| Axis                | Who clashes                   | Where            |
| ------------------- | ----------------------------- | ---------------- |
| Brief specificity   | Renée → Maya                  | `s-recruiting`   |
| Voice vs SEO        | journalist ↔ seo-strategist   | `s-bullpen`      |
| Sourcing rigour     | fact-checker → researcher     | `s-fact-check`   |
| Vendor PR filter    | Maya → news-scout             | `s-news-wire`    |
| Schedule vs quality | Maya ↔ fact-checker           | `s-bullpen`      |
