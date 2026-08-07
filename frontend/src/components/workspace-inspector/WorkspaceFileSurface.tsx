import React, { FC, useMemo } from "react";
import { CodeView } from "@pierre/diffs/react";
import type { CodeViewFileItem } from "@pierre/diffs";
import { Alert, Box, CircularProgress, Typography } from "@mui/material";
import useLightTheme from "../../hooks/useLightTheme";
import { DIFF_UNSAFE_CSS, PIERRE_THEMES } from "./pierreStyles";
import { useWorkspaceFile } from "./workspaceReviewService";
import WorkspaceFileTree from "./WorkspaceFileTree";

interface WorkspaceFileSurfaceProps {
  sessionId: string;
  workspace?: string;
  workspacePath?: string;
  path: string | null;
  revealPath: string | null;
  onOpenFile: (path: string) => void;
}

const WorkspaceFileSurface: FC<WorkspaceFileSurfaceProps> = ({
  sessionId,
  workspace,
  workspacePath,
  path,
  revealPath,
  onOpenFile,
}) => {
  const lightTheme = useLightTheme();
  const fileQuery = useWorkspaceFile(sessionId, workspace, path);
  const item = useMemo<CodeViewFileItem[] | null>(() => {
    if (
      !path ||
      !fileQuery.data ||
      fileQuery.data.binary ||
      fileQuery.data.contents === undefined
    )
      return null;
    return [
      {
        id: `${path}:${fileQuery.data.content_hash || fileQuery.data.byte_length || 0}`,
        type: "file",
        file: {
          name: path,
          contents: fileQuery.data.contents,
          cacheKey: fileQuery.data.content_hash,
        },
      },
    ];
  }, [fileQuery.data, path]);

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
        ) : item ? (
          <Box sx={{ flex: 1, minHeight: 0 }}>
            <CodeView
              items={item}
              style={{ height: "100%", minHeight: 0, overflow: "auto" }}
              options={{
                theme: PIERRE_THEMES,
                themeType: lightTheme.isLight ? "light" : "dark",
                overflow: "scroll",
                stickyHeaders: true,
                tokenizeMaxLineLength: 1_000,
                unsafeCSS: DIFF_UNSAFE_CSS,
                itemMetrics: { diffHeaderHeight: 32 },
                layout: { gap: 0, paddingTop: 0, paddingBottom: 0 },
              }}
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
