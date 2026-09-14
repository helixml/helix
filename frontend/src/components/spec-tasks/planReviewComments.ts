import {
  inferCommentLanguage,
  type WorkspaceReviewComment,
} from "../workspace-inspector/workspaceReviewComments";

export type PlanDocumentType =
  | "requirements"
  | "technical_design"
  | "implementation_plan";

const PLAN_DOCUMENTS: Record<PlanDocumentType, { filename: string; label: string }> = {
  requirements: { filename: "requirements.md", label: "Requirements Specification" },
  technical_design: { filename: "design.md", label: "Technical Design" },
  implementation_plan: { filename: "tasks.md", label: "Implementation Plan" },
};

export function buildPlanReviewComment(input: {
  id: string;
  specTaskId: string;
  designDocPath?: string;
  documentType: PlanDocumentType;
  documentContent: string;
  selectedText: string;
  text: string;
}): WorkspaceReviewComment {
  const document = PLAN_DOCUMENTS[input.documentType];
  const normalizedContent = input.documentContent.replace(/\r\n?/g, "\n");
  const selectedText = input.selectedText.trim();
  const startIndex = normalizedContent.indexOf(selectedText);
  const endIndex = startIndex < 0
    ? -1
    : startIndex + Math.max(0, selectedText.length - 1);
  return {
    id: input.id,
    sectionId: `plan:${input.documentType}`,
    sectionTitle: "Plan comment",
    filePath: `design/tasks/${input.designDocPath || input.specTaskId}/${document.filename}`,
    startIndex,
    endIndex,
    rangeLabel: document.label,
    text: input.text.trim(),
    contents: selectedText,
    language: inferCommentLanguage(document.filename),
  };
}
