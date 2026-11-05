import { describe, expect, it } from "vitest";
import { createTheme } from "@mui/material/styles";

import {
  parseVisualizationReference,
  readVisualizationContentHeight,
  readVisualizationLinkRequest,
  visualizationThemeFragment,
  visualizationThemeFromMui,
  visualizationThemeMessage,
  VISUALIZATION_RESULT_MARKER,
} from "./visualizationTheme";
import { getChatColors } from "./chatStyles";

describe("parseVisualizationReference", () => {
  it("extracts a reference from a marked tool result", () => {
    const content =
      `${VISUALIZATION_RESULT_MARKER} {"id":"viz_abc123","title":"Revenue","height":480}\n` +
      `Shown to the reader above your reply.`;
    expect(parseVisualizationReference(content)).toEqual({
      id: "viz_abc123",
      title: "Revenue",
      height: 480,
    });
  });

  it("finds the marker even when a harness wraps the tool output", () => {
    const content =
      "**Tool Call: Run MCP tool `html_render`**\nStatus: Completed\n\n" +
      `${VISUALIZATION_RESULT_MARKER} {"id":"viz_x1","title":"Chart","height":0}`;
    expect(parseVisualizationReference(content)?.id).toBe("viz_x1");
  });

  it("returns undefined without the marker", () => {
    expect(parseVisualizationReference("just some tool output")).toBeUndefined();
  });

  it("rejects an id that is not a viz_ id (no path traversal)", () => {
    const content = `${VISUALIZATION_RESULT_MARKER} {"id":"../../etc/passwd","title":"x"}`;
    expect(parseVisualizationReference(content)).toBeUndefined();
  });

  it("tolerates malformed JSON after the marker", () => {
    expect(parseVisualizationReference(`${VISUALIZATION_RESULT_MARKER} {not json`)).toBeUndefined();
  });
});

describe("visualization theme mapping", () => {
  it("derives appearance and core variables from a dark MUI theme", () => {
    const theme = createTheme({ palette: { mode: "dark" } });
    const viz = visualizationThemeFromMui(theme);
    expect(viz.appearance).toBe("dark");
    expect(viz.variables["--background"]).toBe(getChatColors(theme).canvas);
    expect(viz.variables["--foreground"]).toBe(getChatColors(theme).assistantForeground);
    expect(viz.variables["--chart-1"]).toBeTruthy();
    expect(viz.variables["--chart-6"]).toBeTruthy();
  });

  it("matches the chat canvas, not the page background, in light mode", () => {
    const theme = createTheme({ palette: { mode: "light" } });
    const viz = visualizationThemeFromMui(theme);
    expect(viz.appearance).toBe("light");
    // The chat column paints #fafafa over MUI's #fff page background.
    expect(viz.variables["--background"]).toBe("#fafafa");
  });

  it("round-trips the theme through the URL fragment", () => {
    const viz = visualizationThemeFromMui(createTheme({ palette: { mode: "dark" } }));
    const fragment = visualizationThemeFragment(viz);
    expect(fragment.startsWith("#helix-viz-theme=")).toBe(true);
    const encoded = fragment.slice("#helix-viz-theme=".length);
    expect(JSON.parse(decodeURIComponent(encoded))).toEqual(viz);
  });

  it("builds a host-context-changed message", () => {
    const viz = visualizationThemeFromMui(createTheme({ palette: { mode: "light" } }));
    const message = visualizationThemeMessage(viz);
    expect(message.method).toBe("ui/notifications/host-context-changed");
    expect(message.params.theme).toBe("light");
    expect(message.params.styles.variables["--background"]).toBe(viz.variables["--background"]);
  });
});

describe("protocol readers", () => {
  it("reads a size-changed height", () => {
    expect(
      readVisualizationContentHeight({
        jsonrpc: "2.0",
        method: "ui/notifications/size-changed",
        params: { height: 512 },
      }),
    ).toBe(512);
  });

  it("ignores a non-size message", () => {
    expect(readVisualizationContentHeight({ jsonrpc: "2.0", method: "other" })).toBeUndefined();
    expect(readVisualizationContentHeight(null)).toBeUndefined();
  });

  it("reads an open-link request only for http(s) urls", () => {
    expect(
      readVisualizationLinkRequest({
        jsonrpc: "2.0",
        id: "l1",
        method: "ui/open-link",
        params: { url: "https://example.com" },
      }),
    ).toEqual({ id: "l1", url: "https://example.com" });
    expect(
      readVisualizationLinkRequest({
        jsonrpc: "2.0",
        id: "l2",
        method: "ui/open-link",
        params: { url: "javascript:alert(1)" },
      }),
    ).toBeUndefined();
  });
});
