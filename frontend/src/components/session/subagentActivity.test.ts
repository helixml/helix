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
