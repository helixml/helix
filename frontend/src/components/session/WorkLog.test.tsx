import { fireEvent, render, screen } from "@testing-library/react";
import { ThemeProvider, createTheme } from "@mui/material/styles";
import { describe, expect, it } from "vitest";

import { WorkLog } from "./WorkLog";

const entries = [
  { id: "1", toolName: "first command", status: "Completed", body: "first output" },
  { id: "2", toolName: "second command", status: "Completed", body: "second output" },
  { id: "3", toolName: "latest command", status: "Running", body: "latest output" },
];

const renderWorkLog = () =>
  render(
    <ThemeProvider theme={createTheme({ palette: { mode: "dark" } })}>
      <WorkLog entries={entries} />
    </ThemeProvider>,
  );

describe("WorkLog", () => {
  it("shows only the latest entry until previous entries are requested", () => {
    renderWorkLog();

    expect(screen.getByText("latest command")).toBeInTheDocument();
    expect(screen.queryByText("first command")).not.toBeInTheDocument();
    const disclosure = screen.getByRole("button", { name: /\+2 previous tool calls/ });
    expect(disclosure).toBeInTheDocument();
    expect(disclosure.querySelector(".MuiButton-startIcon")).toBeInTheDocument();
    expect(disclosure.querySelector(".MuiButton-endIcon")).not.toBeInTheDocument();
    expect(disclosure).toHaveStyle({ color: "rgb(245, 245, 245)" });
    expect(disclosure.querySelector("svg")).toHaveStyle({
      color: "rgb(150, 150, 150)",
      transform: "translateX(-1px)",
    });
  });

  it("reveals the full log and can collapse it again", () => {
    renderWorkLog();

    fireEvent.click(screen.getByRole("button", { name: /\+2 previous tool calls/ }));

    expect(screen.getByText("Tool calls")).toBeInTheDocument();
    expect(screen.getByText("first command")).toBeInTheDocument();
    expect(screen.getByText("second command")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Show fewer tool calls/ })).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: /Show fewer tool calls/ }));

    expect(screen.queryByText("first command")).not.toBeInTheDocument();
    expect(screen.getByText("latest command")).toBeInTheDocument();
  });
});
