import React, { FC, useState } from "react";
import Box from "@mui/material/Box";
import { useTheme } from "@mui/material/styles";
import { ChevronDown } from "lucide-react";

import { CollapsibleToolCall } from "./CollapsibleToolCall";
import { preserveDisclosureExpansion } from "./disclosureScroll";
import { getChatColors } from "./chatStyles";
import { APP_FONT_FAMILY } from "../../styles/typography";

export interface WorkLogEntry {
  id: string;
  toolName: string;
  status: string;
  body: string;
}

interface WorkLogProps {
  entries: WorkLogEntry[];
}

/**
 * Keeps a long sequence of agent activity out of the main response flow.
 * The current entry remains visible; older entries are available on demand.
 */
export const WorkLog: FC<WorkLogProps> = ({ entries }) => {
  const [expanded, setExpanded] = useState(false);
  const theme = useTheme();
  const isDark = theme.palette.mode === "dark";
  const chatColors = getChatColors(theme);
  const latestEntry = entries[entries.length - 1];
  const previousEntries = entries.slice(0, -1);
  const previousCount = Math.max(0, entries.length - 1);

  if (!latestEntry) return null;

  const toggleExpanded = (event: React.MouseEvent<HTMLElement>) => {
    if (!expanded) preserveDisclosureExpansion(event.currentTarget);
    setExpanded((value) => !value);
  };

  return (
    <Box sx={{ my: 0.25 }}>
      <CollapsibleToolCall
        toolName={latestEntry.toolName}
        status={latestEntry.status}
        body={latestEntry.body}
        dense
      />
      {previousCount > 0 && (
        <Box
          component="button"
          type="button"
          onClick={toggleExpanded}
          aria-expanded={expanded}
          sx={{
            display: "flex",
            alignItems: "center",
            width: "100%",
            minHeight: 24,
            m: 0,
            px: 0.25,
            py: 0.25,
            gap: 0.75,
            border: 0,
            borderRadius: "6px",
            background: "transparent",
            color: isDark ? chatColors.foreground : "text.primary",
            fontSize: "0.75rem",
            lineHeight: "20px",
            fontWeight: 500,
            fontFamily: APP_FONT_FAMILY,
            textAlign: "left",
            cursor: "pointer",
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
            <ChevronDown
              size={14}
              strokeWidth={1.8}
              style={{
                transform: `rotate(${expanded ? 180 : 0}deg)`,
                transition: "transform 200ms ease",
              }}
            />
          </Box>
          {expanded
            ? "Show fewer tool calls"
            : `+${previousCount} previous tool ${previousCount === 1 ? "call" : "calls"}`}
        </Box>
      )}
      {expanded &&
        previousEntries.map((entry) => (
          <CollapsibleToolCall
            key={entry.id}
            toolName={entry.toolName}
            status={entry.status}
            body={entry.body}
            dense
          />
        ))}
    </Box>
  );
};

export default WorkLog;
