# MLOps Newsletter

A three-Worker team that produces an opinionated MLOps newsletter.
The Editor picks the angle; the Researcher hunts for news; the
Journalist writes it. Run it twice with different briefs to see how
the angle drives everything else.

The only files on disk are the three Roles in [`roles/`](roles).
The team builds itself from one prompt typed into a chat session.

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

## 2. Bootstrap and open a chat (terminal 2)

```bash
cd demos/mlops-newsletter
../../bin/helix-org bootstrap --db /tmp/mlops.db --envs-dir /tmp/mlops-envs
../../bin/helix-org chat --new
```

Everything below is typed into this chat.

## 3. Spin up the team

> Set up an MLOps newsletter team from this directory. For each
> `.md` file under `./roles/`, call `create_role` with `id='r-' +`
> the file's basename and `content` equal to the file body. Create
> three positions `p-editor`, `p-researcher`, `p-journalist` under
> `p-root`, each pointing at the matching role. Hire three AI
> workers `w-editor`, `w-researcher`, `w-journalist` into them.
> For each hire set `identityContent` to a one-line stub like
> `"You are the <role>."` Read each role.md to find its
> `Tools (MCP)` line and grant exactly those tool names. Confirm
> when done.

The editor's hire activation creates the five streams and
subscribes; the researcher and journalist subscribe to their inputs.
~30 seconds.

## 4. Publish a brief and follow the cascade

> Publish to `s-briefs`: `"Time for this week's MLOps newsletter.
> Surprise me with the angle."` Then subscribe me to `s-newsletter`
> and `read_events` with `wait=60` until I interrupt — summarise
> each event as it lands.

The cascade you'll see:

- Editor wakes, picks an angle, publishes to `s-angles`.
- Researcher wakes, generates news items, publishes to `s-findings`.
- Journalist wakes, writes ~250 words, publishes to `s-drafts`.
- Editor wakes again, polishes, publishes to `s-newsletter`.

Press Ctrl-C in the chat to stop the read loop, then continue.

## 5. Run it again with a sharper brief

> Publish to `s-briefs`: `"New issue. This week, focus on what is
> quietly broken in MLOps tooling that nobody talks about."` Then
> watch `s-newsletter` until the next issue arrives.

The second issue's angle will be sharper because the brief is.
Same team, same code — just a different prompt.

## 6. Stop

Ctrl-C terminal 1.

## What this shows

Three terse role prompts and one setup message. There is no
scaffolding for "newsletter generation" anywhere in the codebase —
the workflow is the conversation between three Roles on five
Streams. Edit `roles/editor.md` to widen or narrow the angles;
ask the chat to `update_role` from the file; the next issue follows
the new rule.
