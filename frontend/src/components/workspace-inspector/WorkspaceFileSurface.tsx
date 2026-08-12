import React, { FC } from "react";
import { Alert, Box, CircularProgress, Typography } from "@mui/material";
import { useWorkspaceFile } from "./workspaceReviewService";
import WorkspaceFileTree from "./WorkspaceFileTree";
import WorkspaceEditableFile from "./WorkspaceEditableFile";
import type { WorkspaceReviewComment } from "./workspaceReviewComments";

interface WorkspaceFileSurfaceProps {
  sessionId: string;
  workspace?: string;
  workspacePath?: string;
  path: string | null;
  revealPath: string | null;
  onOpenFile: (path: string) => void;
  comments: readonly WorkspaceReviewComment[];
  onUpsertComment: (comment: WorkspaceReviewComment) => void;
  onRemoveComment: (commentId: string) => void;
}

const WorkspaceFileSurface: FC<WorkspaceFileSurfaceProps> = ({
  sessionId,
  workspace,
  workspacePath,
  path,
  revealPath,
  onOpenFile,
  comments,
  onUpsertComment,
  onRemoveComment,
}) => {
  const fileQuery = useWorkspaceFile(sessionId, workspace, path);

  return (
    <Box sx={{ display: "flex", minHeight: 0, height: "100%" }}>
      <Box
        sx={{
          flex: 1,
          minWidth: 0,
          minHeight: 0,
          display: "flex",
          flexDirection: "column",
          borderRight: "1px solid",
          borderColor: "divider",
        }}
      >
        {/*
          The server caps reads at 1 MiB. Rendering the prefix without saying so
          would present a partial file as the whole one.
        */}
        {fileQuery.data?.truncated && (
          <Alert severity="warning" square sx={{ py: 0, fontSize: 12 }}>
            Showing the first {fileQuery.data.byte_length?.toLocaleString()}{" "}
            bytes — this file is larger than the preview limit.
          </Alert>
        )}
        {!path ? (
          <Box sx={{ flex: 1, display: "grid", placeItems: "center" }}>
            <Typography variant="body2" color="text.secondary">
              Choose a file from the browser.
            </Typography>
          </Box>
        ) : fileQuery.isLoading ? (
          <Box sx={{ flex: 1, display: "grid", placeItems: "center" }}>
            <CircularProgress size={22} />
          </Box>
        ) : fileQuery.isError ? (
          <Box sx={{ p: 2 }}>
            <Alert severity="error">Could not read {path}.</Alert>
          </Box>
        ) : fileQuery.data?.binary ? (
          <Box
            sx={{
              flex: 1,
              display: "grid",
              placeItems: "center",
              textAlign: "center",
              p: 3,
            }}
          >
            <Box>
              <Typography variant="body2">Binary file</Typography>
              <Typography variant="caption" color="text.secondary">
                {fileQuery.data.byte_length?.toLocaleString()} bytes
              </Typography>
            </Box>
          </Box>
        ) : fileQuery.data?.truncated ? (
          <Box sx={{ flex: 1, minHeight: 0, overflow: "auto", p: 2 }}>
            <Typography component="pre" sx={{ m: 0, fontFamily: "monospace", fontSize: 12, whiteSpace: "pre-wrap" }}>
              {fileQuery.data.contents}
            </Typography>
          </Box>
        ) : fileQuery.data?.contents !== undefined && fileQuery.data.content_hash ? (
          <Box sx={{ flex: 1, minHeight: 0 }}>
            <WorkspaceEditableFile
              key={`${workspace || "primary"}:${path}`}
              sessionId={sessionId}
              workspace={workspace}
              path={path}
              initialContents={fileQuery.data.contents}
              initialContentHash={fileQuery.data.content_hash}
              comments={comments}
              onUpsertComment={onUpsertComment}
              onRemoveComment={onRemoveComment}
              onReload={async () => (await fileQuery.refetch()).data}
            />
          </Box>
        ) : null}
      </Box>
      <Box sx={{ width: "42%", minWidth: 260, maxWidth: 440, minHeight: 0 }}>
        <WorkspaceFileTree
          sessionId={sessionId}
          workspace={workspace}
          workspacePath={workspacePath}
          selectedPath={path}
          revealPath={revealPath}
          onOpenFile={onOpenFile}
        />
      </Box>
    </Box>
  );
};

export default WorkspaceFileSurface;
