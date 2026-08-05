import React, { FC, useState } from "react";
import Box from "@mui/material/Box";
import Button from "@mui/material/Button";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import { useTheme } from "@mui/material/styles";

import { CollapsibleToolCall } from "./CollapsibleToolCall";
import { preserveDisclosureExpansion } from "./disclosureScroll";
import { getChatColors } from "./chatStyles";
import { APP_MONO_FONT_FAMILY } from "../../styles/typography";

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
    <Box sx={{ my: 0.5 }}>
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
          aria-expanded={expanded}
          startIcon={
            <ExpandMoreIcon
              sx={{
                fontSize: 15,
                color: isDark ? chatColors.subtle : "rgba(0,0,0,0.45)",
                transform: `translateX(-1px) rotate(${expanded ? 180 : 0}deg)`,
              }}
            />
          }
          sx={{
            minHeight: 24,
            px: 0,
            gap: 0.75,
            color: isDark ? chatColors.foreground : "text.primary",
            fontSize: "0.76rem",
            fontWeight: 600,
            fontFamily: APP_MONO_FONT_FAMILY,
            textTransform: "none",
            "&:hover": { backgroundColor: "transparent" },
            "& .MuiButton-startIcon": { m: 0 },
          }}
        >
          +{previousCount} previous tool{" "}
          {previousCount === 1 ? "call" : "calls"}
        </Button>
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
