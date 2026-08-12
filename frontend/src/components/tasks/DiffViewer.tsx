import React, { lazy, Suspense } from "react";
import { Box, CircularProgress } from "@mui/material";
import type { WorkspaceReviewComment } from "../workspace-inspector/workspaceReviewComments";

const WorkspaceInspector = lazy(
  () => import("../workspace-inspector/WorkspaceInspector"),
);

interface DiffViewerProps {
  sessionId: string | undefined;
  baseBranch?: string;
  pollInterval?: number;
  primarySurface?: "changes" | "files";
  onPrimarySurfaceChange?: (surface: "changes" | "files") => void;
  onStartDesktop?: () => void;
  isDesktopStarting?: boolean;
  desktopRunning?: boolean;
  connectSubscriptionLabel?: string;
  onConnectSubscription?: () => void;
  desktopUnavailableDetail?: string;
  desktopUnavailableTitle?: string;
  desktopUnavailableDescription?: string;
  comments?: readonly WorkspaceReviewComment[];
  onUpsertComment?: (comment: WorkspaceReviewComment) => void;
  onRemoveComment?: (commentId: string) => void;
}

export default function DiffViewer(props: DiffViewerProps) {
  return (
    <Suspense
      fallback={
        <Box sx={{ height: "100%", display: "grid", placeItems: "center" }}>
          <CircularProgress size={22} />
        </Box>
      }
    >
      <WorkspaceInspector {...props} />
    </Suspense>
  );
}
