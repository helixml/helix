import React, { FC, useState } from "react";
import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import Typography from "@mui/material/Typography";
import { useTheme } from "@mui/material/styles";
import {
  CheckCircle2,
  ChevronDown,
  ChevronUp,
  CircleAlert,
  LoaderCircle,
  Terminal,
} from "lucide-react";
import { preserveDisclosureExpansion } from "./disclosureScroll";

/**
 * Represents a parsed segment of a response message.
 * Either a regular markdown block or a tool call block.
 */
export interface MessageSegment {
  type: "markdown" | "toolcall";
  content: string;
  /** Only present for toolcall segments */
  toolName?: string;
  /** Only present for toolcall segments */
  status?: string;
  /** The body content after the header/status lines */
  body?: string;
}

/**
 * Parse a response message into segments of regular markdown and tool call blocks.
 *
 * Tool call blocks follow this pattern (produced by Zed's ToolCall.to_markdown()):
 *   **Tool Call: <name>**
 *   Status: <status>
 *
 *   <body content...>
 *
 * A tool call block is terminated by the next **Tool Call:** header or end of string.
 * Incomplete tool call blocks (missing Status: line) are left as raw markdown.
 */
export function parseToolCallBlocks(text: string): MessageSegment[] {
  if (!text) return [];

  const segments: MessageSegment[] = [];

  // Match **Tool Call: <name>** or **Tool Call: <name>**\n at the start of a line
  // The regex finds all positions where a tool call block starts
  const toolCallPattern =
    /^\*\*Tool Call: (.+?)\*\*\s*\nStatus: (\S+)/gm;

  let lastIndex = 0;
  const matches: { index: number; fullMatch: string; name: string; status: string }[] = [];

  let match;
  while ((match = toolCallPattern.exec(text)) !== null) {
    matches.push({
      index: match.index,
      fullMatch: match[0],
      name: match[1],
      status: match[2],
    });
  }

  if (matches.length === 0) {
    // No tool calls found — return the whole text as markdown
    return [{ type: "markdown", content: text }];
  }

  for (let i = 0; i < matches.length; i++) {
    const m = matches[i];

    // Add any markdown content before this tool call
    if (m.index > lastIndex) {
      const before = text.slice(lastIndex, m.index).trim();
      if (before) {
        segments.push({ type: "markdown", content: before });
      }
    }

    // Determine where this tool call block ends
    const nextStart = i + 1 < matches.length ? matches[i + 1].index : text.length;
    const fullBlock = text.slice(m.index, nextStart);

    // The body is everything after the header+status lines
    const headerEnd = m.index + m.fullMatch.length;
    const body = text.slice(headerEnd, nextStart).trim();

    segments.push({
      type: "toolcall",
      content: fullBlock.trim(),
      toolName: m.name,
      status: m.status,
      body,
    });

    lastIndex = nextStart;
  }

  // Add any trailing markdown after the last tool call
  if (lastIndex < text.length) {
    const trailing = text.slice(lastIndex).trim();
    if (trailing) {
      segments.push({ type: "markdown", content: trailing });
    }
  }

  return segments;
}

const statusIcon = (status: string) => {
  const lower = status.toLowerCase();
  if (lower === "completed") {
    return <CheckCircle2 size={15} strokeWidth={1.8} color="#66bb6a" aria-hidden="true" />;
  }
  if (lower === "failed" || lower === "rejected" || lower === "canceled") {
    return <CircleAlert size={15} strokeWidth={1.8} color="#ef5350" aria-hidden="true" />;
  }
  // Pending, InProgress, etc.
  return <LoaderCircle size={15} strokeWidth={1.8} color="#ffb74d" aria-hidden="true" />;
};

const commandToolPattern = /^(bash|sh|zsh|shell|terminal|command)(?::\s*(.*))?$/i;

const extractCommand = (body: string) => {
  const tableCommand = body.match(/\|\s*Command\s*\|\s*`([^`]+)`\s*\|/i);
  if (tableCommand?.[1]) return tableCommand[1].trim();

  const fieldCommand = body.match(/(?:^|\n)Command:\s*(.+)$/im);
  if (fieldCommand?.[1]) return fieldCommand[1].trim();

  const firstLine = body
    .split("\n")
    .map((line) => line.trim())
    .find((line) => line && !line.startsWith("```") && !line.startsWith("|"));
  return firstLine || "";
};

export const getToolCallPresentation = (toolName: string, body: string) => {
  const toolMatch = toolName.match(commandToolPattern);
  const bodyCommand = extractCommand(body);
  const isCommand = Boolean(toolMatch || body.match(/\|\s*Command\s*\|/i));

  if (!isCommand) {
    return { label: toolName, preview: "" };
  }

  const namedCommand = toolMatch?.[2]?.trim();
  const preview = namedCommand
    ? toolName.trim()
    : bodyCommand
      ? `${toolName.trim()}: ${bodyCommand}`
      : toolName.trim();

  return { label: "Ran command", preview };
};

interface CollapsibleToolCallProps {
  toolName: string;
  status: string;
  body: string;
  /** If true, render expanded by default (e.g. during streaming) */
  defaultExpanded?: boolean;
  /** Use the tighter row sizing needed inside a Work Log. */
  dense?: boolean;
}

export const CollapsibleToolCall: FC<CollapsibleToolCallProps> = ({
  toolName,
  status,
  body,
  defaultExpanded = false,
  dense = false,
}) => {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const theme = useTheme();
  const isDark = theme.palette.mode === "dark";
  const presentation = getToolCallPresentation(toolName, body);

  return (
    <Box
      sx={{
        my: dense ? 0.25 : 0.75,
      }}
    >
      {/* Collapsed header — always visible */}
      <Box
        onClick={(event) => {
          if (!expanded) preserveDisclosureExpansion(event.currentTarget)
          setExpanded(!expanded)
        }}
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 0.75,
          px: 0,
          py: dense ? 0.25 : 0.5,
          cursor: "pointer",
          backgroundColor: "transparent",
          "&:hover": {
            backgroundColor: "transparent",
          },
          userSelect: "none",
        }}
      >
        <Terminal
          size={15}
          strokeWidth={1.8}
          color={isDark ? "rgba(255,255,255,0.5)" : "rgba(0,0,0,0.45)"}
          aria-hidden="true"
        />
        <Box
          sx={{
            display: "flex",
            alignItems: "baseline",
            gap: 0.75,
            minWidth: 0,
            flex: 1,
            whiteSpace: "nowrap",
            overflow: "hidden",
          }}
        >
          <Typography
            variant="body2"
            sx={{
              flexShrink: 0,
              fontSize: dense ? "0.76rem" : "0.82rem",
              color: presentation.preview
                ? isDark
                  ? "#f5f5f5"
                  : "text.primary"
                : isDark
                  ? "rgba(255,255,255,0.65)"
                  : "text.secondary",
              fontWeight: presentation.preview ? 600 : 400,
              fontFamily: "monospace",
            }}
          >
            {presentation.label}
          </Typography>
          {presentation.preview && (
            <Typography
              variant="body2"
              sx={{
                minWidth: 0,
                overflow: "hidden",
                textOverflow: "ellipsis",
                fontSize: dense ? "0.72rem" : "0.78rem",
                color: isDark ? "rgba(255,255,255,0.42)" : "text.secondary",
                fontFamily: "monospace",
              }}
            >
              {presentation.preview}
            </Typography>
          )}
        </Box>
        {statusIcon(status)}
        <IconButton
          size="small"
          sx={{ p: 0, ml: 0.5, "&:hover": { backgroundColor: "transparent" } }}
        >
          {expanded ? <ChevronUp size={15} strokeWidth={1.8} /> : <ChevronDown size={15} strokeWidth={1.8} />}
        </IconButton>
      </Box>

      {/* Expanded body */}
      {expanded && body && (
        <Box
          sx={{
            pl: dense ? 2.5 : 2.75,
            pr: 0,
            py: 1,
            fontSize: "0.8rem",
            fontFamily: "monospace",
            whiteSpace: "pre-wrap",
            wordBreak: "break-word",
            color: isDark ? "rgba(255,255,255,0.55)" : "text.secondary",
            backgroundColor: "transparent",
            maxHeight: "300px",
            overflow: "auto",
          }}
        >
          {body}
        </Box>
      )}
    </Box>
  );
};

export default CollapsibleToolCall;
