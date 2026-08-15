# Headless spec tasks

Spec tasks can be created with either `ubuntu-desktop` or `headless-ubuntu` as
an immutable sandbox runtime. Existing tasks with no stored runtime continue to
use the full desktop.

## Runtime

Headless spec tasks use the versioned `helix-ubuntu` image because the agent
requires the bundled Zed binary, ACP harnesses, workspace setup, settings sync,
and nested Docker toolchain. `HELIX_HEADLESS=1` selects an agent-only startup
path in that image:

- Zed starts with `--headless`.
- GNOME, streaming, render-node detection, scanout setup, and NVIDIA runtime
  configuration do not start.
- The desktop bridge runs in workspace-only mode. It exposes the file, diff,
  checkpoint, and fixed git-plumbing APIs through RevDial without initializing
  GStreamer, D-Bus, a compositor, input, video, screenshots, or desktop MCP.
- Hydra does not inject display or GPU configuration. It waits for the
  workspace-only bridge before declaring the headless sandbox ready.
- Workspace setup, settings sync, and nested Docker remain available.

The owning task is the source of truth for runtime on every launch path,
including resume, fork, and reconciliation. This prevents a headless task from
being restarted as a desktop later.

## UI

Task creation exposes compute and environment in one compact sectioned menu
on both the home chat composer and board form. Headless task detail
views omit the Desktop tab and initially collapse the task panel; users can
expand it to inspect changes and task details. The environment remains visible
in that menu after launch but is locked, with a tooltip directing users to
start a new task if they need a different runtime.

The last environment a user selects is stored per project in the browser and
becomes that user's next-task default. A project owner can also set the shared
baseline in Project Settings → Sandbox. The personal preference wins once a
user makes an explicit choice; otherwise the project default is used. API
clients that omit `sandbox_runtime` inherit the project default on the server.
