import { FC } from "react";
import { Box, Button, Typography } from "@mui/material";
import { Clock3 } from "lucide-react";

interface SpecTaskLaunchWindowProps {
  phase: "queued" | "starting";
  mode: "planning" | "implementation";
  queueReason?: string;
  onMoveToBacklog?: () => void;
  isMovingToBacklog?: boolean;
}

interface SpecTaskLaunchState {
  status?: string;
  queueReason?: string;
  activeSessionId?: string;
  hasDesktopLifecycleState: boolean;
}

export const getSpecTaskLaunchPhase = ({
  status,
  queueReason,
  activeSessionId,
  hasDesktopLifecycleState,
}: SpecTaskLaunchState): "queued" | "starting" | null => {
  if (status === "queued_spec_generation" || status === "queued_implementation") {
    return queueReason?.trim() ? "queued" : "starting";
  }

  if (status === "spec_generation" || status === "implementation") {
    // planning_session_id is persisted before repository sync and StartDesktop.
    // Until StartDesktop writes lifecycle fields, the session is launching rather
    // than paused. A lifecycle state means the normal desktop UI can take over.
    if (!activeSessionId || !hasDesktopLifecycleState) return "starting";
  }

  return null;
};

const SpecTaskLaunchWindow: FC<SpecTaskLaunchWindowProps> = ({
  phase,
  mode,
  queueReason,
  onMoveToBacklog,
  isMovingToBacklog = false,
}) => {
  const isQueued = phase === "queued";
  const activity = mode === "implementation" ? "implementation" : "planning";

  return (
    <Box
      data-testid="spec-task-launch-window"
      sx={{
        flex: 1,
        minHeight: 0,
        display: "flex",
        alignItems: "center",
        justifyContent: "center",
        px: 3,
        bgcolor: "background.default",
      }}
    >
      {isQueued ? (
        <Box
          sx={{
            maxWidth: 560,
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            textAlign: "center",
          }}
        >
          <Box
            sx={{
              width: 52,
              height: 52,
              mb: 2.5,
              display: "grid",
              placeItems: "center",
              borderRadius: "50%",
              bgcolor: "action.hover",
              color: "text.secondary",
            }}
          >
            <Clock3 size={24} />
          </Box>
          <Typography variant="h5" sx={{ fontWeight: 650, mb: 1 }}>
            Task queued
          </Typography>
          <Typography color="text.secondary" sx={{ lineHeight: 1.65 }}>
            {queueReason || `Waiting for ${activity} capacity.`}
          </Typography>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 1.25 }}>
            It will start automatically when capacity is available.
          </Typography>
          {onMoveToBacklog && (
            <Button
              variant="text"
              color="inherit"
              onClick={onMoveToBacklog}
              disabled={isMovingToBacklog}
              sx={{ mt: 3 }}
            >
              {isMovingToBacklog ? "Moving..." : "Move to backlog"}
            </Button>
          )}
        </Box>
      ) : (
        <Typography
          role="status"
          aria-live="polite"
          sx={{
            color: "text.secondary",
            fontSize: { xs: "0.875rem", sm: "0.95rem" },
            fontWeight: 450,
            letterSpacing: "0.01em",
            animation: "desktopBootPulse 1.8s ease-in-out infinite",
            "@keyframes desktopBootPulse": {
              "0%, 100%": { opacity: 0.48 },
              "50%": { opacity: 1 },
            },
          }}
        >
          booting virtual desktop environment...
        </Typography>
      )}
    </Box>
  );
};

export default SpecTaskLaunchWindow;
