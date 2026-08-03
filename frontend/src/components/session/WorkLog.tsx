import React, { FC, useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import Typography from "@mui/material/Typography";
import ExpandLessIcon from "@mui/icons-material/ExpandLess";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import { useTheme } from "@mui/material/styles";

import { CollapsibleToolCall } from "./CollapsibleToolCall";
import { preserveDisclosureExpansion } from "./disclosureScroll";

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
  const latestEntry = entries[entries.length - 1];
  const previousCount = Math.max(0, entries.length - 1);

  if (!latestEntry) return null;

  const toggleExpanded = (event: React.MouseEvent<HTMLElement>) => {
    if (!expanded) preserveDisclosureExpansion(event.currentTarget);
    setExpanded((value) => !value);
  };

  if (!expanded) {
    return (
      <Box sx={{ my: 1 }}>
        <CollapsibleToolCall
          toolName={latestEntry.toolName}
          status={latestEntry.status}
          body={latestEntry.body}
          dense
        />
        {previousCount > 0 && (
          <Button
            size="small"
            onClick={toggleExpanded}
            aria-expanded={false}
            endIcon={<ExpandMoreIcon sx={{ fontSize: 16 }} />}
            sx={{
              minHeight: 24,
              px: 0.5,
              color: isDark ? "rgba(255,255,255,0.65)" : "text.secondary",
              fontSize: "0.72rem",
              textTransform: "none",
              "&:hover": { backgroundColor: "transparent" },
            }}
          >
            +{previousCount} previous log {previousCount === 1 ? "entry" : "entries"}
          </Button>
        )}
      </Box>
    );
  }

  return (
    <Box sx={{ my: 1 }}>
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          minHeight: 28,
          color: isDark ? "rgba(255,255,255,0.7)" : "text.secondary",
        }}
      >
        <Typography
          variant="caption"
          sx={{ flex: 1, fontWeight: 600, letterSpacing: "0.01em" }}
        >
          Work Log
        </Typography>
        <Button
          size="small"
          onClick={toggleExpanded}
          aria-expanded={true}
          endIcon={<ExpandLessIcon sx={{ fontSize: 16 }} />}
          sx={{
            minHeight: 24,
            px: 0.5,
            color: "inherit",
            fontSize: "0.72rem",
            textTransform: "none",
            "&:hover": { backgroundColor: "transparent" },
          }}
        >
          Show fewer log entries
        </Button>
      </Box>
      {entries.map((entry) => (
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
