import { fireEvent, render, screen } from "@testing-library/react";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import type { ComponentProps } from "react";
import { describe, expect, it, vi } from "vitest";
import SpecTaskLaunchWindow from "./SpecTaskLaunchWindow";

const renderWindow = (props: ComponentProps<typeof SpecTaskLaunchWindow>) =>
  render(
    <ThemeProvider theme={createTheme({ palette: { mode: "dark" } })}>
      <SpecTaskLaunchWindow {...props} />
    </ThemeProvider>,
  );

describe("SpecTaskLaunchWindow", () => {
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

  it("shows chat and desktop startup placeholders while launching", () => {
    renderWindow({ phase: "starting", mode: "planning" });

    expect(screen.getByText("Starting planning")).toBeInTheDocument();
    expect(screen.getByText("Connecting your agent…")).toBeInTheDocument();
    expect(screen.getByText("Desktop")).toBeInTheDocument();
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
