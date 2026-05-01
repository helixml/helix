# Requirements: Blog Post — Working with the Garage Door Up

## Context

The user wants a short, engaging blog post for helix.ml/blog about building in the open, riffing on Andy Matuschak's "Work with the garage door up" essay (https://notes.andymatuschak.org/Work_with_the_garage_door_up). The post should highlight a few exciting features Helix has built recently, drawing concrete examples from the public spec task design docs at https://github.com/helixml/helix/tree/helix-specs/design/tasks.

The angle is non-obvious and worth leaning into: **Helix's spec task system is itself an embodiment of "garage door up."** Every feature ships with a public requirements/design/tasks trio committed to a public branch. You can scroll through the work-in-progress of any feature, finished or not, including the architectural pivots and dead ends.

## User Story

As a Helix reader (developer, prospect, or community member), I want a short blog post that explains why Helix chooses to design and build features in the open — and shows me where to look — so that I understand the company's ethos and can browse the work for myself.

## Acceptance Criteria

- **Length:** ~500–800 words. Reads in under 4 minutes.
- **Hook:** Opens with a concrete image (the literal garage door / glassblowing studio metaphor from Matuschak/Sloan), not corporate throat-clearing.
- **Source attribution:** Links to Andy Matuschak's note in the first or second paragraph.
- **Core argument:** Frames Helix's spec task workflow as a structural commitment to working with the garage door up — not a marketing posture.
- **Concrete highlights:** References 2–4 recent spec tasks by name with one-line descriptions and links to their design docs on GitHub. Examples to draw from:
  - 001959 — "Sandbox grows inference" (an architectural pivot mid-flight, captured in the design doc itself)
  - 001972 — Agent-driven multi-PR proposals (agents that propose their own scope breakdown)
  - 001962 — Spec comments as interrupts (changing how reviewers steer mid-turn agents)
  - 001956 — Agent screencast recording (a "round 3" retry that's open about prior failures)
- **Style match:** Mirrors the existing helix.ml/blog voice — conversational, technical, narrative-driven, no marketing fluff. Reference posts: "76ms writes on an NVMe" (technical war story), "The Off-by-One Bug That Made Our AI Responses Land in the Wrong Place" (debugging narrative).
- **Close:** Ends with an invitation — point readers at the public spec task tree and encourage them to browse.
- **Deliverable:** A single Markdown file saved to `/home/retro/work/helix/design/2026-05-01-blog-garage-door-up.md` (matching the existing blog draft convention in that folder), committed and pushed to the helix repo.

## Out of Scope

- Publishing to helix.ml itself (that pipeline is owned elsewhere — this task delivers the draft).
- Designing graphics, social cards, or hero images.
- Cross-posting to other channels.
- Writing more than one blog post.
