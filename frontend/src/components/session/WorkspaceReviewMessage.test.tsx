import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

import WorkspaceReviewMessage from "./WorkspaceReviewMessage";

vi.mock("./Markdown", () => ({
  default: ({ text }: { text: string }) => <div>{text}</div>,
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
  it("renders comment text with compact, expandable context", () => {
    const { container } = render(
      <WorkspaceReviewMessage
        text={message}
        session={{ id: "ses_1" }}
        getFileURL={() => ""}
      />,
    );

    expect(screen.getByText("Please explain this.")).toBeInTheDocument();
    expect(screen.getByText("What does this line do?")).toBeInTheDocument();
    const contextChip = screen.getByRole("button", {
      name: "View file comment context: README.md · L23",
    });
    expect(contextChip).toBeInTheDocument();
    expect(container.querySelector("[data-chat-code-block]")).not.toBeInTheDocument();

    fireEvent.click(contextChip);
    expect(screen.getByText("File comment · L23")).toBeInTheDocument();
    expect(screen.getByText("- [Docker](https://docs.docker.com/get-docker/)"))
      .toBeInTheDocument();
    expect(container).not.toHaveTextContent("review_comment");
    expect(container).not.toHaveTextContent("sectionId");
  });

  it("uses the plan comment label in the context action", () => {
    render(
      <WorkspaceReviewMessage
        text={message.split("File comment").join("Plan comment")}
        session={{ id: "ses_1" }}
        getFileURL={() => ""}
      />,
    );

    expect(
      screen.getByRole("button", {
        name: "View plan comment context: README.md · L23",
      }),
    ).toBeInTheDocument();
  });
});
