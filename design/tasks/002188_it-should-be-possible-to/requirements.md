# Requirements: Drag-and-Drop File Upload in New SpecTask Form

## Background

The New SpecTask form (`frontend/src/components/tasks/NewSpecTaskForm.tsx`)
currently lets users attach files only by clicking the "Attach files" button,
which opens a native file picker. Users expect to be able to drag files or
screenshots straight onto the form. The codebase already uses `react-dropzone`
for drag-and-drop elsewhere, so we should reuse it here.

## User Stories

### US-1: Drag files onto the form
As a user creating a spec task, I want to drag files/screenshots from my desktop
onto the attachment area so I can attach them without clicking through a file
picker.

**Acceptance criteria**
- A dropzone surrounds the "Attach files" button / attachment row.
- Dropping one or more files adds them to the pending attachments, identical to
  picking them with the file picker.
- While dragging a file over the dropzone, there is clear visual feedback
  (e.g. highlighted border / overlay) indicating files can be dropped.
- The feedback disappears when the drag leaves the zone or the drop completes.

### US-2: Same validation as the file picker
As a user, I want dropped files to obey the same rules as picked files so I get
consistent behaviour and clear errors.

**Acceptance criteria**
- Dropped files are validated against the existing limits: max
  `SPEC_TASK_ATTACHMENT_MAX_PER_TASK` (10) files total, max
  `SPEC_TASK_ATTACHMENT_MAX_BYTES` (10 MB) each.
- Files exceeding the per-file size limit are rejected individually with the
  existing snackbar error.
- Dropping more files than the remaining slot count shows the existing
  "Can only attach N more file(s)" error and rejects the batch.
- Only the accepted attachment MIME types / extensions
  (`SPEC_TASK_ATTACHMENT_ACCEPTED_MIME`) are accepted; other file types are
  ignored/rejected.

### US-3: Existing button still works
As a user, I want the existing "Attach files" click behaviour to keep working
exactly as before.

**Acceptance criteria**
- Clicking the "Attach files" label still opens the native file picker.
- Clicking anywhere in the dropzone area (other than the button) does NOT open
  the file picker (avoid double-trigger), unless that is the explicit chosen
  behaviour.
- Attached files still display as deletable chips; the helper hint text remains.
- Upload still happens after task creation, unchanged.

## Out of Scope
- Pasting images from clipboard.
- Changing the upload endpoint, timing, accepted types, or size limits.
- Drag-and-drop in any other form.
