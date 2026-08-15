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

  it("renders a T3-style header on a single flat code surface", () => {
    const { container } = renderCodeBlock();
    const block = container.querySelector<HTMLElement>("[data-chat-code-block]");
    const header = container.querySelector<HTMLElement>("[data-chat-code-block-header]");

    expect(screen.getByText("go")).toBeInTheDocument();
    expect(block).toHaveStyle({
      overflow: "hidden",
      border: "1px solid",
    });
    // The header shares the block surface: no tint and no divider of its own.
    expect(header).toHaveStyle({
      display: "flex",
      justifyContent: "space-between",
      backgroundColor: "",
      borderBottom: "",
    });
  });

  it("keeps the highlighted code free of its own background", () => {
    const { container } = renderCodeBlock();
    // react-markdown's inline-code chrome keys off `:not(pre) > code`, so the
    // highlighter must emit a real <pre> wrapper with a transparent code tag.
    const codeTag = container.querySelector<HTMLElement>("pre > code");

    expect(codeTag).not.toBeNull();
    expect(codeTag).toHaveStyle({ background: "transparent" });
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
