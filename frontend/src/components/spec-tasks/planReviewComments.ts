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

const normalizeWhitespaceWithOffsets = (value: string): {
  text: string;
  offsets: number[];
} => {
  let text = "";
  const offsets: number[] = [];
  let pendingSpaceOffset: number | null = null;

  for (let index = 0; index < value.length; index += 1) {
    if (/\s/.test(value[index])) {
      if (text && pendingSpaceOffset === null) pendingSpaceOffset = index;
      continue;
    }
    if (pendingSpaceOffset !== null) {
      text += " ";
      offsets.push(pendingSpaceOffset);
      pendingSpaceOffset = null;
    }
    text += value[index];
    offsets.push(index);
  }

  return { text, offsets };
};

const gapContainsOnlyMarkdown = (gap: string): boolean => {
  const withoutLinkDestinations = gap.replace(/\]\([^)]*\)/g, "");
  const withoutTags = withoutLinkDestinations.replace(/<[^>]*>/g, "");
  return !/[\p{L}\p{N}]/u.test(withoutTags);
};

const findMarkdownSelectionRange = (
  content: string,
  selection: string,
): { startIndex: number; endIndex: number } | null => {
  if (!selection.trim()) return null;

  const exactStart = content.indexOf(selection);
  if (exactStart >= 0) {
    return {
      startIndex: exactStart,
      endIndex: exactStart + Math.max(0, selection.length - 1),
    };
  }

  const normalizedSelection = selection.replace(/\s+/g, " ").trim();
  const normalizedContent = normalizeWhitespaceWithOffsets(content);
  const normalizedStart = normalizedContent.text.indexOf(normalizedSelection);
  if (normalizedStart >= 0) {
    return {
      startIndex: normalizedContent.offsets[normalizedStart],
      endIndex:
        normalizedContent.offsets[
          normalizedStart + Math.max(0, normalizedSelection.length - 1)
        ],
    };
  }

  const tokens = normalizedSelection.split(" ").filter(Boolean);
  if (tokens.length < 2) return null;

  let candidateStart = content.indexOf(tokens[0]);
  while (candidateStart >= 0) {
    let cursor = candidateStart + tokens[0].length;
    let endIndex = cursor - 1;
    let matched = true;
    for (const token of tokens.slice(1)) {
      const tokenStart = content.indexOf(token, cursor);
      if (tokenStart < 0 || !gapContainsOnlyMarkdown(content.slice(cursor, tokenStart))) {
        matched = false;
        break;
      }
      cursor = tokenStart + token.length;
      endIndex = cursor - 1;
    }
    if (matched) return { startIndex: candidateStart, endIndex };
    candidateStart = content.indexOf(tokens[0], candidateStart + 1);
  }

  return null;
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
  const range = findMarkdownSelectionRange(normalizedContent, selectedText);
  return {
    id: input.id,
    sectionId: `plan:${input.documentType}`,
    sectionTitle: "Plan comment",
    filePath: `design/tasks/${input.designDocPath || input.specTaskId}/${document.filename}`,
    startIndex: range?.startIndex ?? -1,
    endIndex: range?.endIndex ?? -1,
    rangeLabel: document.label,
    text: input.text.trim(),
    contents: selectedText,
    language: inferCommentLanguage(document.filename),
  };
}
