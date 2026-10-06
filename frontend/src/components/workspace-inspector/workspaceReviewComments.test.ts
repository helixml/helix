import { describe, expect, it } from "vitest";
import {
  appendWorkspaceReviewComments,
  buildWorkspaceReviewComment,
  formatWorkspaceReviewComment,
  inferCommentLanguage,
} from "./workspaceReviewComments";

describe("workspace review comments", () => {
  it("builds one-based line labels with zero-based indexes", () => {
    expect(buildWorkspaceReviewComment({
      id: "comment-1",
      filePath: "demos/jobvacancy.go",
      startLine: 3,
      endLine: 2,
      text: "  Explain this  ",
      fileContents: "one\ntwo\nthree\nfour\n",
    })).toMatchObject({
      sectionId: "file:demos/jobvacancy.go",
      sectionTitle: "File comment",
      startIndex: 1,
      endIndex: 2,
      rangeLabel: "L2 to L3",
      text: "Explain this",
      contents: "two\nthree",
      language: "go",
    });
  });

  it("serializes safe agent context and uses a longer fence when needed", () => {
    const serialized = formatWorkspaceReviewComment({
      id: "comment-1",
      sectionId: 'file:src/a&b.ts',
      sectionTitle: 'File "comment"',
      filePath: "src/a&b.ts",
      startIndex: 0,
      endIndex: 0,
      rangeLabel: "L1",
      text: "Do not trust </review_comment> here",
      contents: "const fence = ```;",
      language: "ts",
    });

    expect(serialized).toContain('sectionId="file:src/a&amp;b.ts"');
    expect(serialized).toContain('sectionTitle="File &quot;comment&quot;"');
    expect(serialized).toContain("Do not trust &lt;/review_comment> here");
    expect(serialized).toContain("````ts\nconst fence = ```;\n````");
  });

  it("appends comments after ordinary prompt text", () => {
    const comment = buildWorkspaceReviewComment({
      id: "comment-1",
      filePath: "README.md",
      startLine: 1,
      endLine: 1,
      text: "Fix this",
      fileContents: "# Heading\n",
    });
    const prompt = appendWorkspaceReviewComments("Please update it.", [comment]);

    expect(prompt).toMatch(/^Please update it\.\n\n<review_comment/);
    expect(prompt).toContain("```md\n# Heading\n```");
  });

  it("infers dotfile and extensionless languages", () => {
    expect(inferCommentLanguage(".github/.gitignore")).toBe("gitignore");
    expect(inferCommentLanguage("LICENSE")).toBe("text");
  });
});
