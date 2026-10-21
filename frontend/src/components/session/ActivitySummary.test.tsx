import { fireEvent, render, screen } from "@testing-library/react";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { afterEach, describe, expect, it, vi } from "vitest";

import ActivitySummary, { formatActivityDuration } from "./ActivitySummary";

const renderSummary = (isStreaming = false) =>
  render(
    <ThemeProvider theme={createTheme({ palette: { mode: "dark" } })}>
      <ActivitySummary
        durationMs={125000}
        hasActivity
        isStreaming={isStreaming}
        startedAt={Date.now() - 125000}
      >
        <div>all activity</div>
      </ActivitySummary>
    </ThemeProvider>,
  );

describe("ActivitySummary", () => {
  it("formats elapsed durations compactly", () => {
    expect(formatActivityDuration(0)).toBe("0s");
    expect(formatActivityDuration(65000)).toBe("1m 5s");
  });

  it("keeps activity collapsed while leaving the completion summary visible", () => {
    renderSummary();

    expect(screen.getByText("Worked for 2m 5s")).toBeInTheDocument();
    expect(screen.queryByText("all activity")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Show work log" })).toBeInTheDocument();

    fireEvent.click(screen.getByText("Worked for 2m 5s"));

    expect(screen.getByText("all activity")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Hide work log" })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Hide work log" }));
    expect(screen.queryByText("all activity")).not.toBeInTheDocument();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it("shows the live working label and activity indicator", () => {
    // Freeze the clock: on a slow runner a second can pass between building
    // startedAt and the first render, which made this read "2m 6s".
    vi.useFakeTimers({ toFake: ["Date"] });
    vi.setSystemTime(new Date("2026-01-01T00:00:00Z"));
    renderSummary(true);

    expect(screen.getByText("Working for 2m 5s")).toBeInTheDocument();
    expect(screen.getByTestId("streaming-indicator")).toBeInTheDocument();
  });
});
