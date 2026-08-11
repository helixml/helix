import React, { FC, useState } from "react";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import { useTheme } from "@mui/material/styles";
import {
  ChevronDown,
  Check,
  Minus,
  Terminal,
  Wrench,
  X,
} from "lucide-react";
import { preserveDisclosureExpansion } from "./disclosureScroll";
import { getChatColors } from "./chatStyles";
import { APP_FONT_FAMILY, APP_MONO_FONT_FAMILY } from "../../styles/typography";

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

const statusIcon = (status: string, mutedColor: string) => {
  const lower = status.toLowerCase();
  if (lower === "completed") {
    return <Check size={12} strokeWidth={1.8} color={mutedColor} aria-hidden="true" />;
  }
  if (lower === "failed" || lower === "rejected" || lower === "canceled") {
    return <X size={12} strokeWidth={1.8} color="#ef5350" aria-hidden="true" />;
  }
  return <Minus size={12} strokeWidth={1.8} color={mutedColor} aria-hidden="true" />;
};

const commandToolPattern = /^(bash|sh|zsh|shell|terminal|command)(?::\s*(.*))?$/i;

const formatMCPProvider = (provider: string) => {
  const normalized = provider.toLowerCase().replace(/_/g, "-");
  if (normalized === "github") return "GitHub";
  if (normalized === "t3-code") return "T3-code";
  return normalized.charAt(0).toUpperCase() + normalized.slice(1);
};

const parseMCPTool = (toolName: string) => {
  if (toolName.startsWith("mcp__")) {
    const parts = toolName.split("__");
    if (parts.length >= 3) {
      return {
        provider: parts[parts.length - 2],
        tool: parts[parts.length - 1],
      };
    }
  }

  if (toolName.startsWith("mcp.")) {
    const parts = toolName.split(".");
    if (parts.length >= 3) {
      return {
        provider: parts[parts.length - 2],
        tool: parts[parts.length - 1],
      };
    }
  }

  return null;
};

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
  const isTerminalResult = /(?:^|\n)Terminal:\s*/im.test(body);
  const bodyCommand = extractCommand(body);
  const isCommand = Boolean(
    toolMatch || isTerminalResult || body.match(/\|\s*Command\s*\|/i),
  );

  if (isCommand) {
    const namedCommand = toolMatch?.[2]?.trim();
    const preview = isTerminalResult
      ? toolName.trim()
      : namedCommand
        ? namedCommand
        : bodyCommand
          ? bodyCommand
          : toolName.trim();

    return { kind: "command" as const, label: "Ran command", preview };
  }

  const mcpTool = parseMCPTool(toolName);
  if (mcpTool) {
    return {
      kind: "tool" as const,
      label: `${formatMCPProvider(mcpTool.provider)} · ${mcpTool.tool}`,
      preview: "",
    };
  }

  return { kind: "tool" as const, label: toolName, preview: "" };
};

const stripToolCallEnvelope = (body: string) => body
  .replace(/^\*\*Tool Call:.*?\*\*\s*\nStatus:\s*\S+\s*/s, "")
  .trim();

const unwrapCodeFence = (value: string) => {
  const match = value.trim().match(/^```[^\n]*\n([\s\S]*?)\n```$/);
  return match?.[1]?.trim() || value.trim();
};

export const getToolCallExpandedBody = (toolName: string, body: string) => {
  const presentation = getToolCallPresentation(toolName, body);
  const content = stripToolCallEnvelope(body);

  if (presentation.kind !== "command") return content;

  const terminal = content.match(/^Terminal:\s*\n([\s\S]*)$/i);
  if (terminal?.[1]) {
    const output = unwrapCodeFence(terminal[1]);
    return [presentation.preview, output].filter(Boolean).join("\n\n");
  }

  return content;
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
  const chatColors = getChatColors(theme);
  const presentation = getToolCallPresentation(toolName, body);
  const expandedBody = getToolCallExpandedBody(toolName, body);
  const detailBorder = isDark
    ? "rgba(255,255,255,0.03)"
    : "rgba(0,0,0,0.045)";

  return (
    <Box
      sx={{
        my: dense ? 0.125 : 0.5,
      }}
    >
      {/* Collapsed header — always visible */}
      <Box
        onClick={(event) => {
          if (!expanded) preserveDisclosureExpansion(event.currentTarget)
          setExpanded(!expanded)
        }}
        onKeyDown={(event) => {
          if (event.key !== "Enter" && event.key !== " ") return;
          event.preventDefault();
          if (!expanded) preserveDisclosureExpansion(event.currentTarget);
          setExpanded(!expanded);
        }}
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        aria-label={`${presentation.label}${presentation.preview ? ` ${presentation.preview}` : ""}`}
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 0.75,
          px: 0.25,
          py: 0.25,
          minHeight: 24,
          borderRadius: "6px",
          cursor: "pointer",
          backgroundColor: "transparent",
          transition: "background-color 150ms ease",
          "&:hover": {
            backgroundColor: isDark
              ? "rgba(255,255,255,0.012)"
              : "rgba(0,0,0,0.025)",
          },
          "&:focus-visible": {
            outline: `2px solid ${isDark ? chatColors.borderStrong : "rgba(0,0,0,0.2)"}`,
            outlineOffset: -2,
          },
          userSelect: "none",
        }}
      >
        <Box
          component="span"
          sx={{
            display: "flex",
            width: 20,
            height: 20,
            flexShrink: 0,
            alignItems: "center",
            justifyContent: "center",
            color: isDark ? chatColors.subtle : "rgba(0,0,0,0.45)",
          }}
        >
          {presentation.kind === "command" ? (
            <Terminal size={14} strokeWidth={1.8} aria-hidden="true" />
          ) : (
            <Wrench size={14} strokeWidth={1.8} aria-hidden="true" />
          )}
        </Box>
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
              fontSize: "0.75rem",
              lineHeight: "20px",
              color: isDark ? chatColors.foreground : "text.primary",
              fontWeight: 500,
              fontFamily: APP_FONT_FAMILY,
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
                fontSize: "0.75rem",
                lineHeight: "20px",
                color: isDark ? chatColors.subtle : "text.secondary",
                fontFamily: APP_FONT_FAMILY,
              }}
            >
              {presentation.preview}
            </Typography>
          )}
        </Box>
        <Box
          component="span"
          sx={{
            display: "flex",
            alignItems: "center",
            gap: "1px",
            color: isDark ? chatColors.subtle : "rgba(0,0,0,0.45)",
          }}
        >
          <Box
            component="span"
            sx={{ display: "flex", width: 16, height: 16, alignItems: "center", justifyContent: "center" }}
          >
            <ChevronDown
              size={12}
              strokeWidth={1.8}
              style={{
                opacity: 0.7,
                transform: `rotate(${expanded ? 180 : 0}deg)`,
                transition: "transform 200ms ease",
              }}
            />
          </Box>
          <Box
            component="span"
            sx={{ display: "flex", width: 16, height: 16, alignItems: "center", justifyContent: "center" }}
          >
            {statusIcon(status, isDark ? chatColors.subtle : "rgba(0,0,0,0.45)")}
          </Box>
        </Box>
      </Box>

      {/* Expanded body */}
      {expanded && expandedBody && (
        <Box
          sx={{
            mt: 0.5,
            ml: 3.5,
            pl: 1.5,
            pr: 0,
            pt: 0.5,
            pb: 0.25,
            borderLeft: `1px solid ${detailBorder}`,
            fontSize: "0.6875rem",
            lineHeight: 1.625,
            fontFamily: APP_MONO_FONT_FAMILY,
            whiteSpace: "pre-wrap",
            wordBreak: "break-word",
            color: isDark ? chatColors.subtle : "text.secondary",
            backgroundColor: "transparent",
            maxHeight: "256px",
            overflow: "auto",
          }}
        >
          {expandedBody}
        </Box>
      )}
    </Box>
  );
};

export default CollapsibleToolCall;
