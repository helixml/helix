import { describe, expect, it } from "vitest";

import {
  parseWorkspaceReviewMessage,
  workspaceReviewMessageCopyText,
  workspaceReviewMessagePreview,
} from "./workspaceReviewMessage";

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

describe("workspace review message", () => {
  it("turns the transport envelope into display segments", () => {
    expect(parseWorkspaceReviewMessage(message)).toEqual([
      { type: "text", text: "Please explain this." },
      {
        type: "comment",
        filePath: "README.md",
        rangeLabel: "L23",
        text: "What does this line do?",
        contents: "- [Docker](https://docs.docker.com/get-docker/)",
        language: "md",
      },
    ]);
  });

  it("creates a readable navigator preview without transport metadata", () => {
    const preview = workspaceReviewMessagePreview(message);
    expect(preview).toBe(
      "Please explain this. · Comment on README.md L23: What does this line do?",
    );
    expect(preview).not.toContain("sectionId");
    expect(preview).not.toContain("review_comment");
  });

  it("uses line indexes when an older message has no range label", () => {
    const legacy = message.replace(' rangeLabel="L23"', "");
    expect(parseWorkspaceReviewMessage(legacy)[1]).toMatchObject({ rangeLabel: "L23" });
  });

  it("formats readable text for copying from history", () => {
    const copied = workspaceReviewMessageCopyText(message);
    expect(copied).toBe([
      "Please explain this.",
      "",
      "File comment · README.md L23",
      "",
      "What does this line do?",
      "",
      "```md",
      "- [Docker](https://docs.docker.com/get-docker/)",
      "```",
    ].join("\n"));
    expect(copied).not.toContain("review_comment");
  });
});
