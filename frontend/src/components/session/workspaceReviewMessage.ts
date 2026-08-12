export interface WorkspaceReviewMessageComment {
  type: "comment";
  filePath: string;
  rangeLabel: string;
  text: string;
  contents: string;
  language: string;
}

export interface WorkspaceReviewMessageText {
  type: "text";
  text: string;
}

export type WorkspaceReviewMessageSegment =
  | WorkspaceReviewMessageComment
  | WorkspaceReviewMessageText;

const REVIEW_COMMENT_PATTERN =
  /<review_comment\b([^>]*)>([\s\S]*?)<\/review_comment>/giu;
const ATTRIBUTE_PATTERN = /\s+([a-z][\w-]*)="([^"]*)"/giu;

function decodeAttribute(value: string): string {
  return value
    .replace(/&quot;/giu, '"')
    .replace(/&lt;/giu, "<")
    .replace(/&gt;/giu, ">")
    .replace(/&amp;/giu, "&");
}

function parseAttributes(value: string): Record<string, string> {
  const attributes: Record<string, string> = {};
  for (const match of value.matchAll(ATTRIBUTE_PATTERN)) {
    attributes[match[1]] = decodeAttribute(match[2]);
  }
  return attributes;
}

function parseCommentBody(value: string): {
  text: string;
  contents: string;
  language: string;
} {
  const lines = value
    .replace(/\r\n?/g, "\n")
    .replace(/^\n/, "")
    .replace(/\n$/, "")
    .split("\n");
  const closingFenceMatch = lines.at(-1)?.match(/^(`{3,})\s*$/);
  if (!closingFenceMatch) {
    return { text: lines.join("\n").trim(), contents: "", language: "text" };
  }

  const fence = closingFenceMatch[1];
  let openingFenceIndex = -1;
  let language = "text";
  for (let index = lines.length - 2; index >= 0; index -= 1) {
    const openingFenceMatch = lines[index].match(/^(`+)(.*)$/);
    if (openingFenceMatch?.[1] !== fence) continue;
    openingFenceIndex = index;
    language = openingFenceMatch[2].trim() || "text";
    break;
  }

  if (openingFenceIndex < 0) {
    return { text: lines.join("\n").trim(), contents: "", language: "text" };
  }

  return {
    text: lines.slice(0, openingFenceIndex).join("\n").trim(),
    contents: lines.slice(openingFenceIndex + 1, -1).join("\n"),
    language,
  };
}

function inferredRangeLabel(attributes: Record<string, string>): string {
  const startIndex = Number.parseInt(attributes.startIndex || "", 10);
  const endIndex = Number.parseInt(attributes.endIndex || "", 10);
  if (!Number.isFinite(startIndex)) return "";
  const startLine = startIndex + 1;
  const endLine = Number.isFinite(endIndex) ? endIndex + 1 : startLine;
  return startLine === endLine ? `L${startLine}` : `L${startLine}–${endLine}`;
}

export function parseWorkspaceReviewMessage(
  message: string,
): WorkspaceReviewMessageSegment[] {
  const segments: WorkspaceReviewMessageSegment[] = [];
  let cursor = 0;

  for (const match of message.matchAll(REVIEW_COMMENT_PATTERN)) {
    const matchIndex = match.index ?? 0;
    const precedingText = message.slice(cursor, matchIndex).trim();
    if (precedingText) segments.push({ type: "text", text: precedingText });

    const attributes = parseAttributes(match[1]);
    const body = parseCommentBody(match[2]);
    if (!attributes.filePath) {
      segments.push({ type: "text", text: match[0] });
    } else {
      segments.push({
        type: "comment",
        filePath: attributes.filePath,
        rangeLabel: attributes.rangeLabel || inferredRangeLabel(attributes),
        ...body,
      });
    }
    cursor = matchIndex + match[0].length;
  }

  const trailingText = message.slice(cursor).trim();
  if (trailingText) segments.push({ type: "text", text: trailingText });
  return segments;
}

export function workspaceReviewMessagePreview(message: string): string | null {
  const segments = parseWorkspaceReviewMessage(message);
  if (!segments.some((segment) => segment.type === "comment")) return null;

  return segments
    .map((segment) => {
      if (segment.type === "text") return segment.text;
      const location = [segment.filePath, segment.rangeLabel]
        .filter(Boolean)
        .join(" ");
      return `Comment on ${location}: ${segment.text}`;
    })
    .join(" · ")
    .replace(/\s+/g, " ")
    .trim();
}

export function workspaceReviewMessageCopyText(message: string): string | null {
  const segments = parseWorkspaceReviewMessage(message);
  if (!segments.some((segment) => segment.type === "comment")) return null;

  return segments
    .map((segment) => {
      if (segment.type === "text") return segment.text;
      const location = [segment.filePath, segment.rangeLabel]
        .filter(Boolean)
        .join(" ");
      const code = segment.contents
        ? `\n\n\`\`\`${segment.language}\n${segment.contents}\n\`\`\``
        : "";
      return `File comment · ${location}\n\n${segment.text}${code}`;
    })
    .join("\n\n");
}
