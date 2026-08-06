import { FC } from "react";
import { Box, Button, Chip, CircularProgress, Typography } from "@mui/material";
import { Bot, Clock3, MessageSquare, Monitor, Sparkles } from "lucide-react";

interface SpecTaskLaunchWindowProps {
  phase: "queued" | "starting";
  mode: "planning" | "implementation";
  queueReason?: string;
  onMoveToBacklog?: () => void;
  isMovingToBacklog?: boolean;
}

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
        p: { xs: 1.5, sm: 3 },
        bgcolor: "background.default",
      }}
    >
      <Box
        sx={{
          width: "100%",
          maxWidth: 960,
          minHeight: { xs: 420, md: 520 },
          display: "flex",
          flexDirection: "column",
          overflow: "hidden",
          border: "1px solid",
          borderColor: "divider",
          borderRadius: 2.5,
          bgcolor: "background.paper",
          boxShadow: (theme) =>
            theme.palette.mode === "light"
              ? "0 24px 70px rgba(15, 23, 42, 0.09)"
              : "0 24px 70px rgba(0, 0, 0, 0.28)",
        }}
      >
        <Box
          sx={{
            minHeight: 48,
            px: 2,
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            gap: 2,
            borderBottom: "1px solid",
            borderColor: "divider",
          }}
        >
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.75 }}>
            {[0, 1, 2].map((dot) => (
              <Box
                key={dot}
                sx={{ width: 7, height: 7, borderRadius: "50%", bgcolor: "action.disabled" }}
              />
            ))}
          </Box>
          <Chip
            size="small"
            icon={isQueued ? <Clock3 size={14} /> : <Sparkles size={14} />}
            label={isQueued ? "Queued" : "Starting"}
            color={isQueued ? "default" : "primary"}
            variant={isQueued ? "outlined" : "filled"}
            sx={{ height: 26, fontWeight: 600 }}
          />
        </Box>

        {isQueued ? (
          <Box
            sx={{
              flex: 1,
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "center",
              textAlign: "center",
              px: 3,
              py: 6,
            }}
          >
            <Box
              sx={{
                width: 64,
                height: 64,
                mb: 2.5,
                display: "grid",
                placeItems: "center",
                borderRadius: 3,
                bgcolor: "action.hover",
                color: "text.secondary",
              }}
            >
              <Clock3 size={30} />
            </Box>
            <Typography variant="h5" sx={{ fontWeight: 650, mb: 1 }}>
              Task queued
            </Typography>
            <Typography color="text.secondary" sx={{ maxWidth: 560, lineHeight: 1.65 }}>
              {queueReason || `Waiting for ${activity} capacity.`}
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 1.25 }}>
              It will start automatically when capacity is available. Chat and desktop will open here.
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
          <Box
            sx={{
              flex: 1,
              minHeight: 0,
              display: "grid",
              gridTemplateColumns: { xs: "1fr", md: "minmax(260px, 0.75fr) 1.25fr" },
            }}
          >
            <Box
              sx={{
                minHeight: { xs: 170, md: "auto" },
                display: "flex",
                flexDirection: "column",
                borderRight: { md: "1px solid" },
                borderBottom: { xs: "1px solid", md: "none" },
                borderColor: "divider",
              }}
            >
              <Box
                sx={{
                  minHeight: 44,
                  px: 1.75,
                  display: "flex",
                  alignItems: "center",
                  gap: 1,
                  borderBottom: "1px solid",
                  borderColor: "divider",
                  color: "text.secondary",
                }}
              >
                <MessageSquare size={16} />
                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                  Chat
                </Typography>
              </Box>
              <Box
                sx={{
                  flex: 1,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  gap: 1.25,
                  px: 3,
                  color: "text.secondary",
                }}
              >
                <Bot size={21} />
                <Typography variant="body2">Connecting your agent…</Typography>
              </Box>
            </Box>

            <Box sx={{ minHeight: { xs: 230, md: "auto" }, display: "flex", flexDirection: "column" }}>
              <Box
                sx={{
                  minHeight: 44,
                  px: 1.75,
                  display: "flex",
                  alignItems: "center",
                  gap: 1,
                  borderBottom: "1px solid",
                  borderColor: "divider",
                  color: "text.secondary",
                }}
              >
                <Monitor size={16} />
                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                  Desktop
                </Typography>
              </Box>
              <Box
                sx={{
                  flex: 1,
                  display: "flex",
                  flexDirection: "column",
                  alignItems: "center",
                  justifyContent: "center",
                  textAlign: "center",
                  px: 3,
                  py: 4,
                  background: (theme) =>
                    theme.palette.mode === "light"
                      ? "radial-gradient(circle at 50% 45%, rgba(93, 95, 239, 0.08), transparent 48%)"
                      : "radial-gradient(circle at 50% 45%, rgba(139, 141, 255, 0.12), transparent 48%)",
                }}
              >
                <CircularProgress size={38} thickness={3.5} />
                <Typography variant="h6" sx={{ mt: 2.5, fontWeight: 650 }}>
                  Starting {activity}
                </Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.75, maxWidth: 380 }}>
                  Preparing the workspace and launching the coding agent. This window will update automatically.
                </Typography>
              </Box>
            </Box>
          </Box>
        )}
      </Box>
    </Box>
  );
};

export default SpecTaskLaunchWindow;
