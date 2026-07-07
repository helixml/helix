# Humans in the org

Status: draft for review
Date: 2026-07-07
Area: `api/pkg/org/` (helix-org runtime), `frontend/src/pages/HelixOrg*`

## Summary

The org graph can only represent agents. There is no way to put a real
person into it. This blocks two things we want:

1. A bot that finds work (a bug, a sales lead, a doc gap) cannot resolve
   "who is responsible for this?" to a person and reach them.
2. When a bot needs to ask a person something, the message has nowhere
   to go that the person actually sees.

This doc proposes a **human node**: a first-class-but-lightweight entry
in the org graph that holds a person's cross-system identifiers (Slack,
GitHub, email, ...) plus a plain-text responsibility description, and is
reachable on those channels. It reuses the existing Bot aggregate and
the existing transport machinery rather than introducing a parallel
entity type.

## Motivation

Two concrete failures today.

**"Who owns this?" has no answer.** A bot finds a bug in the billing
code. The intended flow is: ask the org who owns that code -> resolve to
a person -> tag them on GitHub and email them. None of the middle steps
exist. There is no person to resolve to.

**Bot-to-human messages disappear.** When the first Chief-of-Staff bot
was created, it announced it would query the operator to learn what the
org is about and what other bots were needed. That message was never
seen. The reason is not that topics are hidden (the per-topic event view
renders them fine). It is that **there was no human node to address**.
The bot's `dm` tool needs a reporting-line peer, and the operator was
not in the graph, so the question landed on the bot's own transcript or
an unwatched topic. The graph literally had nobody named "the operator"
to send to.

Both failures have the same root cause: a person is not representable.

## What exists today

Worth stating plainly, because the proposal leans on it rather than
replacing it.

- **Bot** is the single org-graph node. It has no `kind` - a bot "is its
  own job description" (`Content` markdown), a `Tools` list, reporting
  lines, and topic subscriptions. There is no human/AI split and no
  separate identity record.
- **Topic** is a channel. Every Topic carries a **Transport**: `local`,
  `slack`, `github`, `email` (Postmark), or `webhook`.
- **Dispatch** does two independent things when an event lands on a
  Topic:
  1. Fan-out to subscribers: each subscribed bot is *activated* (an
     agent run spawns).
  2. Outbound emit: if the Topic's Transport is external
     (slack/github/email/webhook), the message is delivered out. This is
     per-Topic and independent of who subscribes.
- Reporting lines auto-provision channels via `channels.Required`:
  `s-dm-<a>-<b>` (1:1 between reporting pairs), `s-team-<mgr>`,
  `s-transcript-<bot>`.
- `streaming.PrincipalKindHuman` already exists. The model already
  anticipates a human sender; nothing consumes it as a node yet.
- The `Bot` table has **no** link to real Helix org members (users). The
  org graph and the RBAC/membership layer are today disjoint.

## Design

### A human is a Bot that never runs

The whole runtime is built on "Bot is the one node." Reporting lines, DM
channels, subscriptions, and the chart canvas all key off `Bot`.
Introducing a separate `Human` entity means re-plumbing every one of
those for a parallel node type. Instead, a human is a `Bot` with two
additions and one behavioural difference.

```
Bot.Kind        = "human"        // new; default "" == agent, unchanged
Bot.HelixUserID = "usr_..."      // new; optional link to a real org member
Bot.Identity    = {              // new; per-channel addresses
    slack:   "@a-teammate",
    github:  "a-teammate",
    email:   "a-teammate@example.com",
    discord: "a-teammate#1234",
}
Bot.Content     = "Point of contact for billing code; runs commercial
                   sales meetings."   // REUSED unchanged as the
                                       // responsibility description
```

The one behavioural difference: **dispatch must never spawn a human**. A
human node cannot run an agent loop. Where dispatch would activate a
subscriber, a `kind=human` subscriber is delivered-to instead.

What this reuses for free:

- **"Who owns X"** = a bot calls the existing `read_bots`, reads human
  nodes' `Content`, and matches on the responsibility text. No new query
  tool for v1.
- **Reaching a person** rides the existing reporting-line DM channel and
  the existing outbound transports.
- **Chart rendering** is a node style, not a new node type.

### Reach: the address travels with the human

This is the load-bearing decision, so it gets its own section.

The channel/medium (Slack vs GitHub vs email) is naturally a Transport.
But the *address within that medium* (which @handle, which email) is a
property of the **person**, not of the topic. A shared team Slack topic
does not @-mention a specific person.

There are two candidate models:

**A. Address lives on the Topic.** Pre-wire a transport-configured Topic
per person per channel; a bot reaches someone by publishing to the right
one. Near-zero new code (reuses outbound emit untouched). But it does
not scale: with `n` people reachable four ways each, that is `n x 4`
pre-provisioned topics, and it cannot express "tag on GitHub AND email"
for one message.

**B. Address travels with the human.** The human node holds all its
handles in `Identity`. A single delivery function resolves the address
per requested channel at send time. Costs one new function plus the
dispatch no-spawn branch. Native multi-channel. Scales cleanly: the
address is on the node, topics are reused.

**We choose B.** Model A is only simpler in the degenerate case of one
person, one channel. The moment there are several teammates each
reachable several ways - the actual world - the address must live on the
person. Multi-channel is not a speculative future here; "tag on GitHub
and email them" is the headline use case. Model A would ship something
that cannot do the thing this feature exists for.

The core new code is one function:

```
deliverToHuman(human Bot, msg Message, channels []Channel)
    for each channel in channels:
        emitter := outbound[channel.Transport]      // existing emitter
        emitter.EmitTo(human.Identity[channel], msg) // person's address
```

It loops the requested channels and reuses the existing per-transport
emitters, addressed from the person's `Identity`. The in-app inbox (see
below) is just one more channel in this list.

### The signed-in human vs the addressable placeholder

A subtlety the first cut missed: an org has `n` people, but typically
only one is signed into Helix looking at the screen. These are two
different needs, and only one of them is the placeholder.

- **The addressable placeholder** is the general case. Most human nodes
  represent teammates who will never open Helix. For them, "reach" means
  external transport only: tag on GitHub, send an email, Slack DM. They
  have no UI.
- **The in-app inbox** is a property of *being a signed-in org member*,
  not of the placeholder. When a human node's `HelixUserID` is set and
  that user is signed in, their bot-to-human messages should surface
  live in the UI.

So the inbox is not part of the "which external channel" decision - it
exists in either model, keyed off `human.HelixUserID`. It is `inapp`
treated as one more delivery channel, always on when the node is linked
to a real user. For a signed-in user, the inbox is: find the human node
where `HelixUserID == me`, show its DM / inbox topic events with unread
counts, reusing the existing per-topic event renderer.

This also gives us the fix for the vanished Chief-of-Staff message: once
the operator is a human node linked to their own account, the bot has a
concrete target to `dm`, and that DM is both delivered externally and
shown in the operator's inbox.

## What is new code vs reused

New:

- `Bot.Kind`, `Bot.HelixUserID`, `Bot.Identity` fields + a migration.
  This is the one genuinely-new concept the backlog flagged (there is no
  human/AI kind today).
- Dispatch branch: `kind=human` subscribers are delivered-to, never
  spawned.
- `deliverToHuman()` - one function, loops channels, reuses existing
  transport emitters plus an `inapp` channel.
- UI: render human nodes distinctly on the chart; an inbox surface for
  the signed-in user (reuses the existing event-list renderer).
- A path to create a human node with its identity (a `create_human` MCP
  tool, or an extension of `create_bot`).

Reused unchanged:

- Reporting lines, DM/team/transcript channel provisioning.
- The transport layer and outbound emitters.
- The per-topic event view.
- `read_bots` for "who owns X" resolution.

## v1 slice

Keep the first cut small; the shape supports more without rework.

1. Schema: add the three fields + migration.
2. Dispatch: the no-spawn branch for `kind=human`.
3. `deliverToHuman` with two default channels: `inapp` (when linked) and
   the person's `primary` external channel.
4. Create + render a human node; a signed-in user's inbox view.

Deliberately deferred (all cheap to add later, most are text not code):

- **Which channel for which context** ("bug -> GitHub, sales -> email")
  lives in the bot's Role markdown, not in Go. Add by editing text.
- **Full multi-channel fan-out** per message (`channels[]` with more than
  the default two) is a caller argument to `deliverToHuman`, already
  supported by its shape.
- **Team-as-target** ("who owns X" resolves to a team) = fan
  `deliverToHuman` over the team's members. Composes; not built now.
- **A dedicated `find_owner` query tool** - only if `read_bots` +
  prompt matching proves unreliable in practice.

## Open questions

1. **Identity storage.** A JSON column on the bot row
   (`serializer:json`, matching `Tools`) is the lazy option. Is a
   per-channel typed table ever needed? Not for v1.
2. **Inbound replies.** A person replying on Slack/email/GitHub arrives
   as a `PrincipalKindHuman` (or `PrincipalKindTransport`) event on the
   channel topic. Confirm the existing inbound transports resolve a
   known human node's handle back to its node id, so a reply is
   attributed to the person rather than an anonymous external sender.
3. **Linking a node to a user.** How is `HelixUserID` set - operator
   picks from org members when creating the node, or a self-claim flow?
   v1 can be operator-set.
4. **Notification/badge.** "Show up more clearly" implies more than a
   list - an unread badge, maybe a push. What is the minimum that counts
   as "clearly" for v1?

## Why not a separate Human entity

Recorded because it will come up. A separate first-class `Human` table
is conceptually cleaner (a person is not a bot). But it forces a
parallel implementation of reporting lines, DM channels, subscriptions,
and chart rendering, all of which key off `Bot` today. The reuse win
from "a human is a bot that never runs" is large, and the only real cost
is the one no-spawn branch in dispatch. The design philosophy for this
package is explicitly "prefer data and text over code" and "keep the
core generic" - a `kind` field plus an identity blob is squarely that.
