import { describe, expect, it } from "vitest";

import { buildActivityTimeline, ResponseEntry } from "./InteractionInference";

const entry = (
  message_id: string,
  type: ResponseEntry["type"],
  content: string,
  tool_name?: string,
): ResponseEntry => ({ message_id, type, content, tool_name });

describe("buildActivityTimeline", () => {
  it("preserves prose order and groups only adjacent tool calls", () => {
    const entries = [
      entry("1", "text", "First progress update"),
      entry("2", "tool_call", "first output", "first tool"),
      entry("3", "text", "<thinking>Internal-only reasoning</thinking>"),
      entry("4", "tool_call", "second output", "second tool"),
      entry("5", "text", "<thinking>Check this</thinking>Second progress update"),
      entry("6", "tool_call", "third output", "third tool"),
      entry("7", "tool_call", "fourth output", "fourth tool"),
      entry("8", "text", "Final answer"),
    ];

    const timeline = buildActivityTimeline(entries, false);

    expect(timeline.finalTextIndex).toBe(7);
    expect(timeline.activitySegments.map((segment) => segment.type)).toEqual([
      "text",
      "tools",
      "text",
      "tools",
      "text",
      "tools",
    ]);
    expect(timeline.activitySegments[1]).toMatchObject({
      type: "tools",
      entries: [
        { toolName: "first tool", body: "first output" },
      ],
    });
    expect(timeline.activitySegments[3]).toMatchObject({
      type: "tools",
      entries: [
        { toolName: "second tool", body: "second output" },
      ],
    });
    expect(timeline.activitySegments[4]).toMatchObject({
      type: "text",
      renderThinking: true,
      renderContent: true,
    });
    expect(timeline.activitySegments[5]).toMatchObject({
      type: "tools",
      entries: [
        { toolName: "third tool", body: "third output" },
        { toolName: "fourth tool", body: "fourth output" },
      ],
    });
  });

  it("keeps the current prose in the visible timeline while streaming", () => {
    const entries = [
      entry("1", "tool_call", "output", "preview_snapshot"),
      entry("2", "text", "I am checking the live page"),
    ];

    const timeline = buildActivityTimeline(entries, true);

    expect(timeline.finalTextIndex).toBeUndefined();
    expect(timeline.activitySegments).toMatchObject([
      { type: "tools", entries: [{ toolName: "preview_snapshot" }] },
      {
        type: "text",
        entry: { content: "I am checking the live page" },
        renderThinking: false,
        renderContent: true,
      },
    ]);
  });

  it("groups live tool calls across hidden thoughts while working", () => {
    const entries = [
      entry("1", "text", "Visible progress update"),
      entry("2", "tool_call", "first output", "first tool"),
      entry("3", "text", "<thinking>Old reasoning</thinking>"),
      entry("4", "tool_call", "second output", "second tool"),
      entry("5", "text", "<thinking>Current reasoning</thinking>"),
    ];

    const timeline = buildActivityTimeline(entries, true);

    expect(timeline.activitySegments).toMatchObject([
      {
        type: "text",
        entry: { content: "Visible progress update" },
        renderThinking: false,
        renderContent: true,
      },
      {
        type: "tools",
        entries: [
          { toolName: "first tool" },
          { toolName: "second tool" },
        ],
      },
    ]);
  });

  it("keeps contiguous live tool calls in the same group", () => {
    const entries = [
      entry("1", "tool_call", "first output", "first tool"),
      entry("2", "tool_call", "second output", "second tool"),
      entry("3", "text", "Progress update"),
      entry("4", "tool_call", "third output", "third tool"),
    ];

    const timeline = buildActivityTimeline(entries, true);

    expect(timeline.activitySegments).toMatchObject([
      {
        type: "tools",
        entries: [
          { toolName: "first tool" },
          { toolName: "second tool" },
        ],
      },
      {
        type: "text",
        entry: { content: "Progress update" },
      },
      {
        type: "tools",
        entries: [{ toolName: "third tool" }],
      },
    ]);
  });

  it("keeps final-entry thinking in the work transcript", () => {
    const entries = [
      entry("1", "text", "<thinking>Check the result</thinking>Final answer"),
    ];

    const timeline = buildActivityTimeline(entries, false);

    expect(timeline.finalTextIndex).toBe(0);
    expect(timeline.activitySegments).toMatchObject([
      { type: "text", renderThinking: true, renderContent: false },
    ]);
  });

  it("does not mistake a trailing thought-only entry for the final answer", () => {
    const entries = [
      entry("1", "text", "Final answer"),
      entry("2", "text", "<thinking>Finishing checks</thinking>"),
    ];

    const timeline = buildActivityTimeline(entries, false);

    expect(timeline.finalTextIndex).toBe(0);
    expect(timeline.activitySegments).toMatchObject([
      { type: "text", renderThinking: true, renderContent: false },
    ]);
  });

  it("keeps plan snapshots out of prose and tool activity", () => {
    const entries = [
      entry("1", "tool_call", "output", "read_file"),
      entry("plan", "plan", '{"steps":[{"step":"Build","status":"inProgress"}]}'),
      entry("2", "text", "Done"),
    ];

    const timeline = buildActivityTimeline(entries, false);

    expect(timeline.finalTextIndex).toBe(2);
    expect(timeline.activitySegments).toMatchObject([
      { type: "tools", entries: [{ toolName: "read_file" }] },
    ]);
  });
});
