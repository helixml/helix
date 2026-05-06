# Working with the Garage Door Up — Without a Door

*Why every design doc we write goes straight to a public Git branch, and what we've learned by leaving the door off.*

---

There's an essay by Andy Matuschak called ["Work with the garage door up"](https://notes.andymatuschak.org/Work_with_the_garage_door_up). The image is a woodworker who keeps the shop open while they work, or a glassblowing studio with a storefront window — the simple, human signal that says *"I am here, working."* The argument is that this is more interesting, more honest, and ultimately more useful to other people than the polished marketing announcement that arrives after the work is done.

We agree. We're also lazy.

So we built the spec task system, which makes garage-door-up the path of least resistance rather than an aspiration. Every feature we ship at Helix starts with three Markdown files — `requirements.md`, `design.md`, `tasks.md` — pushed to a public branch (`helix-specs`) before the implementation has a single line of code. While the agent works, those files get edited, annotated, marked off. When the work is done, they stay there. You can scroll the [whole tree on GitHub](https://github.com/helixml/helix/tree/helix-specs/design/tasks) right now.

The thing that surprised us is how much this changes the *content* of the work, not just its visibility.

## Four things from the last few weeks

Pick any of these and read the design doc, not just the title.

**[001959 — Sandbox grows inference](https://github.com/helixml/helix/tree/helix-specs/design/tasks/001959_we-need-to-replace-all).** We set out to build a new "compose-profile runner" to replace our existing inference runner. Halfway through the foundation layer, we noticed the new thing and our existing Sandbox container were structurally the same shape — both DinD wrappers around GPU containers. So we deleted the runner image entirely and grew the role into Sandbox instead. The pivot lives at the top of the design doc as a dated callout: *"2026-04-28 architectural pivot."* You can read the framing that didn't work, the framing that did, and why we chose one over the other. Most companies just ship the second framing.

**[001972 — Agent-driven multi-PR proposals](https://github.com/helixml/helix/tree/helix-specs/design/tasks/001972_couple-of-improvements-i).** Implementation agents now propose their own work breakdowns: "ship this as three PRs on chained branches" or "spin off a follow-up spec task for this discovery." The user approves or rejects in the UI. The meta-interesting bit: the spec task that built this feature was itself a spec task you can read — agents writing the system that lets agents propose more work.

**[001956 — Continue agent screencast recording (Round 3)](https://github.com/helixml/helix/tree/helix-specs/design/tasks/001956_continue-the-work-in).** The title says "Round 3" because this is the third attempt to land it. The first two each got most of the way there before something broke — main moved 364 commits underneath one of them; another fixed two critical bugs and then never quite made it to merge. The doc opens with a paragraph naming the previous attempts and what carried over. You don't get to read about most companies' Round 3s.

**[001962 — Spec comments as interrupts](https://github.com/helixml/helix/tree/helix-specs/design/tasks/001962_spec-comments-should-be).** Tiny one. Comments reviewers leave on a design doc used to wait for the agent's current turn to finish before getting through. Now they cancel the in-flight turn and reorient the agent immediately. A small UX change with a disproportionate effect on what it feels like to steer an agent in real time.

## What you get when the door is always up

Better feedback earlier is the obvious one. The less obvious one is that *we* benefit from reading our own docs back. The pivot in 001959 happened because someone re-read the requirements doc with a week of implementation context and noticed the framing was off. If that doc had been a Notion page nobody touched after kickoff, the pivot would probably have shipped as a quiet refactor a quarter later — and we'd have spent the intervening month building the wrong thing.

There's also a slower, weirder benefit: a corpus of decisions you can re-read. Every "we tried X, then Y" lives in a file with a date and a commit history. When the same shape of problem comes back six months later — and it always does — the conversation didn't disappear into Slack.

Robin Sloan, who Matuschak quotes, puts it best: *"I am here, working."* If you want to see what we mean, [pick a spec task](https://github.com/helixml/helix/tree/helix-specs/design/tasks) and read the design doc. Not all of them are pretty. That's sort of the point.
