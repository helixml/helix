import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import { beforeEach, describe, expect, it, vi } from "vitest";

import MarkdownTable from "./MarkdownTable";

const theme = createTheme({ palette: { mode: "dark" } });
const writeText = vi.fn().mockResolvedValue(undefined);

function renderTable() {
  render(
    <ThemeProvider theme={theme}>
      <MarkdownTable>
        <thead>
          <tr>
            <th>Name</th>
            <th style={{ textAlign: "right" }}>Details</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>
              <code>api|worker</code>
            </td>
            <td>{'Uses "quoted" values'}</td>
          </tr>
        </tbody>
      </MarkdownTable>
    </ThemeProvider>,
  );
}

describe("MarkdownTable", () => {
  beforeEach(() => {
    writeText.mockClear();
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
  });

  it("copies the table as Markdown", async () => {
    renderTable();

    fireEvent.click(screen.getByRole("button", { name: "Copy table" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Copy as Markdown" }));

    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        [
          "| Name | Details |",
          "| --- | ---: |",
          "| `api\\|worker` | Uses \"quoted\" values |",
        ].join("\n"),
      ),
    );
    expect(screen.getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });

  it("copies the table as CSV with escaped values", async () => {
    renderTable();

    fireEvent.click(screen.getByRole("button", { name: "Copy table" }));
    fireEvent.click(screen.getByRole("menuitem", { name: "Copy as CSV" }));

    await waitFor(() =>
      expect(writeText).toHaveBeenCalledWith(
        ['Name,Details', 'api|worker,"Uses ""quoted"" values"'].join("\n"),
      ),
    );
  });

  it("toggles long table cells between collapsed and expanded", () => {
    renderTable();

    const container = document.querySelector(".chat-markdown-table-container");
    expect(container).toHaveAttribute("data-expanded", "false");

    fireEvent.click(screen.getByRole("button", { name: "Expand table cells" }));

    expect(container).toHaveAttribute("data-expanded", "true");
    expect(
      screen.getByRole("button", { name: "Collapse table cells" }),
    ).toHaveAttribute("aria-pressed", "true");
  });
});
