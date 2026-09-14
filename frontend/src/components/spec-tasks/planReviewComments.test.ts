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
});
