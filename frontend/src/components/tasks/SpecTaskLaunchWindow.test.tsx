import { fireEvent, render, screen } from "@testing-library/react";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import SpecTaskLaunchWindow, {
  getSpecTaskLaunchPhase,
} from "./SpecTaskLaunchWindow";

const renderWindow = (props: ComponentProps<typeof SpecTaskLaunchWindow>) =>
  render(
    <ThemeProvider theme={createTheme({ palette: { mode: "dark" } })}>
      <SpecTaskLaunchWindow {...props} />
    </ThemeProvider>,
  );

describe("SpecTaskLaunchWindow", () => {
  it("keeps a newly-created session in the launch transition until desktop startup begins", () => {
    expect(getSpecTaskLaunchPhase({
      status: "implementation",
      activeSessionId: "session-1",
      hasDesktopLifecycleState: false,
    })).toBe("starting");
  });

  it("hands a provisioned session to the normal desktop UI", () => {
    expect(getSpecTaskLaunchPhase({
      status: "implementation",
      activeSessionId: "session-1",
      hasDesktopLifecycleState: true,
    })).toBeNull();
  });

  it("shows the authoritative reason when a task is genuinely queued", () => {
    renderWindow({
      phase: "queued",
      mode: "implementation",
      queueReason: "Waiting for implementation capacity — 5 tasks are already being implemented (limit 5).",
    });

    expect(screen.getByText("Task queued")).toBeInTheDocument();
    expect(screen.getByText(/Waiting for implementation capacity/)).toBeInTheDocument();
    expect(screen.queryByText(/Starting implementation/)).not.toBeInTheDocument();
  });

  it("shows a single restrained desktop boot message while launching", () => {
    renderWindow({ phase: "starting", mode: "planning" });

    expect(screen.getByRole("status")).toHaveTextContent(
      "booting virtual desktop environment...",
    );
    expect(screen.queryByText("Chat")).not.toBeInTheDocument();
    expect(screen.queryByText("Desktop")).not.toBeInTheDocument();
  });

  it("allows a queued task to be moved back to the backlog", () => {
    const onMoveToBacklog = vi.fn();
    renderWindow({
      phase: "queued",
      mode: "planning",
      queueReason: "Waiting for planning capacity.",
      onMoveToBacklog,
    });

    fireEvent.click(screen.getByRole("button", { name: "Move to backlog" }));
    expect(onMoveToBacklog).toHaveBeenCalledOnce();
  });
});
