# Newsroom — philwinder.com

A seven-Worker editorial team that pitches, researches, drafts, edits
and opens PRs against the philwinder.com Hugo repo. Phil is the
Owner; he authors and maintains the Roles, hires Maya (EIC) and Renée
(recruiter), reads streams when he wants, and merges PRs.

## Role vs Worker

A **Role** is the *job* — streams, triggers, tools, duties,
constraints. Owner-only, slow-moving, edited manually. The system
stores it as a markdown blob and propagates updates to every Worker
running it via `update_role`.

A **Worker** is the *person* in a Position that runs a Role — name,
voice, stance, personality refusals. Variable per hire. The Worker
never modifies the Role.

[`roles/`](roles) — job descriptions Phil maintains; these become the
content of the system's Role records:
[editor-in-chief](roles/editor-in-chief.md),
[news-scout](roles/news-scout.md),
[researcher](roles/researcher.md),
[journalist](roles/journalist.md),
[seo-strategist](roles/seo-strategist.md),
[fact-checker](roles/fact-checker.md),
[recruiter](roles/recruiter.md).

[`workers/`](workers) — the only Worker identities Phil authors:
[Maya](workers/maya.md), [Renée](workers/renee.md). Everyone else's
identity is sourced live by Renée at cast time.

## How a hire works

The Worker's Environment ends up with three files:

- `role.md` — the Role's content, stamped by `hire_worker` from the
  system Role record. Re-stamped on every `update_role` call.
- `identity.md` — supplied by the manager (from `workers/<name>.md`
  for Phil's two hires, from a Renée-sourced candidate for the rest).
- `agent.md` — a fixed stub the Spawner reads: *"Read role.md and
  identity.md. Trigger below. Act."*

The split means Phil can edit `roles/journalist.md`, run
`update_role`, and watch every journalist's behaviour shift on their
next activation — without touching identities. The reverse: cast a
new journalist with a different identity and the job is unchanged,
the voice isn't.

## Prerequisites

- `helix-org` and `claude` on PATH.
- Each Worker Environment is provisioned with bash + standard Unix
  tools, plus `gh` and `git`. The `gh` token is scoped to
  `philwinder/philwinder.com` only. No bespoke MCP tool for
  publishing — Maya's Role tells her how to clone, branch, commit,
  push, and open a PR; she runs the commands herself.

## Run the demo

Three terminals: server, prompts, and a second `claude` session
watching every stream as the team works. Run from `demos/newsroom/`
so the prompts can refer to `./roles/` and `./workers/` by relative
path.

### 1. Start the server (terminal 1)

```bash
cd demos/newsroom
helix-org serve --db /tmp/newsroom.db --envs-dir /tmp/newsroom-envs
```

### 2. Bootstrap the Owner (terminal 2)

```bash
helix-org bootstrap --install-claude-mcp
```

`--install-claude-mcp` registers the owner's MCP endpoint with your
`claude` CLI under user scope, so plain `claude` sessions in step 3
and step 5 can drive the org.

### 2½. Watch the room (terminal 3, optional but recommended)

```bash
claude --permission-mode bypassPermissions "List every stream, subscribe me to all of them, then loop read_events with wait=60, summarising each event as it lands. Don't stop until I interrupt."
```

Claude calls `subscribe` per stream then long-polls `read_events`,
streaming a one-line summary as each event arrives. To narrow during
a story, ask the same prompt with a different scope — e.g.
"Subscribe me to s-bullpen and s-recruiting only…", or "…just the
s-fact* streams". Multiple watcher sessions are fine; each is its
own MCP client and the broadcaster wakes them all.

You now have `w-owner` with grants for every structural tool.

### 3. Phil scaffolds the team — one prompt

```bash
claude -p --permission-mode bypassPermissions "Set up the newsroom from this directory:

1. For each .md file under ./roles/, call create_role with
   id='r-' + the file's basename (e.g. roles/editor-in-chief.md ->
   r-editor-in-chief), and content equal to the file body.

2. Create Position p-eic under p-root with roleId r-editor-in-chief.
   Create Position p-recruiter under p-root with roleId r-recruiter.

3. Hire two AI workers:

   - id=w-maya into p-eic, identityContent from ./workers/maya.md.
     Grants for the tools her role lists in its 'Tools (MCP)' section
     (read role.md to find them).

   - id=w-renee into p-recruiter, identityContent from
     ./workers/renee.md. Same approach — grants for the tools her
     role lists.

Confirm what you did when finished."
```

Claude reads the directory, makes seven `create_role` calls, two
`create_position` calls, two `hire_worker` calls, and reports back.
~30 sec.

### 4. The team casts itself

From those two hires, the team builds itself. Maya's hire activation
reads `role.md` and the "On first hire" trigger fires: she creates
the streams, then hires the rest of the team one at a time *via*
Renée. For each opening she posts a brief to `recruiting`; Renée
sources three identity candidates inline; Maya picks one by handle
and calls `hire_worker` with that candidate's content as
`identityContent`. ~2 min.

When you see "Newsroom is up" on `editorial`, the team is live.

### 5. Push a brief or live-edit a Role

To push a brief into `editorial`:

```bash
claude -p --permission-mode bypassPermissions "publish to s-editorial: 'Mistral released Foo this morning, see if there's a piece in it.'"
```

Felix (news-scout) pitches → Maya picks → researcher researches →
journalist drafts → journalist and SEO strategist argue in `bullpen`
about the title → fact-checker blocks one number → researcher
re-verifies → Maya ships. PR URL appears in `published`.

To live-edit a Role and watch every Worker pick up the change on
their next activation, edit the file then run:

```bash
# edit roles/journalist.md however you like, then:
claude -p --permission-mode bypassPermissions "Update the journalist role: replace its content with the current contents of ./roles/journalist.md."
```

Every journalist's `role.md` rewrites in place. Their next event
activation reads the new content; behaviour shifts org-wide.

## Two operating modes

1. **Prompted.** Phil pushes a brief on `editorial` (step 5 above).
2. **Autonomous.** A cron fires `tick-morning` at 7am. Felix wakes,
   pitches three stories without prompting. Same flow runs while
   Phil sleeps; he wakes to PRs in his GitHub inbox.

## What to point at during the demo

- **Watch `s-recruiting` during cast time** — ask the watcher
  session to "narrow down to s-recruiting". Renée sources three
  identities per opening *live*. They did not exist five seconds
  ago. Maya picks one. The team is *cast*, not authored.
  **First wow.**
- **Watch `s-bullpen` during a story** — same trick: narrow to
  s-bullpen. Journalist vs SEO strategist, voice vs findability.
  They disagree on something specific. **Second wow.**
- **`update_role` while the team is running** — Phil edits
  `roles/journalist.md` and reruns the prompt from step 5. Every
  journalist's `role.md` rewrites. Next activation, they obey the new
  rule. **Third wow** — a one-file edit shifts org-wide behaviour.
- `ls /tmp/newsroom-envs/w-renee/candidates/researcher/` — three
  drafts on disk, including the two not picked.
- `cat /tmp/newsroom-envs/<researcher>/investigations/<slug>/` —
  research artefacts the system never read.
- `gh pr view` on the published PR — real Hugo content, real branch.
- `diff roles/journalist.md /tmp/newsroom-envs/<journalist>/role.md`
  — identical: the file in the Environment is a copy of the
  canonical, kept fresh by `update_role`.

## Friction map (designed-in clashes)

| Axis                | Who clashes                   | Where            |
| ------------------- | ----------------------------- | ---------------- |
| Brief specificity   | Renée → Maya                  | `recruiting`     |
| Voice vs SEO        | journalist ↔ seo-strategist   | `bullpen`        |
| Sourcing rigour     | fact-checker → researcher     | `fact-check`     |
| Vendor PR filter    | Maya → news-scout             | `news-wire`      |
| Schedule vs quality | Maya ↔ fact-checker           | `bullpen` (rare) |
