import React from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import type { ResponseEntry } from "./InteractionInference";
import {
  hasPlanSource,
  PlanProgress,
  planStepsFromChecklist,
  planStepsForResponse,
  planStepsFromResponseEntries,
} from "./PlanProgress";

const entry = (overrides: Partial<ResponseEntry>): ResponseEntry => ({
  type: "tool_call",
  content: "",
  message_id: "1",
  ...overrides,
});

describe("plan progress normalization", () => {
  it("reads native ACP plan snapshots", () => {
    expect(planStepsFromResponseEntries([entry({
      type: "plan",
      content: JSON.stringify({ steps: [
        { step: "Inspect the code", status: "completed" },
        { step: "Build the UI", status: "in_progress" },
        { step: "Verify it", status: "pending" },
      ] }),
    })])).toEqual([
      { step: "Inspect the code", status: "completed" },
      { step: "Build the UI", status: "inProgress" },
      { step: "Verify it", status: "pending" },
    ]);
  });

  it("falls back to TodoWrite and update_plan tool payloads", () => {
    expect(planStepsFromResponseEntries([entry({
      tool_name: "TodoWrite",
      content: '```json\n{"todos":[{"content":"Ship it","status":"in_progress"}]}\n```',
    })])).toEqual([{ step: "Ship it", status: "inProgress" }]);
    expect(planStepsFromResponseEntries([entry({
      tool_name: "update_plan",
      content: '{"plan":[{"step":"Test it","status":"completed"}]}',
    })])).toEqual([{ step: "Test it", status: "completed" }]);
  });

  it("prefers the latest native snapshot", () => {
    expect(planStepsFromResponseEntries([
      entry({ tool_name: "TodoWrite", content: '{"todos":[{"content":"Old","status":"pending"}]}' }),
      entry({ type: "plan", message_id: "plan", content: '{"steps":[{"step":"Current","status":"completed"}]}' }),
    ])).toEqual([{ step: "Current", status: "completed" }]);
  });

  it("treats an empty native snapshot as an explicit plan reset", () => {
    const entries = [entry({
      type: "plan",
      message_id: "plan",
      content: '{"steps":[]}',
    })];

    expect(hasPlanSource(entries)).toBe(true);
    expect(planStepsFromResponseEntries(entries)).toEqual([]);
    expect(planStepsForResponse(entries, {
      tasks: [{ description: "Stale tasks.md item", status: "in_progress" }],
    })).toEqual([]);
  });

  it("normalizes tasks.md checklist progress", () => {
    expect(planStepsFromChecklist({ tasks: [
      { description: "One", status: "completed" },
      { description: "Two", status: "in_progress" },
    ] })).toEqual([
      { step: "One", status: "completed" },
      { step: "Two", status: "inProgress" },
    ]);
  });

  it("renders the compact progress row and expands the checklist", () => {
    render(<PlanProgress steps={[
      { step: "Inspect", status: "completed" },
      { step: "Build", status: "inProgress" },
      { step: "Verify", status: "pending" },
    ]} />);

    const toggle = screen.getByRole("button", { name: "Expand plan" });
    expect(toggle).toHaveTextContent("Build");
    expect(toggle).toHaveTextContent("1/3");
    expect(screen.queryByText("Inspect")).not.toBeInTheDocument();

    fireEvent.click(toggle);

    expect(screen.getByRole("button", { name: "Collapse plan" })).toBeInTheDocument();
    expect(screen.getByText("Inspect")).toBeInTheDocument();
    expect(screen.getByText("Verify")).toBeInTheDocument();
  });
});
