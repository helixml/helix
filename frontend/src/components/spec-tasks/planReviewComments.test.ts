import { describe, expect, it } from "vitest";
import { buildPlanReviewComment } from "./planReviewComments";

describe("buildPlanReviewComment", () => {
  it("maps a selected plan passage to its canonical spec file", () => {
    expect(buildPlanReviewComment({
      id: "plan-1",
      specTaskId: "spt_1",
      designDocPath: "2026-09-14_feature_1",
      documentType: "technical_design",
      documentContent: "Intro\n\nSelected design text\n",
      selectedText: "Selected design text",
      text: "Clarify this boundary",
    })).toMatchObject({
      sectionTitle: "Plan comment",
      filePath: "design/tasks/2026-09-14_feature_1/design.md",
      rangeLabel: "Technical Design",
      text: "Clarify this boundary",
      contents: "Selected design text",
      language: "md",
    });
  });

  it("uses the task id when a design document path is not available", () => {
    expect(buildPlanReviewComment({
      id: "plan-2",
      specTaskId: "spt_2",
      documentType: "requirements",
      documentContent: "Requirement",
      selectedText: "Requirement",
      text: "Be specific",
    }).filePath).toBe("design/tasks/spt_2/requirements.md");
  });

  it("does not claim the start of the document when rendered text is absent from markdown", () => {
    expect(buildPlanReviewComment({
      id: "plan-3",
      specTaskId: "spt_3",
      documentType: "requirements",
      documentContent: "Source text",
      selectedText: "Rendered-only text",
      text: "Clarify this",
    })).toMatchObject({
      startIndex: -1,
      endIndex: -1,
      contents: "Rendered-only text",
    });
  });

  it("locates rendered text across inline markdown formatting", () => {
    const documentContent =
      "Use **bold guidance** and [`code links`](https://example.com/docs) here.";
    const comment = buildPlanReviewComment({
      id: "plan-4",
      specTaskId: "spt_4",
      documentType: "technical_design",
      documentContent,
      selectedText: "Use bold guidance and code links here.",
      text: "Keep this concrete",
    });

    expect(comment.startIndex).toBe(documentContent.indexOf("Use"));
    expect(comment.endIndex).toBe(
      documentContent.indexOf("here.") + "here.".length - 1,
    );
  });

  it("maps collapsed rendered whitespace back to source offsets", () => {
    const documentContent = "First line\n\nsecond line";
    const comment = buildPlanReviewComment({
      id: "plan-5",
      specTaskId: "spt_5",
      documentType: "requirements",
      documentContent,
      selectedText: "First line second line",
      text: "Join this thought",
    });

    expect(comment.startIndex).toBe(0);
    expect(comment.endIndex).toBe(documentContent.length - 1);
  });
});
