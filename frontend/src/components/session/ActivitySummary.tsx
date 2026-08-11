import React, { FC, ReactNode, useEffect, useRef, useState } from "react";
import Box from "@mui/material/Box";
import Typography from "@mui/material/Typography";
import { useTheme } from "@mui/material/styles";
import { ChevronDown, ChevronUp } from "lucide-react";

import StreamingIndicator from "./StreamingIndicator";
import { getChatColors } from "./chatStyles";
import { APP_FONT_FAMILY } from "../../styles/typography";

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
  const dividerColor = theme.palette.mode === "dark"
    ? "rgba(255,255,255,0.045)"
    : "rgba(0,0,0,0.055)";
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
    <Box
      sx={{
        mt: 0.5,
        mb: 1,
        pt: 0.5,
        pb: 1,
        borderBottom: `1px solid ${dividerColor}`,
      }}
    >
      <Box
        component={hasActivity ? "button" : "div"}
        type={hasActivity ? "button" : undefined}
        onClick={hasActivity ? toggleExpanded : undefined}
        onKeyDown={handleHeaderKeyDown}
        aria-expanded={hasActivity ? expanded : undefined}
        aria-label={hasActivity ? (expanded ? "Hide work log" : "Show work log") : undefined}
        sx={{
          display: "flex",
          alignItems: "center",
          gap: 0.5,
          width: "fit-content",
          minHeight: 20,
          m: 0,
          p: "0 4px",
          border: 0,
          borderRadius: "6px",
          background: "transparent",
          font: "inherit",
          cursor: hasActivity ? "pointer" : "default",
          color: textColor,
          fontVariantNumeric: "tabular-nums",
          transition: "color 150ms ease",
          "&:hover": hasActivity ? { color: chatColors.foreground } : undefined,
          "&:focus-visible": hasActivity
            ? { outline: "2px solid currentColor", outlineOffset: -2 }
            : undefined,
        }}
      >
        {isStreaming && <StreamingIndicator />}
        <Typography
          variant="body2"
          sx={{
            fontSize: "0.75rem",
            lineHeight: "20px",
            color: "inherit",
            fontFamily: APP_FONT_FAMILY,
          }}
        >
          {isStreaming
            ? `Working for ${formatActivityDuration(elapsedMs)}`
            : `Worked for ${formatActivityDuration(elapsedMs)}`}
        </Typography>
        {hasActivity && (
          <Box
            component="span"
            sx={{
              display: "inline-flex",
              width: 14,
              height: 14,
              alignItems: "center",
              justifyContent: "center",
            }}
          >
            {expanded ? (
              <ChevronUp size={14} strokeWidth={1.8} />
            ) : (
              <ChevronDown size={14} strokeWidth={1.8} />
            )}
          </Box>
        )}
      </Box>
      {expanded ? children : null}
    </Box>
  );
};

export default ActivitySummary;
