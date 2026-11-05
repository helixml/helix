import { describe, expect, it } from "vitest";

import { buildActivityTimeline, ResponseEntry } from "./InteractionInference";
import { VISUALIZATION_RESULT_MARKER } from "./visualizationTheme";

const entry = (
  message_id: string,
  type: ResponseEntry["type"],
  content: string,
  tool_name?: string,
): ResponseEntry => ({ message_id, type, content, tool_name });

describe("buildActivityTimeline — visualizations", () => {
  it("renders a completed html_render tool call as a visualization segment", () => {
    const entries = [
      entry("1", "text", "Working on it"),
      entry(
        "2",
        "tool_call",
        `**Tool Call: Run MCP tool \`html_render\`**\nStatus: Completed\n\n${VISUALIZATION_RESULT_MARKER} {"id":"viz_chart1","title":"Sales","height":400}`,
        "Render HTML",
      ),
      entry("3", "text", "Final reply"),
    ];

    const timeline = buildActivityTimeline(entries, false);
    const types = timeline.activitySegments.map((segment) => segment.type);
    expect(types).toContain("visualization");
    expect(types).not.toContain("tools");

    const viz = timeline.activitySegments.find((s) => s.type === "visualization");
    expect(viz).toMatchObject({
      type: "visualization",
      visualization: { id: "viz_chart1", title: "Sales", height: 400 },
    });
  });

  it("shows an in-flight html_render call as a normal tool call until its result arrives", () => {
    const entries = [
      entry("1", "tool_call", "rendering…", "Render HTML"),
    ];
    const timeline = buildActivityTimeline(entries, true);
    expect(timeline.activitySegments.map((s) => s.type)).toEqual(["tools"]);
  });
});
