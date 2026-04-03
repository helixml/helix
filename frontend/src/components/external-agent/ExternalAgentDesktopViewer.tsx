import React, { FC, useState, useEffect, useCallback, useRef, useMemo } from "react";
import {
  Box,
  Button,
  Typography,
  CircularProgress,
  IconButton,
  Tooltip,
  Collapse,
} from "@mui/material";
import PlayArrow from "@mui/icons-material/PlayArrow";
import ChatIcon from "@mui/icons-material/Chat";
import ChevronRightIcon from "@mui/icons-material/ChevronRight";

import DesktopStreamViewer from "./DesktopStreamViewer";
import ScreenshotViewer from "./ScreenshotViewer";
import SandboxDropZone from "./SandboxDropZone";
import EmbeddedSessionView from "../session/EmbeddedSessionView";
import RobustPromptInput from "../common/RobustPromptInput";
import useApi from "../../hooks/useApi";
import useSnackbar from "../../hooks/useSnackbar";
import { useStreaming } from "../../contexts/streaming";
import { SESSION_TYPE_TEXT } from "../../types";
import { Api } from "../../api/api";
import { useQueryClient } from "@tanstack/react-query";
import { GET_SESSION_QUERY_KEY, useGetSession } from "../../services/sessionService";

// Hook to track sandbox container state for external agent sessions
// Exported for use in SpecTaskDetailContent.tsx toolbar buttons
// Uses React Query (useGetSession) for automatic deduplication — multiple components
// polling the same sessionId share a single network request.
export const useSandboxState = (sessionId: string, enabled: boolean = true) => {
  const { data: sessionResponse } = useGetSession(sessionId, {
    enabled: enabled && !!sessionId,
    refetchInterval: 3000, // Poll every 3 seconds
  });

  const { sandboxState, statusMessage } = useMemo(() => {
    if (!sessionResponse?.data) {
      return { sandboxState: "loading", statusMessage: "" };
    }

    const session = sessionResponse.data;
    const status = session.config?.external_agent_status || "";
    const desiredState = (session.config as (typeof session.config & { desired_state?: string }))?.desired_state || "";
    const hasContainer = !!session.config?.container_name;
    const msg = session.config?.status_message || "";

    // Map session metadata to sandbox state
    // Check stopped status first - it takes priority from the backend check
    let state: string;
    if (status === "stopped") {
      state = "absent";
    } else if (
      status === "running" ||
      (hasContainer && desiredState === "running")
    ) {
      state = "running";
    } else if (status === "starting") {
      state = "starting";
    } else if (desiredState === "stopped") {
      state = "absent";
    } else if (!hasContainer && desiredState === "running") {
      state = "starting";
    } else if (!hasContainer) {
      state = "absent";
    } else {
      state = hasContainer ? "running" : "absent";
    }

    return { sandboxState: state, statusMessage: msg };
  }, [sessionResponse?.data]);

  // Backend now returns 'starting' state for recently-created containers
  // Include 'loading' in isStarting to prevent DesktopStreamViewer from mounting
  // before we know the real state (avoids mount/unmount flicker)
  const isRunning = sandboxState === "running" || sandboxState === "resumable";
  const isStarting = sandboxState === "starting" || sandboxState === "loading";
  // Show "paused" only if container was previously running but is now absent
  const isPaused = sandboxState === "absent";

  return { sandboxState, isRunning, isPaused, isStarting, statusMessage };
};

interface ExternalAgentDesktopViewerProps {
  sessionId: string;
  sandboxId?: string;
  height?: number; // Optional - required for screenshot mode, ignored for stream mode (uses flex)
  mode?: "screenshot" | "stream"; // Screenshot mode for Kanban cards, stream mode for floating window
  onClientIdCalculated?: (clientId: string) => void;
  // Display settings from app's ExternalAgentConfig
  displayWidth?: number;
  displayHeight?: number;
  displayFps?: number;
  // Session panel settings (for stream mode only)
  showSessionPanel?: boolean; // Enable the collapsible session panel feature
  specTaskId?: string; // For prompt history sync
  projectId?: string; // For prompt history sync
  apiClient?: Api<unknown>["api"]; // For prompt history sync
  defaultPanelOpen?: boolean; // Default state of the session panel (default: false)
  startupErrorMessage?: string;
  // Pre-computed sandbox state from the task list (avoids per-card session polling on Kanban)
  initialSandboxState?: string;
  initialSandboxStatusMessage?: string;
}

const ExternalAgentDesktopViewer: FC<ExternalAgentDesktopViewerProps> = ({
  sessionId,
  sandboxId,
  height,
  mode = "stream", // Default to stream for floating window
  onClientIdCalculated,
  displayWidth,
  displayHeight,
  displayFps,
  showSessionPanel = false,
  specTaskId,
  projectId,
  apiClient,
  defaultPanelOpen = false,
  startupErrorMessage,
  initialSandboxState,
  initialSandboxStatusMessage,
}) => {
  const api = useApi();
  const snackbar = useSnackbar();
  const queryClient = useQueryClient();
  const { NewInference, setCurrentSessionId } = useStreaming();
  // When initialSandboxState is provided (Kanban screenshot mode), skip polling — state comes from the task list.
  const pollingEnabled = initialSandboxState === undefined;
  const polled = useSandboxState(sessionId, pollingEnabled);
  const sandboxStateValue = initialSandboxState ?? polled.sandboxState;
  const statusMessage = initialSandboxStatusMessage ?? polled.statusMessage;
  const isRunning = sandboxStateValue === "running" || sandboxStateValue === "resumable";
  const isStarting = sandboxStateValue === "starting" || sandboxStateValue === "loading";
  const isPaused = sandboxStateValue === "absent";
  const [isResuming, setIsResuming] = useState(false);
  // Track if we've ever been running - once running, keep stream mounted to avoid fullscreen exit
  const [hasEverBeenRunning, setHasEverBeenRunning] = useState(false);
  // Session panel state
  const [sessionPanelOpen, setSessionPanelOpen] = useState(defaultPanelOpen);
  // Track uploaded file paths to append to prompt input (uses unique key to trigger append)
  const [uploadedFilePath, setUploadedFilePath] = useState<
    string | undefined
  >();
  const uploadCountRef = useRef(0);
  const desktopLimitError =
    startupErrorMessage && startupErrorMessage.toLowerCase().startsWith('desktop limit reached')
      ? startupErrorMessage
      : undefined;

  // Track how long we've been in "starting" state so we can show a recovery UI
  // if the desktop fails to start within 2 minutes (issue #5 from ZFS deployment).
  const startingStartTimeRef = useRef<number | null>(null);
  const [startingTooLong, setStartingTooLong] = useState(false);
  const [isStopping, setIsStopping] = useState(false);

  useEffect(() => {
    if (isStarting) {
      if (startingStartTimeRef.current === null) {
        startingStartTimeRef.current = Date.now();
        setStartingTooLong(false);
      }
      const check = setInterval(() => {
        if (startingStartTimeRef.current !== null && Date.now() - startingStartTimeRef.current > 120_000) {
          setStartingTooLong(true);
        }
      }, 5000);
      return () => clearInterval(check);
    } else {
      startingStartTimeRef.current = null;
      setStartingTooLong(false);
    }
  }, [isStarting]);

  const handleStopFromStarting = async (e?: React.MouseEvent) => {
    e?.stopPropagation();
    setIsStopping(true);
    try {
      await api.getApiClient().v1SessionsStopExternalAgentDelete(sessionId);
    } catch (error: any) {
      console.error("Failed to stop session:", error);
      snackbar.error(error?.message || "Failed to stop desktop");
    } finally {
      setIsStopping(false);
    }
  };

  // NOTE: WebSocket subscription is handled by parent components (SpecTaskDetailContent, etc.)
  // based on whether the chat panel is visible. This component no longer subscribes directly
  // to avoid duplicate subscriptions and to allow proper disconnect when chat is collapsed.

  // Handle file upload from drag/drop - append path to prompt input with a unique key
  const handleFileUploaded = useCallback((filePath: string) => {
    uploadCountRef.current += 1;
    // Include counter to make each value unique and trigger the useEffect in RobustPromptInput
    setUploadedFilePath(`${filePath}#${uploadCountRef.current}`);
  }, []);

  // Handle image paste in RobustPromptInput - uploads without opening file manager
  const handleImagePaste = useCallback(
    async (file: File): Promise<string | null> => {
      try {
        const response = await api
          .getApiClient()
          .v1ExternalAgentsUploadCreate(
            sessionId,
            { file },
            { open_file_manager: false },
          );

        if (response.data?.path) {
          snackbar.success(`${file.name} uploaded to ~/work/incoming`);
          return response.data.path;
        }
        return null;
      } catch (error) {
        console.error("Image upload error:", error);
        snackbar.error("Failed to upload image");
        return null;
      }
    },
    [sessionId],
  );

  // Once running, remember it to prevent unmounting on transient state changes
  useEffect(() => {
    if (isRunning && !hasEverBeenRunning) {
      setHasEverBeenRunning(true);
    }
  }, [isRunning, hasEverBeenRunning]);

  const handleResume = async (e?: React.MouseEvent) => {
    e?.stopPropagation(); // Prevent click from bubbling to parent (e.g., Kanban card navigation)
    setIsResuming(true);
    try {
      await api.getApiClient().v1SessionsResumeCreate(sessionId);
      snackbar.success("External agent started successfully");
      // Success - don't reset isResuming here
      // The useEffect below will reset it when container state changes
    } catch (error: any) {
      console.error("Failed to resume agent:", error);
      const message = error?.response?.data || error?.message || "Failed to start agent";
      snackbar.error(typeof message === "string" ? message.trim() : "Failed to start agent");
      // Error - reset so user can retry
      setIsResuming(false);
    }
  };

  // Reset resuming state when container state changes
  // This handles both success (isStarting/isRunning) and failure (isPaused after attempt)
  useEffect(() => {
    // Only reset if we were resuming and state has resolved
    if (isResuming && (isRunning || isStarting)) {
      // Container started successfully - button will disappear anyway
      setIsResuming(false);
    }
  }, [isRunning, isStarting, isResuming]);

  // Also reset if container goes back to paused (failed to start)
  // Use a ref to track if we were in a non-paused state
  const wasNotPausedRef = useRef(false);
  useEffect(() => {
    if (!isPaused) {
      wasNotPausedRef.current = true;
    } else if (wasNotPausedRef.current && isPaused) {
      // Transitioned from non-paused back to paused (container failed)
      setIsResuming(false);
      wasNotPausedRef.current = false;
    }
  }, [isPaused]);

  // Handler for sending messages from the session panel
  // IMPORTANT: This hook must be before any early returns to satisfy React's rules of hooks
  const handleSendMessage = useCallback(
    async (message: string, interrupt?: boolean) => {
      await NewInference({
        type: SESSION_TYPE_TEXT,
        message,
        sessionId,
        interrupt: interrupt ?? true,
      });
      // Immediately invalidate session query to refresh the interaction list
      // This ensures the user's message appears right away without waiting for poll
      queryClient.invalidateQueries({
        queryKey: GET_SESSION_QUERY_KEY(sessionId),
      });
    },
    [NewInference, sessionId, queryClient],
  );

  // Session panel width
  const SESSION_PANEL_WIDTH = 400;

  // For screenshot mode (Kanban cards), use traditional early-return rendering
  // For stream mode (floating window), keep stream mounted to prevent fullscreen exit on hiccups

  // Screenshot mode: use traditional early-return rendering
  // Use height prop if provided, otherwise fill parent container (for aspect-ratio containers)
  const screenshotHeight = height ?? "100%";

  if (mode === "screenshot") {
    // Starting state - show spinner
    if (isStarting) {
      return (
        <Box
          sx={{
            width: "100%",
            height: screenshotHeight,
            position: "relative",
            border: "1px solid",
            borderColor: "divider",
            borderRadius: 1,
            overflow: "hidden",
            backgroundColor: "#1a1a1a",
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            gap: 2,
          }}
        >
          {desktopLimitError ? (
            <Typography variant="body2" sx={{ color: 'error.main', fontWeight: 500, textAlign: 'center', px: 2 }}>
              {desktopLimitError}
            </Typography>
          ) : (
            <>
              <CircularProgress size={32} sx={{ color: 'primary.main' }} />
              <Typography variant="body2" sx={{ color: 'rgba(255,255,255,0.7)', fontWeight: 500 }}>
                {startingTooLong ? "Desktop may have failed to start" : (statusMessage || "Starting Desktop...")}
              </Typography>
              <Button
                variant="outlined"
                size="small"
                onClick={handleStopFromStarting}
                disabled={isStopping}
                sx={{ color: 'rgba(255,255,255,0.6)', borderColor: 'rgba(255,255,255,0.3)', mt: 1 }}
              >
                {isStopping ? "Stopping..." : "Stop"}
              </Button>
            </>
          )}
        </Box>
      );
    }

    if (isPaused) {
      // Show the last screenshot (served from PausedScreenshotPath on the API) behind
      // a semi-transparent overlay. onError hides the img gracefully if unavailable.
      const screenshotUrl = `/api/v1/external-agents/${sessionId}/screenshot`;
      return (
        <Box
          sx={{
            width: "100%",
            height: screenshotHeight,
            position: "relative",
            border: "1px solid",
            borderColor: "divider",
            borderRadius: 1,
            overflow: "hidden",
            backgroundColor: "#1a1a1a",
          }}
        >
          <Box
            component="img"
            src={screenshotUrl}
            alt="Paused Desktop"
            sx={{
              width: "100%",
              height: "100%",
              objectFit: "contain",
              filter: "grayscale(0.5) brightness(0.7) blur(1px)",
              opacity: 0.6,
            }}
            onError={(e: React.SyntheticEvent<HTMLImageElement>) => {
              e.currentTarget.style.display = "none";
            }}
          />
          <Box
            sx={{
              position: "absolute",
              top: 0,
              left: 0,
              right: 0,
              bottom: 0,
              backgroundColor: "rgba(0,0,0,0.3)",
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              justifyContent: "center",
              gap: 2,
            }}
          >
            <Typography
              variant="body1"
              sx={{ color: "rgba(255,255,255,0.9)", fontWeight: 500 }}
            >
              Desktop Paused
            </Typography>
            <Button
              variant="contained"
              color="primary"
              size="large"
              startIcon={
                isResuming ? <CircularProgress size={20} /> : <PlayArrow />
              }
              onClick={handleResume}
              disabled={isResuming}
            >
              {isResuming ? "Starting..." : "Start Desktop"}
            </Button>
          </Box>
        </Box>
      );
    }

    return (
      <Box
        sx={{
          height: screenshotHeight,
          width: "100%",
          overflow: "hidden",
        }}
      >
        <ScreenshotViewer
          sessionId={sessionId}
          autoRefresh={true}
          refreshInterval={1500}
          enableStreaming={false}
          showToolbar={false}
          showTimestamp={false}
          quality={5}
        />
      </Box>
    );
  }

  // Stream mode (floating window) - KEEP STREAM MOUNTED to prevent fullscreen exit on hiccups
  // Once we've been running, show overlays instead of unmounting the stream viewer
  // Use flex: 1 to fill available space (no fixed height)

  // Starting state before we've ever been running - show spinner
  if (isStarting && !hasEverBeenRunning) {
    return (
      <Box
        sx={{
          width: "100%",
          flex: 1,
          minHeight: 0,
          position: "relative",
          border: "1px solid",
          borderColor: "divider",
          borderRadius: 1,
          overflow: "hidden",
          backgroundColor: "#1a1a1a",
          display: "flex",
          flexDirection: "column",
          alignItems: "center",
          justifyContent: "center",
          gap: 2,
        }}
      >
        {desktopLimitError ? (
          <Typography variant="body2" sx={{ color: 'error.main', fontWeight: 500, textAlign: 'center', px: 2 }}>
            {desktopLimitError}
          </Typography>
        ) : (
          <>
            <CircularProgress size={32} sx={{ color: 'primary.main' }} />
            <Typography variant="body2" sx={{ color: 'rgba(255,255,255,0.7)', fontWeight: 500 }}>
              {startingTooLong ? "Desktop may have failed to start — click Stop to retry" : (statusMessage || "Starting Desktop...")}
            </Typography>
            <Button
              variant="outlined"
              size="small"
              onClick={handleStopFromStarting}
              disabled={isStopping}
              sx={{ color: 'rgba(255,255,255,0.6)', borderColor: 'rgba(255,255,255,0.3)', mt: 1 }}
            >
              {isStopping ? "Stopping..." : "Stop"}
            </Button>
          </>
        )}
      </Box>
    );
  }

  // Paused state before we've ever been running - show paused UI
  if (isPaused && !hasEverBeenRunning) {
    const screenshotUrl = `/api/v1/external-agents/${sessionId}/screenshot?t=${Date.now()}`;
    return (
      <Box
        sx={{
          width: "100%",
          flex: 1,
          minHeight: 0,
          position: "relative",
          border: "1px solid",
          borderColor: "divider",
          borderRadius: 1,
          overflow: "hidden",
          backgroundColor: "#1a1a1a",
        }}
      >
        <Box
          component="img"
          src={screenshotUrl}
          alt="Paused Desktop"
          sx={{
            width: "100%",
            height: "100%",
            objectFit: "contain",
            filter: "grayscale(0.5) brightness(0.7) blur(1px)",
            opacity: 0.6,
          }}
          onError={(e) => {
            e.currentTarget.style.display = "none";
          }}
        />
        <Box
          sx={{
            position: "absolute",
            top: 0,
            left: 0,
            right: 0,
            bottom: 0,
            display: "flex",
            flexDirection: "column",
            alignItems: "center",
            justifyContent: "center",
            gap: 2,
            backgroundColor: "rgba(0,0,0,0.3)",
          }}
        >
          <Typography
            variant="body1"
            sx={{ color: "rgba(255,255,255,0.9)", fontWeight: 500 }}
          >
            Desktop Paused
          </Typography>
          <Button
            variant="contained"
            color="primary"
            size="large"
            startIcon={
              isResuming ? <CircularProgress size={20} /> : <PlayArrow />
            }
            onClick={handleResume}
            disabled={isResuming}
          >
            {isResuming ? "Starting..." : "Start Desktop"}
          </Button>
        </Box>
      </Box>
    );
  }

  // Once running (or has ever been running) - ALWAYS keep DesktopStreamViewer mounted
  // Show overlays for state changes instead of unmounting (prevents fullscreen exit)
  const showReconnectingOverlay = !isRunning && hasEverBeenRunning;

  return (
    <Box
      sx={{
        display: "flex",
        flex: 1,
        minHeight: 0,
        width: "100%",
        position: "relative",
      }}
    >
      {/* Main desktop viewer */}
      <SandboxDropZone
        sessionId={sessionId}
        disabled={!isRunning}
        onFileUploaded={handleFileUploaded}
      >
        <Box
          sx={{
            flex: 1,
            minHeight: 0,
            width: "100%",
            overflow: "hidden",
            position: "relative",
          }}
        >
          <DesktopStreamViewer
            sessionId={sessionId}
            sandboxId={sandboxId}
            width={displayWidth}
            height={displayHeight}
            fps={displayFps}
            onError={(error) => {
              console.error("Stream viewer error:", error);
            }}
            onClientIdCalculated={onClientIdCalculated}
            // Suppress DesktopStreamViewer's overlay when we're showing our own reconnecting overlay
            // This prevents double spinners when container state changes
            suppressOverlay={showReconnectingOverlay}
          />

          {/* Reconnecting overlay - shown when state changes but stream stays mounted */}
          {showReconnectingOverlay && (
            <Box
              sx={{
                position: "absolute",
                top: 0,
                left: 0,
                right: 0,
                bottom: 0,
                display: "flex",
                flexDirection: "column",
                alignItems: "center",
                justifyContent: "center",
                gap: 2,
                backgroundColor: "rgba(0,0,0,0.7)",
                zIndex: 100,
              }}
            >
              <CircularProgress size={40} sx={{ color: "warning.main" }} />
              <Typography
                variant="body1"
                sx={{ color: "rgba(255,255,255,0.9)", fontWeight: 500 }}
              >
                {isPaused ? "Desktop Paused" : "Reconnecting..."}
              </Typography>
              {isPaused && (
                <Button
                  variant="contained"
                  color="primary"
                  size="large"
                  startIcon={
                    isResuming ? <CircularProgress size={20} /> : <PlayArrow />
                  }
                  onClick={handleResume}
                  disabled={isResuming}
                  sx={{ mt: 1 }}
                >
                  {isResuming ? "Starting..." : "Restart Desktop"}
                </Button>
              )}
            </Box>
          )}

          {/* Session panel toggle button - positioned on the right edge */}
          {showSessionPanel && (
            <Tooltip
              title={
                sessionPanelOpen ? "Hide session panel" : "Show session panel"
              }
            >
              <IconButton
                onClick={() => setSessionPanelOpen(!sessionPanelOpen)}
                sx={{
                  position: "absolute",
                  right: sessionPanelOpen ? SESSION_PANEL_WIDTH + 8 : 8,
                  top: 8,
                  zIndex: 200,
                  bgcolor: "background.paper",
                  border: "1px solid",
                  borderColor: "divider",
                  boxShadow: 2,
                  transition: "right 0.3s ease",
                  "&:hover": {
                    bgcolor: "action.hover",
                  },
                }}
              >
                {sessionPanelOpen ? <ChevronRightIcon /> : <ChatIcon />}
              </IconButton>
            </Tooltip>
          )}
        </Box>
      </SandboxDropZone>

      {/* Collapsible session panel */}
      {showSessionPanel && (
        <Collapse
          in={sessionPanelOpen}
          orientation="horizontal"
          sx={{
            flexShrink: 0,
            borderLeft: sessionPanelOpen ? "1px solid" : "none",
            borderColor: "divider",
          }}
        >
          <Box
            sx={{
              width: SESSION_PANEL_WIDTH,
              height: "100%",
              display: "flex",
              flexDirection: "column",
              bgcolor: "background.paper",
            }}
          >
            {/* Session messages */}
            <Box
              sx={{
                flex: 1,
                minHeight: 0,
                overflow: "hidden",
                display: "flex",
                flexDirection: "column",
              }}
            >
              <EmbeddedSessionView sessionId={sessionId} />
            </Box>

            {/* Prompt input */}
            <Box
              sx={{ p: 2, borderTop: 1, borderColor: "divider", flexShrink: 0 }}
            >
              <RobustPromptInput
                sessionId={sessionId}
                specTaskId={specTaskId}
                projectId={projectId}
                apiClient={apiClient}
                onSend={handleSendMessage}
                placeholder="Send message to agent..."
                appendText={uploadedFilePath}
                onImagePaste={handleImagePaste}
              />
            </Box>
          </Box>
        </Collapse>
      )}
    </Box>
  );
};

export default ExternalAgentDesktopViewer;
