# Design: Blog Post — Working with the Garage Door Up

## The Thesis

The angle that ties everything together: **Helix didn't decide to "be transparent." Helix's build process makes transparency the path of least resistance.** Every spec task produces three Markdown files (requirements, design, tasks) that live in a public Git branch from the moment the task starts. The garage door isn't open by intent — it has no door.

This reframes "building in the open" from a marketing slogan into a structural property of the workflow. The post should make this concrete, not abstract.

## Suggested Structure

Keep it short. ~500–800 words, 4–6 sections at most.

1. **Hook (1 short paragraph).** Andy Matuschak's image: the woodworker's open shop, the glassblowing studio with the storefront window. Quote or paraphrase. Link the source.
2. **The pitch problem (1 short paragraph).** Most software companies only ever show finished, polished work. Marketing posts. Launch tweets. The interesting parts — the dead ends, the pivots, the "why did we even try that" — disappear.
3. **What we did instead (2 paragraphs).** Introduce the spec task system: every feature starts with a requirements/design/tasks trio. They live in a public branch (`helix-specs`). Anyone can read them, comment on them, watch them evolve. Link the GitHub tree.
4. **Three things from the last few weeks (3–4 short blurbs).** Pick a small number of recent spec tasks and link directly to their design docs. For each, give one line of context and one line of why it's interesting *as a piece of work-in-progress to look at*. Suggested picks:
   - **001959 — Sandbox grows inference.** A mid-implementation architectural pivot, captured in the design doc with a dated `> 2026-04-28 architectural pivot:` callout. Shows how design docs evolve, not just describe.
   - **001972 — Agent-driven multi-PR proposals.** The agents themselves now propose work breakdowns — meta-interesting because the agent that built this feature was itself a spec task.
   - **001956 — Continue agent screencast recording (Round 3).** Title acknowledges this is the third attempt. Most companies hide their third attempts.
   - **001962 — Spec comments as interrupts.** A small UX change with an outsized effect on how reviewers steer in-flight agents.
5. **Why this matters (1 short paragraph).** What you get when you build this way: better feedback earlier, more invested community, a corpus of decisions you can re-read. Echo Matuschak: "I am here, working."
6. **Invitation (1–2 sentences).** Link to the spec task tree. Tell readers to pick one and read it.

## Voice and Style

- **Match the existing helix.ml/blog tone.** Read recent posts (e.g., "76ms writes on an NVMe", "The Off-by-One Bug…") to calibrate.
- Conversational, first-person plural ("we"), narrative voice.
- Technical without being jargon-heavy — assume a developer reader who doesn't know Helix internals.
- Concrete over abstract. Show, then briefly tell.
- No marketing voice ("revolutionary," "game-changing," "unlock"). No emojis.
- Short paragraphs. Aggressive trimming. Every sentence earns its place.

## File Location and Format

- Save as `/home/retro/work/helix/design/2026-05-01-blog-garage-door-up.md`.
- Follow the format of existing blog drafts in that folder (e.g. `2026-03-15-blog-off-by-one-ai-responses.md`): H1 title, optional italicised tagline, `---` separator, then prose with H2 section headings.
- Plain Markdown. No frontmatter required (existing drafts don't use it).

## Key Decisions and Rationale

- **Pick a small number of highlights, not a feature dump.** The post is about the *practice* of building in the open, with the features as evidence. Listing ten features turns it into a changelog. Three or four lets each one breathe and gives the reader something to actually click on.
- **Lean on the spec task system being the unique structural angle.** Other companies have engineering blogs. Few have a public commit history of every design conversation. That's the wedge.
- **No call-to-action beyond "go read."** This post isn't a funnel — it's an invitation. Per Matuschak, the appeal of working with the door up is precisely that it isn't pitching.
- **Draft only — don't try to publish.** The blog publishing pipeline lives outside this task's scope. Deliverable is the Markdown file, committed and pushed.

## Notes for the Implementing Agent

- **Read 1–2 recent helix.ml/blog posts before drafting.** Style match matters more than perfect facts. Tone calibration is the main risk on this task.
- **Verify the linked spec tasks still match the descriptions.** A quick read of each referenced task's `requirements.md` first paragraph is enough — descriptions in this design doc were accurate as of 2026-05-01 but spec tasks evolve.
- **GitHub link convention:** the helix-specs branch lives at `https://github.com/helixml/helix/tree/helix-specs/design/tasks/<task-id>_<slug>` — verify a couple of links resolve before committing.
- **Don't fabricate metrics or quotes.** If you don't have the number, write around it.
- The CLAUDE.md rule about full GitHub URLs (no `owner/repo#123` shorthand) applies.
