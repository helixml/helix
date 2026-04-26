# MLOps Newsletter

A three-Worker team that produces an opinionated MLOps newsletter.
The Editor picks a fresh angle for each issue; the Researcher hunts
for news that fits; the Journalist writes the prose. Run it twice
with different briefs to see how the angle drives everything else.

The only files on disk are the three Roles in [`roles/`](roles).
Streams, positions, identities, and the team itself are all spun
up by a single `helix-org prompt` call.

## Setup

```bash
cd /home/phil/helix/helix-org
make build
rm -rf /tmp/mlops-envs /tmp/mlops.db
```

## 1. Start the server (terminal 1)

```bash
cd demos/mlops-newsletter
../../bin/helix-org serve --db /tmp/mlops.db --envs-dir /tmp/mlops-envs
```

## 2. Bootstrap the Owner (terminal 2)

```bash
../../bin/helix-org bootstrap
```

## 3. Spin up the team — one prompt

```bash
../../bin/helix-org prompt "Set up an MLOps newsletter team from
this directory. For each .md file under ./roles/, call create_role
with id='r-' + the file's basename (e.g. roles/editor.md ->
r-editor) and content equal to the file body. Create three positions
p-editor, p-researcher, p-journalist under p-root, each pointing at
the matching role. Hire three AI workers w-editor, w-researcher,
w-journalist into them. For each hire set identityContent to a
one-line stub like 'You are the <role>.' Read each role.md to find
its 'Tools (MCP)' line and grant exactly those tool names. Confirm
when done."
```

The editor's hire activation creates the five streams and
subscribes; the researcher and journalist subscribe to their inputs.
~30 seconds.

## 4. Watch the cascade

In a third terminal, tail every stream — this is the live view of
the team thinking out loud:

```bash
../../bin/helix-org tail
```

`tail` defaults to `*` (all streams). Use `tail 's-news*'` for a
glob, or `tail s-newsletter` for a single stream.

Then publish a brief from terminal 2:

```bash
../../bin/helix-org prompt "publish to s-briefs: 'Time for this
week's MLOps newsletter. Surprise me with the angle.'"
```

The cascade you'll see in the tail:

- Editor wakes, picks an angle, publishes to `s-angles`.
- Researcher wakes, generates five news items, publishes to `s-findings`.
- Journalist wakes, writes ~250 words, publishes to `s-drafts`.
- Editor wakes again, polishes and publishes to `s-newsletter`.

To see only the finished issues:

```bash
../../bin/helix-org tail s-newsletter
```

## 5. Run it again with a different brief

```bash
../../bin/helix-org prompt "publish to s-briefs: 'New issue. This
week, focus on what is quietly broken in MLOps tooling that nobody
talks about.'"
```

The angle in the second issue will be sharper and more specific
because the brief shapes it. Same team, same code — just a
different prompt. Run it a third time with a brief about vendor
consolidation, or org-chart trends, and watch the angle move.

## 6. Stop

Ctrl-C terminal 1, then:

```bash
pkill -f 'claude -p' 2>/dev/null
```

## What this shows

The whole demo is three terse role prompts and one setup command.
There is no scaffolding for "newsletter generation" anywhere in the
codebase — the workflow is the conversation between three Roles on
five Streams. Edit `roles/editor.md` to widen or narrow the angles;
rerun the kickoff and the next issue follows the new rule.
