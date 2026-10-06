import React, { FC, ReactNode } from "react";
import { Box, Button, CircularProgress, Typography } from "@mui/material";
import { CircleCheck, CircleSlash, LogIn, MonitorPlay, Play } from "lucide-react";

export type TaskSessionPlaceholderTone =
  | "finished"
  | "archived"
  | "paused"
  | "connecting";

interface TaskSessionPlaceholderProps {
  tone: TaskSessionPlaceholderTone;
  title: string;
  description: string;
  /**
   * Why the desktop is not running, when the backend told us. Without this a
   * refused launch is indistinguishable from an ordinary pause, and the start
   * action just fails the same way again.
   */
  detail?: string;
  /** Omit to render an informational state with no action. */
  onStart?: () => void;
  starting?: boolean;
  startLabel?: string;
  /**
   * The action that actually unblocks the user, when starting the desktop
   * cannot. A refused launch offers this as the primary button and demotes
   * the start action, because retrying fails identically until it is done.
   */
  primaryAction?: { label: string; onClick: () => void };
  /**
   * "overlay" renders on an opaque surface, for use on top of a dimmed
   * screenshot backdrop where a near-transparent card would not read.
   */
  variant?: "plain" | "overlay";
}

const TONE_ICONS: Record<TaskSessionPlaceholderTone, ReactNode> = {
  finished: <CircleCheck size={22} />,
  archived: <CircleSlash size={22} />,
  paused: <MonitorPlay size={22} />,
  // A sandbox that is still registering its bridge is on its way to working,
  // so it gets progress rather than a stopped-looking icon.
  connecting: <CircularProgress size={20} color="inherit" />,
};

/**
 * The single empty state for "this task has no running desktop".
 *
 * Desktop, Changes and Files all reach the same dead end when a task's
 * sandbox is stopped, and each used to say something different about it —
 * Changes said "Could not load workspace changes", which reads as a fault
 * rather than the ordinary end of a finished task. One component keeps the
 * explanation and the recovery action consistent across every surface.
 */
const TaskSessionPlaceholder: FC<TaskSessionPlaceholderProps> = ({
  tone,
  title,
  description,
  detail,
  onStart,
  starting = false,
  startLabel = "Start desktop",
  primaryAction,
  variant = "plain",
}) => (
  <Box
    sx={{
      flex: 1,
      minHeight: 0,
      display: "flex",
      alignItems: "center",
      justifyContent: "center",
      p: 4,
    }}
  >
    <Box
      sx={(theme) => ({
        maxWidth: 380,
        width: "100%",
        textAlign: "center",
        px: 3,
        py: 3.5,
        borderRadius: 1,
        border: "1px solid",
        borderColor:
          theme.palette.mode === "light"
            ? "rgba(0, 0, 0, 0.08)"
            : "rgba(255, 255, 255, 0.08)",
        bgcolor:
          variant === "overlay"
            ? "background.paper"
            : theme.palette.mode === "light"
              ? "rgba(0, 0, 0, 0.01)"
              : "rgba(255, 255, 255, 0.02)",
      })}
    >
      <Box
        sx={{
          display: "flex",
          justifyContent: "center",
          mb: 1.5,
          color: tone === "archived" ? "text.disabled" : "text.secondary",
        }}
      >
        {TONE_ICONS[tone]}
      </Box>
      <Typography variant="subtitle2" sx={{ fontWeight: 600, mb: 0.5 }}>
        {title}
      </Typography>
      <Typography variant="body2" color="text.secondary">
        {description}
      </Typography>
      {detail && (
        <Typography
          variant="caption"
          sx={{ display: "block", mt: 1.5, color: "error.main", textAlign: "left" }}
        >
          {detail}
        </Typography>
      )}
      {(primaryAction || onStart) && (
        <Box sx={{ display: "flex", justifyContent: "center", gap: 1, mt: 2.5, flexWrap: "wrap" }}>
          {primaryAction && (
            <Button
              variant="contained"
              size="small"
              startIcon={<LogIn size={14} />}
              onClick={primaryAction.onClick}
              sx={{ textTransform: "none" }}
            >
              {primaryAction.label}
            </Button>
          )}
          {onStart && (
            <Button
              variant="outlined"
              size="small"
              color="inherit"
              startIcon={
                starting ? <CircularProgress size={14} color="inherit" /> : <Play size={14} />
              }
              onClick={onStart}
              disabled={starting}
              sx={{ textTransform: "none" }}
            >
              {starting ? "Starting…" : startLabel}
            </Button>
          )}
        </Box>
      )}
    </Box>
  </Box>
);

export default TaskSessionPlaceholder;
