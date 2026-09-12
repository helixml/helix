import { describe, expect, it } from "vitest";

import type { TypesInteraction } from "../../api/api";
import {
  collectSubagentRuns,
  mergeStreamingInteraction,
  parseSubagentEntry,
  subagentEntryDetail,
} from "./subagentActivity";

const interaction = (entries: unknown[], id = "interaction-1") => ({
  id,
  created: "2026-08-03T10:43:39Z",
  updated: "2026-08-03T10:44:39Z",
  response_entries: entries,
}) as unknown as TypesInteraction;

describe("subagent activity", () => {
  it("recognizes the Codex ACP labels persisted by Helix", () => {
    expect(parseSubagentEntry({
      type: "tool_call",
      tool_name: "Start subagent fresh_spec_run",
      tool_status: "In Progress",
      content: "**Tool Call: Start subagent fresh_spec_run**\nStatus: In Progress\n\nInspecting files",
    })).toEqual({
      id: "fresh_spec_run",
      name: "fresh_spec_run",
      action: "start",
      label: "Start subagent fresh_spec_run",
      detail: "Inspecting files",
      status: "running",
    });
  });

  it("uses persisted ACP identity when the display label is provider-specific", () => {
    expect(parseSubagentEntry({
      type: "tool_call",
      tool_name: "Audit the authentication changes",
      tool_call_id: "call-7",
      tool_call_name: "spawn_agent",
      subagent_id: "child-session-9",
      tool_status: "Completed",
    })).toMatchObject({
      id: "child-session-9",
      name: "Audit the authentication changes",
      action: "start",
      status: "completed",
    });
  });

  it("recognizes the legacy Codex spawnAgent tool name", () => {
    expect(parseSubagentEntry({
      type: "tool_call",
      message_id: "6",
      tool_call_id: "exec-3bb6759f",
      tool_name: "spawnAgent",
      tool_status: "In Progress",
      content: "**Tool Call: spawnAgent**\nStatus: In Progress\n\n",
    })).toEqual({
      id: "exec-3bb6759f",
      name: "Subagent",
      action: "start",
      label: "spawnAgent",
      detail: "",
      status: "running",
    });
  });

  it("recognizes OpenCode task envelopes from Qwen and GLM", () => {
    expect(parseSubagentEntry({
      type: "tool_call",
      message_id: "2",
      tool_call_id: "tool-1",
      tool_name: "Calculate 17 times 19",
      tool_status: "Completed",
      content: [
        "**Tool Call: Calculate 17 times 19**",
        "Status: Completed",
        "",
        '<task id="ses_child_1" state="completed">',
        "<task_result>",
        "323",
        "</task_result>",
        "</task>",
      ].join("\n"),
    })).toEqual({
      id: "ses_child_1",
      name: "Calculate 17 times 19",
      action: "start",
      label: "Calculate 17 times 19",
      detail: "323",
      status: "completed",
    });
  });

  it("hides empty terminal output", () => {
    expect(subagentEntryDetail(
      "**Tool Call: pwd**\nStatus: Completed\n\nTerminal:\n```\n\n```\n",
    )).toBe("");
  });

  it("folds start and interaction entries into one stable run", () => {
    const runs = collectSubagentRuns([interaction([
      { type: "tool_call", message_id: "24", tool_name: "Start subagent fresh_spec_run", tool_status: "Completed", content: "" },
      { type: "tool_call", message_id: "33", tool_name: "Interact with subagent fresh_spec_run", tool_status: "Completed", content: "Result ready" },
    ])]);

    expect(runs).toHaveLength(1);
    expect(runs[0]).toMatchObject({
      name: "fresh_spec_run",
      status: "completed",
      actions: [
        { label: "Start subagent fresh_spec_run" },
        { label: "Interact with subagent fresh_spec_run", detail: "Result ready" },
      ],
    });
  });

  it("adds mirrored child tool work without replacing the subagent name", () => {
    const runs = collectSubagentRuns([interaction([
      {
        type: "tool_call",
        message_id: "24",
        tool_name: "Start subagent reviewer",
        tool_call_name: "spawn_agent",
        subagent_id: "child-session-1",
        tool_status: "Completed",
      },
      {
        type: "tool_call",
        message_id: "subagent:child-session-1:3",
        tool_name: "Run cargo test",
        tool_call_name: "exec_command",
        subagent_id: "child-session-1",
        tool_status: "Completed",
        content: "All tests passed",
      },
    ])]);

    expect(runs).toHaveLength(1);
    expect(runs[0]).toMatchObject({
      name: "reviewer",
      status: "completed",
      actions: [
        { label: "Start subagent reviewer" },
        { label: "Run cargo test", detail: "All tests passed" },
      ],
    });
  });

  it("keeps legacy Codex spawns distinct and completes them after matching closes", () => {
    const runs = collectSubagentRuns([{
      ...interaction([], "legacy-codex-turn"),
      state: "complete",
      response_entries: [
        { type: "tool_call", message_id: "6", tool_call_id: "spawn-1", tool_name: "spawnAgent", tool_status: "In Progress" },
        { type: "tool_call", message_id: "7", tool_call_id: "spawn-2", tool_name: "spawnAgent", tool_status: "In Progress" },
        { type: "tool_call", message_id: "17", tool_call_id: "close-1", tool_name: "closeAgent", tool_status: "Completed" },
        { type: "tool_call", message_id: "18", tool_call_id: "close-2", tool_name: "closeAgent", tool_status: "Completed" },
      ],
    } as unknown as TypesInteraction]);

    expect(runs).toHaveLength(2);
    expect(runs.map((run) => ({ name: run.name, status: run.status }))).toEqual([
      { name: "Subagent 1", status: "completed" },
      { name: "Subagent 2", status: "completed" },
    ]);
  });

  it("lets streamed entries replace the persisted interaction", () => {
    const persisted = interaction([], "turn-1");
    const merged = mergeStreamingInteraction([persisted], {
      id: "turn-1",
      state: "waiting" as never,
      response_entries: [{
        type: "tool_call",
        tool_name: "Start subagent reviewer",
        tool_status: "Completed",
      }] as never,
    });

    expect(merged).toHaveLength(1);
    expect(collectSubagentRuns(merged)[0]).toMatchObject({ name: "reviewer", status: "running" });
  });
});
