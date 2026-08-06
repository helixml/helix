import React, { FC, useCallback, useEffect, useState } from "react";
import { Box, IconButton, MenuItem, Select, Tab, Tabs, Tooltip, Typography } from "@mui/material";
import { Code2, Files, GitCompare, X } from "lucide-react";
import useLightTheme from "../../hooks/useLightTheme";
import useRouter from "../../hooks/useRouter";
import WorkspaceDiffSurface from "./WorkspaceDiffSurface";
import WorkspaceFileSurface from "./WorkspaceFileSurface";
import { useWorkspaces } from "./workspaceReviewService";

interface WorkspaceInspectorProps {
  sessionId: string | undefined;
  baseBranch?: string;
  pollInterval?: number;
}

type Surface = "changes" | "files" | string;

const WorkspaceInspector: FC<WorkspaceInspectorProps> = ({
  sessionId,
  baseBranch = "main",
  pollInterval = 3_000,
}) => {
  const lightTheme = useLightTheme();
  const router = useRouter();
  const workspacesQuery = useWorkspaces(sessionId);
  const [workspace, setWorkspace] = useState<string>();
  const [openFiles, setOpenFiles] = useState<string[]>(() =>
    router.params.preview ? [router.params.preview] : [],
  );
  const [surface, setSurface] = useState<Surface>(() =>
    router.params.preview || "changes",
  );

  useEffect(() => {
    const requestedFile = router.params.preview;
    if (!requestedFile) return;
    setOpenFiles((current) => current.includes(requestedFile) ? current : [...current, requestedFile]);
    setSurface(requestedFile);
  }, [router.params.preview]);

  useEffect(() => {
    if (workspace || !workspacesQuery.data?.workspaces?.length) return;
    const primary = workspacesQuery.data.workspaces.find((candidate) => candidate.is_primary);
    setWorkspace(primary?.name || workspacesQuery.data.workspaces[0].name);
  }, [workspace, workspacesQuery.data?.workspaces]);

  const openFile = useCallback((path: string) => {
    setOpenFiles((current) => current.includes(path) ? current : [...current, path]);
    setSurface(path);
    router.mergeParams({ preview: path });
  }, [router]);

  const closeFile = useCallback((path: string) => {
    setOpenFiles((current) => current.filter((candidate) => candidate !== path));
    if (surface === path) {
      setSurface("changes");
      router.removeParams(["preview"]);
    }
  }, [router, surface]);

  if (!sessionId) {
    return (
      <Box sx={{ height: "100%", display: "grid", placeItems: "center" }}>
        <Typography variant="body2" color="text.secondary">No active session.</Typography>
      </Box>
    );
  }

  const selectedFile = surface !== "changes" && surface !== "files" ? surface : null;

  return (
    <Box
      sx={{
        "--workspace-bg": lightTheme.backgroundColor,
        "--workspace-fg": lightTheme.textColor,
        "--workspace-muted": lightTheme.textColorFaded,
        "--workspace-border": lightTheme.border,
        height: "100%",
        minHeight: 0,
        display: "flex",
        flexDirection: "column",
        bgcolor: lightTheme.backgroundColor,
        color: lightTheme.textColor,
      }}
    >
      <Box sx={{ display: "flex", alignItems: "center", minHeight: 36, borderBottom: "1px solid", borderColor: "divider" }}>
        <Tabs
          value={surface}
          onChange={(_, value) => {
            setSurface(value);
            if (value === "changes" || value === "files") router.removeParams(["preview"]);
            else router.mergeParams({ preview: value });
          }}
          variant="scrollable"
          scrollButtons="auto"
          sx={{
            minHeight: 36,
            flex: 1,
            minWidth: 0,
            "& .MuiTab-root": { minHeight: 36, minWidth: 0, px: 1.25, py: 0, fontSize: 11, textTransform: "none" },
          }}
        >
          <Tab icon={<GitCompare size={14} />} iconPosition="start" value="changes" label="Changes" />
          <Tab icon={<Files size={14} />} iconPosition="start" value="files" label="Files" />
          {openFiles.map((path) => (
            <Tab
              key={path}
              value={path}
              icon={<Code2 size={13} />}
              iconPosition="start"
              label={
                <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, minWidth: 0 }}>
                  <Tooltip title={path}><Box component="span" sx={{ maxWidth: 150, overflow: "hidden", textOverflow: "ellipsis" }}>{path.split("/").at(-1)}</Box></Tooltip>
                  <IconButton component="span" size="small" aria-label={`Close ${path}`} onClick={(event) => { event.stopPropagation(); closeFile(path); }} sx={{ p: 0.15 }}>
                    <X size={11} />
                  </IconButton>
                </Box>
              }
            />
          ))}
        </Tabs>
        {(workspacesQuery.data?.workspaces?.length || 0) > 1 && (
          <Select
            size="small"
            value={workspace || ""}
            onChange={(event) => {
              setWorkspace(event.target.value);
              setOpenFiles([]);
              setSurface("changes");
              router.removeParams(["preview", "file"]);
            }}
            aria-label="Workspace"
            sx={{ height: 26, mr: 1, minWidth: 120, fontSize: 11 }}
          >
            {workspacesQuery.data?.workspaces?.map((candidate) => (
              <MenuItem key={candidate.name} value={candidate.name} sx={{ fontSize: 12 }}>
                {candidate.name}{candidate.is_primary ? " · primary" : ""}
              </MenuItem>
            ))}
          </Select>
        )}
      </Box>
      <Box sx={{ flex: 1, minHeight: 0 }}>
        {surface === "changes" ? (
          <WorkspaceDiffSurface
            sessionId={sessionId}
            workspace={workspace}
            baseBranch={baseBranch}
            pollInterval={pollInterval}
            interactionId={router.params.interaction}
            selectedFile={router.params.file}
            onOpenFile={openFile}
            onExitTurn={() => router.removeParams(["interaction", "file"])}
            onWorkspaceResolved={setWorkspace}
          />
        ) : (
          <WorkspaceFileSurface
            sessionId={sessionId}
            workspace={workspace}
            path={selectedFile}
            onOpenFile={openFile}
          />
        )}
      </Box>
    </Box>
  );
};

export default WorkspaceInspector;
