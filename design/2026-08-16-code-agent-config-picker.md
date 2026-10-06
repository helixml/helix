# Config-backed code-agent picker

Date: 2026-08-16

## Problem

The task model picker represented harnesses as Helix Apps. A user could only
select a harness when a matching App already existed, and changing harnesses
implicitly changed Agent identity. That conflicts with task-owned
`CodeAgentExecutionConfig` and kept historical task Agents in the critical
path.

## Design

`CodeAgentExecutionControls` edits a complete `CodeAgentExecutionConfig` and
is shared by project defaults, task creation, task detail/chat, and ordinary
external-agent sessions. Its picker always exposes Zed Agent, Goose, Claude
Code, Codex, and opencode. Provider/model choices come from provider endpoints,
not Apps.

The sample-project fork dialog uses the same control. Forking therefore stores
a complete task configuration and no longer asks for or submits an App ID.

Claude Code and Codex expose credential mode in the same picker:

- Subscription uses the connected Claude or ChatGPT subscription and the
  runtime's supported subscription model catalogue.
- API usage uses an organization provider endpoint and model.

## General sessions

General external-agent sessions store their selection in
`Session.Metadata.CodeAgentConfig`. `Session.ParentApp` remains intact: it is
still the Helix Agent identity and supplies instructions, tools, MCPs, and org
behavior. At Zed-config generation time Helix overlays the session execution
config onto a copy of that App. SpecTask sessions continue to leave the session
field empty and read their task-owned config.

Legacy `CodeAgentOverrides` remain readable for old general sessions. The first
edit through the new control writes a complete config and clears the overrides.

## Verification

- Frontend picker/defaults tests and the production frontend build pass.
- Server execution-config tests and the server/store/types build pass.
- A Helix CLI-created SpecTask started a live Zed session and completed its
  next chat turn with empty task App ID, empty session ParentApp, and empty
  session overrides.
- A normal App-less session switched to a complete Zed configuration and
  completed the immediately following live chat turn on a new Zed thread.
