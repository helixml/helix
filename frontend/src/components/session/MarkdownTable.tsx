import React, { useEffect, useRef, useState } from "react";
import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import Menu from "@mui/material/Menu";
import MenuItem from "@mui/material/MenuItem";
import Tooltip from "@mui/material/Tooltip";
import { useTheme } from "@mui/material/styles";
import { Check, Copy, Maximize2, Minimize2 } from "lucide-react";
import { copyTextToClipboard } from "../../utils/clipboard";

function wrapInlineCode(code: string): string {
  const longestRun = [...(code.match(/`+/g) ?? [])].reduce(
    (max, run) => Math.max(max, run.length),
    0,
  );
  const fence = "`".repeat(Math.max(1, longestRun + (longestRun > 0 ? 1 : 0)));
  const padding = code.startsWith("`") || code.endsWith("`") ? " " : "";
  return `${fence}${padding}${code}${padding}${fence}`;
}

function serializeCellChildren(node: Node): string {
  return [...node.childNodes].map(serializeCellNode).join("");
}

function serializeCellNode(node: Node): string {
  if (node.nodeType === Node.TEXT_NODE) return node.textContent ?? "";
  if (node.nodeType !== Node.ELEMENT_NODE) return "";

  const element = node as Element;
  const content = serializeCellChildren(element);
  switch (element.tagName) {
    case "BR":
      return "\n";
    case "CODE":
      return wrapInlineCode(element.textContent ?? "");
    case "STRONG":
    case "B":
      return `**${content}**`;
    case "EM":
    case "I":
      return `*${content}*`;
    case "DEL":
    case "S":
      return `~~${content}~~`;
    case "A": {
      const href = element.getAttribute("href") ?? "";
      return /^https?:\/\//i.test(href) ? `[${content}](${href})` : content;
    }
    default:
      return content;
  }
}

function tableRows(table: HTMLTableElement): HTMLTableRowElement[] {
  return [...table.rows];
}

function rowCells(row: HTMLTableRowElement): HTMLTableCellElement[] {
  return [...row.cells];
}

function markdownCell(cell: HTMLTableCellElement): string {
  return serializeCellChildren(cell)
    .replace(/\n+/g, " ")
    .trim()
    .split("|")
    .join("\\|");
}

function markdownSeparator(cells: HTMLTableCellElement[]): string {
  const markers = cells.map((cell) => {
    const alignment = cell.style.textAlign || cell.getAttribute("align") || "";
    if (alignment === "center") return ":---:";
    if (alignment === "right") return "---:";
    return "---";
  });
  return `| ${markers.join(" | ")} |`;
}

export function serializeTableToMarkdown(table: HTMLTableElement): string {
  const rows = tableRows(table);
  if (rows.length === 0) return "";

  const lines: string[] = [];
  rows.forEach((row, index) => {
    const cells = rowCells(row);
    if (cells.length === 0) return;
    lines.push(`| ${cells.map(markdownCell).join(" | ")} |`);
    if (index === 0) lines.push(markdownSeparator(cells));
  });
  return lines.join("\n");
}

function csvCell(value: string): string {
  const normalized = value.replace(/\s+/g, " ").trim();
  return /[",\n]/.test(normalized)
    ? `"${normalized.split('"').join('""')}"`
    : normalized;
}

export function serializeTableToCsv(table: HTMLTableElement): string {
  return tableRows(table)
    .map((row) => rowCells(row).map((cell) => csvCell(cell.textContent ?? "")).join(","))
    .join("\n");
}

export default function MarkdownTable({
  children,
  ...props
}: React.ComponentProps<"table">) {
  const theme = useTheme();
  const tableRef = useRef<HTMLTableElement | null>(null);
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null);
  const [copied, setCopied] = useState(false);
  const [expanded, setExpanded] = useState(false);

  useEffect(
    () => () => {
      if (copiedTimerRef.current) clearTimeout(copiedTimerRef.current);
    },
    [],
  );

  const copyTable = async (format: "markdown" | "csv") => {
    setMenuAnchor(null);
    const table = tableRef.current;
    if (!table) return;

    const text =
      format === "markdown"
        ? serializeTableToMarkdown(table)
        : serializeTableToCsv(table);
    try {
      await copyTextToClipboard(text);
      setCopied(true);
      if (copiedTimerRef.current) clearTimeout(copiedTimerRef.current);
      copiedTimerRef.current = setTimeout(() => setCopied(false), 1200);
    } catch (error) {
      console.error("Failed to copy table", error);
    }
  };

  const toggleExpanded = () => {
    const table = tableRef.current;
    if (!table) return;

    if (!expanded) {
      const columnWidths = tableRows(table).reduce<number[]>((widths, row) => {
        rowCells(row).forEach((cell, columnIndex) => {
          widths[columnIndex] = Math.max(
            widths[columnIndex] ?? 0,
            cell.getBoundingClientRect().width,
          );
        });
        return widths;
      }, []);
      [...(table.tHead?.rows[0]?.cells ?? [])].forEach((cell, columnIndex) => {
        cell.style.minWidth = `${columnWidths[columnIndex] ?? cell.getBoundingClientRect().width}px`;
      });
    }

    setExpanded((value) => !value);
  };

  const copyLabel = copied ? "Copied" : "Copy table";
  const expandLabel = expanded ? "Collapse table cells" : "Expand table cells";

  return (
    <Box
      className="chat-markdown-table-container"
      data-expanded={expanded ? "true" : "false"}
    >
      <Box className="chat-markdown-table-scroll">
        <table ref={tableRef} {...props}>
          {children}
        </table>
      </Box>
      <Box className="chat-markdown-table-footer">
        <Tooltip title={expandLabel} placement="top" arrow>
          <IconButton
            className="chat-markdown-table-action"
            size="small"
            aria-label={expandLabel}
            aria-pressed={expanded}
            onClick={toggleExpanded}
          >
            {expanded ? <Minimize2 size={14} /> : <Maximize2 size={14} />}
          </IconButton>
        </Tooltip>
        <Tooltip title={copyLabel} placement="top">
          <IconButton
            className="chat-markdown-table-action"
            size="small"
            aria-label={copyLabel}
            aria-haspopup="menu"
            aria-expanded={menuAnchor ? "true" : undefined}
            onClick={(event) => setMenuAnchor(event.currentTarget)}
          >
            {copied ? <Check size={14} /> : <Copy size={14} />}
          </IconButton>
        </Tooltip>
        <Menu
          anchorEl={menuAnchor}
          open={!!menuAnchor}
          onClose={() => setMenuAnchor(null)}
          anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
          transformOrigin={{ vertical: "top", horizontal: "right" }}
          PaperProps={{
            sx: {
              mt: 0.5,
              minWidth: 150,
              p: 0.5,
              borderRadius: "6px",
              border: `1px solid ${theme.palette.mode === "dark" ? "rgba(255, 255, 255, 0.08)" : "rgba(0, 0, 0, 0.1)"}`,
              backgroundColor: theme.palette.mode === "dark" ? "#1c1c1f" : "#ffffff",
              backgroundImage: "none",
              boxShadow: theme.palette.mode === "dark"
                ? "0 8px 24px rgba(0, 0, 0, 0.5)"
                : "0 8px 24px rgba(0, 0, 0, 0.14)",
              "& .MuiMenuItem-root": {
                minHeight: 32,
                px: 1.25,
                py: 0.5,
                borderRadius: "4px",
                fontSize: "0.8125rem",
              },
            },
          }}
        >
          <MenuItem onClick={() => void copyTable("markdown")}>
            Copy as Markdown
          </MenuItem>
          <MenuItem onClick={() => void copyTable("csv")}>
            Copy as CSV
          </MenuItem>
        </Menu>
      </Box>
    </Box>
  );
}
