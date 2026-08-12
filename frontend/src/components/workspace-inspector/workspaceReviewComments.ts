export interface WorkspaceReviewComment {
  id: string;
  sectionId: string;
  sectionTitle: string;
  filePath: string;
  startIndex: number;
  endIndex: number;
  rangeLabel: string;
  text: string;
  contents: string;
  language: string;
}

function escapeAttribute(value: string): string {
  return value
    .replace(/&/g, "&amp;")
    .replace(/"/g, "&quot;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;");
}

function neutralizeReviewCommentTags(value: string): string {
  return value.replace(/<(?=\/?review_comment\b)/giu, "&lt;");
}

function fencedCode(language: string, contents: string): string {
  const longestBacktickRun = Math.max(
    0,
    ...Array.from(contents.matchAll(/`+/g), (match) => match[0].length),
  );
  const fence = "`".repeat(Math.max(3, longestBacktickRun + 1));
  return [`${fence}${language}`, contents.replace(/\n$/, ""), fence].join("\n");
}

export function inferCommentLanguage(filePath: string): string {
  const fileName = filePath.replace(/\\/g, "/").split("/").at(-1)?.toLowerCase() || "";
  const extensionIndex = fileName.lastIndexOf(".");
  if (extensionIndex > 0 && extensionIndex < fileName.length - 1) {
    return fileName.slice(extensionIndex + 1);
  }
  if (fileName.startsWith(".") && fileName.length > 1) return fileName.slice(1);
  return "text";
}

export function buildWorkspaceReviewComment(input: {
  id: string;
  filePath: string;
  startLine: number;
  endLine: number;
  text: string;
  fileContents: string;
}): WorkspaceReviewComment {
  const startLine = Math.max(1, Math.min(input.startLine, input.endLine));
  const endLine = Math.max(startLine, Math.max(input.startLine, input.endLine));
  return {
    id: input.id,
    sectionId: `file:${input.filePath}`,
    sectionTitle: "File comment",
    filePath: input.filePath,
    startIndex: startLine - 1,
    endIndex: endLine - 1,
    rangeLabel: startLine === endLine ? `L${startLine}` : `L${startLine} to L${endLine}`,
    text: input.text.trim(),
    contents: input.fileContents.split("\n").slice(startLine - 1, endLine).join("\n"),
    language: inferCommentLanguage(input.filePath),
  };
}

export function formatWorkspaceReviewComment(comment: WorkspaceReviewComment): string {
  return [
    [
      "<review_comment",
      ` sectionId="${escapeAttribute(comment.sectionId)}"`,
      ` sectionTitle="${escapeAttribute(comment.sectionTitle)}"`,
      ` filePath="${escapeAttribute(comment.filePath)}"`,
      ` startIndex="${comment.startIndex}"`,
      ` endIndex="${comment.endIndex}"`,
      ` rangeLabel="${escapeAttribute(comment.rangeLabel)}"`,
      ">",
    ].join(""),
    neutralizeReviewCommentTags(comment.text),
    fencedCode(comment.language, comment.contents),
    "</review_comment>",
  ].join("\n");
}

export function appendWorkspaceReviewComments(
  prompt: string,
  comments: readonly WorkspaceReviewComment[],
): string {
  const blocks = comments.map(formatWorkspaceReviewComment);
  const trimmedPrompt = prompt.trim();
  if (blocks.length === 0) return trimmedPrompt;
  return trimmedPrompt ? `${trimmedPrompt}\n\n${blocks.join("\n\n")}` : blocks.join("\n\n");
}
