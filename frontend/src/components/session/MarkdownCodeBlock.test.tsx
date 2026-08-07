import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { createTheme, ThemeProvider } from "@mui/material/styles";
import { beforeEach, describe, expect, it, vi } from "vitest";

import MarkdownCodeBlock from "./MarkdownCodeBlock";

const theme = createTheme({ palette: { mode: "dark" } });
const writeText = vi.fn().mockResolvedValue(undefined);

function renderCodeBlock() {
  return render(
    <ThemeProvider theme={theme}>
      <MarkdownCodeBlock language="go">{'fmt.Println("hello")\n'}</MarkdownCodeBlock>
    </ThemeProvider>,
  );
}

describe("MarkdownCodeBlock", () => {
  beforeEach(() => {
    writeText.mockClear();
    Object.defineProperty(navigator, "clipboard", {
      configurable: true,
      value: { writeText },
    });
  });

  it("renders a T3-style header and code surface", () => {
    const { container } = renderCodeBlock();
    const block = container.querySelector<HTMLElement>("[data-chat-code-block]");
    const header = container.querySelector<HTMLElement>("[data-chat-code-block-header]");

    expect(screen.getByText("go")).toBeInTheDocument();
    expect(block).toHaveStyle({
      overflow: "hidden",
      border: "1px solid rgba(255, 255, 255, 0.09)",
      borderRadius: "10px",
      backgroundColor: "#111113",
    });
    expect(header).toHaveStyle({
      minHeight: "34px",
      backgroundColor: "#1a1a1d",
      borderBottom: "1px solid rgba(255, 255, 255, 0.09)",
    });
  });

  it("toggles wrapping and copies normalized code", async () => {
    const { container } = renderCodeBlock();
    const block = container.querySelector<HTMLElement>("[data-chat-code-block]");

    fireEvent.click(screen.getByRole("button", { name: "Wrap lines" }));
    expect(block).toHaveAttribute("data-wrap", "true");
    expect(screen.getByRole("button", { name: "Disable line wrap" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );

    fireEvent.click(screen.getByRole("button", { name: "Copy code" }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith('fmt.Println("hello")'));
    expect(screen.getByRole("button", { name: "Copied" })).toBeInTheDocument();
  });
});
