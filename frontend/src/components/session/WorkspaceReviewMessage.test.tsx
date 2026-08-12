import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import WorkspaceReviewMessage from "./WorkspaceReviewMessage";

vi.mock("./Markdown", () => ({
  default: ({ text }: { text: string }) => <div>{text}</div>,
}));

vi.mock("./MarkdownCodeBlock", () => ({
  default: ({ children, language }: { children: string; language: string }) => (
    <pre data-language={language}>{children}</pre>
  ),
}));

const message = [
  "Please explain this.",
  "",
  '<review_comment sectionId="file:README.md" sectionTitle="File comment" filePath="README.md" startIndex="22" endIndex="22" rangeLabel="L23">',
  "What does this line do?",
  "```md",
  "- [Docker](https://docs.docker.com/get-docker/)",
  "```",
  "</review_comment>",
].join("\n");

describe("WorkspaceReviewMessage", () => {
  it("renders a readable file comment without exposing transport markup", () => {
    const { container } = render(
      <WorkspaceReviewMessage
        text={message}
        session={{ id: "ses_1" }}
        getFileURL={() => ""}
      />,
    );

    expect(screen.getByText("Please explain this.")).toBeInTheDocument();
    expect(screen.getByText("File comment")).toBeInTheDocument();
    expect(screen.getByText("README.md")).toBeInTheDocument();
    expect(screen.getByText("L23")).toBeInTheDocument();
    expect(screen.getByText("What does this line do?")).toBeInTheDocument();
    expect(screen.getByText("- [Docker](https://docs.docker.com/get-docker/)")).toHaveAttribute(
      "data-language",
      "md",
    );
    expect(container).not.toHaveTextContent("review_comment");
    expect(container).not.toHaveTextContent("sectionId");
  });
});
