import React, { FC, ReactNode, useEffect, useRef, useState } from "react";
import Box from "@mui/material/Box";
import IconButton from "@mui/material/IconButton";
import Typography from "@mui/material/Typography";
import { useTheme } from "@mui/material/styles";
import { ChevronDown, ChevronUp } from "lucide-react";

import StreamingIndicator from "./StreamingIndicator";
import { getChatColors } from "./chatStyles";
import { APP_MONO_FONT_FAMILY } from "../../styles/typography";

export const formatActivityDuration = (durationMs: number) => {
  const totalSeconds = Math.max(0, Math.floor(durationMs / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;

  if (minutes === 0) return `${seconds}s`;
  return `${minutes}m ${seconds}s`;
};

interface ActivitySummaryProps {
  children?: ReactNode;
  durationMs?: number;
  hasActivity: boolean;
  isStreaming: boolean;
  startedAt?: number;
}

/**
 * Keeps reasoning and tool activity behind one response-level disclosure.
 * The final assistant prose is rendered outside this component and remains
 * visible when the activity is collapsed.
 */
const ActivitySummary: FC<ActivitySummaryProps> = ({
  children,
  durationMs = 0,
  hasActivity,
  isStreaming,
  startedAt,
}) => {
  const theme = useTheme();
  const chatColors = getChatColors(theme);
  const [expanded, setExpanded] = useState(false);
  const [elapsedMs, setElapsedMs] = useState(durationMs);
  const lastElapsedMsRef = useRef(durationMs);
  const textColor = theme.palette.mode === "dark" ? chatColors.subtle : "text.secondary";

  useEffect(() => {
    if (!isStreaming) {
      const completedDuration = durationMs || lastElapsedMsRef.current;
      setElapsedMs(completedDuration);
      lastElapsedMsRef.current = completedDuration;
      return;
    }

    const started = startedAt || Date.now();
    const updateElapsed = () => {
      const nextElapsed = Math.max(0, Date.now() - started);
      setElapsedMs(nextElapsed);
      lastElapsedMsRef.current = nextElapsed;
    };
    updateElapsed();
    const interval = window.setInterval(updateElapsed, 1000);
    return () => window.clearInterval(interval);
  }, [durationMs, isStreaming, startedAt]);

  const toggleExpanded = () => {
    setExpanded((value) => !value);
  };

  const handleHeaderKeyDown = (event: React.KeyboardEvent<HTMLElement>) => {
    if (!hasActivity || (event.key !== "Enter" && event.key !== " ")) return;
    event.preventDefault();
    toggleExpanded();
  };

  return (
    <Box sx={{ my: 0.75 }}>
      <Box
        onClick={hasActivity ? toggleExpanded : undefined}
        onKeyDown={handleHeaderKeyDown}
        role={hasActivity ? "button" : undefined}
        tabIndex={hasActivity ? 0 : undefined}
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 1,
          minHeight: 24,
          cursor: hasActivity ? "pointer" : "default",
          color: textColor,
          "&:focus-visible": hasActivity
            ? { outline: "1px solid currentColor", outlineOffset: 2 }
            : undefined,
        }}
      >
        {isStreaming && <StreamingIndicator />}
        <Typography
          variant="body2"
          sx={{
            flex: 1,
            fontSize: "0.76rem",
            color: "inherit",
            fontFamily: APP_MONO_FONT_FAMILY,
          }}
        >
          {isStreaming
            ? `Working for ${formatActivityDuration(elapsedMs)}`
            : `Worked for ${formatActivityDuration(elapsedMs)}`}
        </Typography>
        {hasActivity && (
          <IconButton
            size="small"
            onClick={(event) => {
              event.stopPropagation();
              toggleExpanded();
            }}
            aria-expanded={expanded}
            aria-label={expanded ? "Hide work log" : "Show work log"}
            sx={{
              p: 0,
              color: "inherit",
              "&:hover": { backgroundColor: "transparent" },
            }}
          >
            {expanded ? (
              <ChevronUp size={15} strokeWidth={1.8} />
            ) : (
              <ChevronDown size={15} strokeWidth={1.8} />
            )}
          </IconButton>
        )}
      </Box>
      {expanded ? children : null}
    </Box>
  );
};

export default ActivitySummary;
