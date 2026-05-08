import React, { FC, useState, useMemo } from "react";
import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import Typography from "@mui/material/Typography";
import { useTheme } from "@mui/material/styles";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import BuildIcon from "@mui/icons-material/Build";
import CheckCircleOutlineIcon from "@mui/icons-material/CheckCircleOutline";
import ErrorOutlineIcon from "@mui/icons-material/ErrorOutline";
import HourglassEmptyIcon from "@mui/icons-material/HourglassEmpty";

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
    return <CheckCircleOutlineIcon sx={{ fontSize: 16, color: "success.main" }} />;
  }
  if (lower === "failed" || lower === "rejected" || lower === "canceled") {
    return <ErrorOutlineIcon sx={{ fontSize: 16, color: "error.main" }} />;
  }
  // Pending, InProgress, etc.
  return <HourglassEmptyIcon sx={{ fontSize: 16, color: "warning.main" }} />;
};

interface CollapsibleToolCallProps {
  toolName: string;
  status: string;
  body: string;
  /** If true, render expanded by default (e.g. during streaming) */
  defaultExpanded?: boolean;
}

export const CollapsibleToolCall: FC<CollapsibleToolCallProps> = ({
  toolName,
  status,
  body,
  defaultExpanded = false,
}) => {
  const [expanded, setExpanded] = useState(defaultExpanded);
  const theme = useTheme();
  const isDark = theme.palette.mode === "dark";

  return (
    <Box
      sx={{
        my: 1,
        borderLeft: `3px solid ${isDark ? "rgba(255,255,255,0.15)" : "rgba(0,0,0,0.12)"}`,
        borderRadius: "4px",
        overflow: "hidden",
      }}
    >
      {/* Collapsed header — always visible */}
      <Box
        onClick={() => setExpanded(!expanded)}
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 0.75,
          px: 1.5,
          py: 0.75,
          cursor: "pointer",
          backgroundColor: isDark
            ? "rgba(255,255,255,0.04)"
            : "rgba(0,0,0,0.03)",
          "&:hover": {
            backgroundColor: isDark
              ? "rgba(255,255,255,0.08)"
              : "rgba(0,0,0,0.06)",
          },
          transition: "background-color 0.15s ease",
          userSelect: "none",
        }}
      >
        <BuildIcon
          sx={{
            fontSize: 16,
            color: isDark ? "rgba(255,255,255,0.5)" : "rgba(0,0,0,0.45)",
          }}
        />
        <Typography
          variant="body2"
          sx={{
            flex: 1,
            fontSize: "0.82rem",
            color: isDark ? "rgba(255,255,255,0.7)" : "rgba(0,0,0,0.6)",
            fontFamily: "monospace",
          }}
        >
          {toolName}
        </Typography>
        {statusIcon(status)}
        <IconButton size="small" sx={{ p: 0, ml: 0.5 }}>
          {expanded ? (
            <ExpandLessIcon sx={{ fontSize: 18 }} />
          ) : (
            <ExpandMoreIcon sx={{ fontSize: 18 }} />
          )}
        </IconButton>
      </Box>

      {/* Expanded body */}
      {expanded && body && (
        <Box
          sx={{
            px: 1.5,
            py: 1,
            fontSize: "0.8rem",
            fontFamily: "monospace",
            whiteSpace: "pre-wrap",
            wordBreak: "break-word",
            color: isDark ? "rgba(255,255,255,0.6)" : "rgba(0,0,0,0.55)",
            backgroundColor: isDark
              ? "rgba(255,255,255,0.02)"
              : "rgba(0,0,0,0.015)",
            borderTop: `1px solid ${isDark ? "rgba(255,255,255,0.06)" : "rgba(0,0,0,0.06)"}`,
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
