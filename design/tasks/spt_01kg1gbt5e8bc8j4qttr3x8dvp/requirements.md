# Requirements: Attach Screenshots and Documents to Spec Tasks

## Background

When users create or refine a spec task, the only context they can give the
agent today is a text prompt. Real product/design feedback is visual: a
screenshot of a broken UI, a Figma export, a PDF spec, a snippet of a log
file. Users currently have to describe these things in words, or paste
URLs the agent cannot reach from inside its sandbox.

This task adds first-class support for attaching files (images, PDFs,
text documents) to a spec task. The files must end up *inside the
agent's dev container* so the agent (Zed / Claude Code) can read them
just like any other file in the workspace.

## User Stories

### US-1 — Attach files when creating a task
> As a user, when I open the "New Spec Task" form, I want to drag-and-drop
> screenshots or documents alongside my prompt so the agent has visual
> context from the very first message.

**Acceptance criteria:**
- The `NewSpecTaskForm` shows a drag-and-drop attachment area below the
  prompt textarea, plus a "Browse" button.
- Each attached file shows: filename, size, a small thumbnail (for
  images) or generic icon (for documents), and a "remove" button.
- Drop / browse adds files to a pending list; nothing is uploaded until
  the user clicks "Create".
- When the user submits the form, files are uploaded *first*; if any
  upload fails the task is not created and the error is surfaced
  in-place.

### US-2 — File types and limits
> As a user, I want clear feedback about what I can upload so I don't
> waste time on rejected files.

**Acceptance criteria:**
- Accepted MIME types: `image/png`, `image/jpeg`, `image/gif`,
  `image/webp`, `image/svg+xml`, `application/pdf`, `text/plain`,
  `text/markdown`, `text/csv`.
- Per-file size limit: **10 MB**.
- Per-task limit: **10 files**.
- Rejected files are shown inline with a reason
  (e.g. "PNG too large — 12.4 MB > 10 MB limit").

### US-3 — Agent can see attachments inside the container
> As an agent, I want attached files to appear at a stable, known path in
> my workspace so I can `Read` or view them without any new tools.

**Acceptance criteria:**
- Attachments are checked into the `helix-specs` branch under
  `design/tasks/<task-dir>/attachments/<original-filename>` so they end
  up inside the container at
  `/home/retro/work/helix-specs/design/tasks/<task-dir>/attachments/...`.
- The planning prompt sent to the agent includes a new
  `## Attachments` section listing each attached file with its absolute
  path, MIME type, and the user-supplied caption (if any).
- A new line is appended to the prompt's existing "Visual Testing"
  section: "If the user attached screenshots above, **read them first**
  before exploring the UI — they are evidence of the bug/feature, not
  decoration."

### US-4 — View and manage attachments after creation
> As a user, on the task detail page, I want to see the files I attached
> and remove or add to them while the task is still in `backlog` or
> `spec_review`.

**Acceptance criteria:**
- The task detail page renders an "Attachments" panel showing all files
  with thumbnails, filenames, sizes, and (for the task owner) remove +
  add buttons.
- Clicking an image opens it in a lightbox; clicking a document opens
  it in a new tab (served via a signed-URL endpoint).
- Add/remove is disabled (read-only view) once the task moves past
  `spec_review` — at that point, the agent has already started using
  them and changing them would be confusing.

### US-5 — Authorization
> As the platform, I must enforce that attachments respect spec-task
> authorization.

**Acceptance criteria:**
- Upload, list, fetch, and delete attachment endpoints all run through
  `authorizeUserToProjectByID` for the parent task's project.
- The signed-URL endpoint validates the user has at least `ActionGet`
  on the parent project before issuing a short-lived (5 min) URL.
- The same anonymous public-design-docs path
  (`PublicDesignDocs == true`) gates anonymous attachment fetches —
  attachments live in the same branch as the design docs they support.

### US-6 — Clone-task behaviour
> As a user, when I clone a task, the cloned task should inherit the
> attachments of the source task.

**Acceptance criteria:**
- `CloneTask` copies attachment files from
  `<source-task-dir>/attachments/` to
  `<new-task-dir>/attachments/` in the same helix-specs commit that
  pre-populates the cloned specs.
- The cloned task's planning prompt lists the inherited attachments
  exactly as it would for an originally-created task.

## Non-Goals

- No video uploads in v1 (the limit is 10 MB; video would force a much
  bigger envelope and a separate transcoding path).
- No live editing of attached PDFs/images inside Helix.
- No attachments on individual chat turns *within* a session — this is
  task-level only. (Session-level attachments already exist via
  `RobustPromptInput.PendingAttachment` and are out of scope.)
- No moving attachments between tasks.

## Out of Scope / Future

- Attaching files via the `helix` CLI (`helix spectask create --attach
  screenshot.png`). The CLI plumbing exists for filestore uploads; we
  can hook it up in a follow-up.
- Attachment versioning. If a user re-uploads a file with the same
  name, it overwrites the previous one.
