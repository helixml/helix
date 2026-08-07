import React, { FC, useCallback, useEffect, useRef, useState } from "react";
import { Box, IconButton, MenuItem, Select, Tab, Tabs, Tooltip, Typography } from "@mui/material";
import { Code2, X } from "lucide-react";
import useLightTheme from "../../hooks/useLightTheme";
import useRouter from "../../hooks/useRouter";
import WorkspaceDiffSurface from "./WorkspaceDiffSurface";
import WorkspaceFileSurface from "./WorkspaceFileSurface";
import { useWorkspaces } from "./workspaceReviewService";

interface WorkspaceInspectorProps {
  sessionId: string | undefined;
  baseBranch?: string;
  pollInterval?: number;
  primarySurface?: "changes" | "files";
  onPrimarySurfaceChange?: (surface: "changes" | "files") => void;
}

type Surface = "changes" | "files" | string;

const WorkspaceInspector: FC<WorkspaceInspectorProps> = ({
  sessionId,
  baseBranch = "main",
  pollInterval = 3_000,
  primarySurface = "changes",
  onPrimarySurfaceChange,
}) => {
  const lightTheme = useLightTheme();
  const router = useRouter();
  const onPrimarySurfaceChangeRef = useRef(onPrimarySurfaceChange);
  onPrimarySurfaceChangeRef.current = onPrimarySurfaceChange;
  const workspacesQuery = useWorkspaces(sessionId);
  const [workspace, setWorkspace] = useState<string>();
  const [openFiles, setOpenFiles] = useState<string[]>(() =>
    router.params.preview ? [router.params.preview] : [],
  );
  const [surface, setSurface] = useState<Surface>(() =>
    router.params.preview || primarySurface,
  );

  useEffect(() => {
    if (primarySurface === "changes") {
      setSurface("changes");
      return;
    }
    const requestedFile = router.params.preview;
    if (!requestedFile) {
      setSurface("files");
      return;
    }
    setOpenFiles((current) => current.includes(requestedFile) ? current : [...current, requestedFile]);
    setSurface(requestedFile);
  }, [primarySurface, router.params.preview]);

  useEffect(() => {
    if (workspace || !workspacesQuery.data?.workspaces?.length) return;
    const primary = workspacesQuery.data.workspaces.find((candidate) => candidate.is_primary);
    setWorkspace(primary?.name || workspacesQuery.data.workspaces[0].name);
  }, [workspace, workspacesQuery.data?.workspaces]);

  const openFile = useCallback((path: string) => {
    setOpenFiles((current) => current.includes(path) ? current : [...current, path]);
    setSurface(path);
    onPrimarySurfaceChangeRef.current?.("files");
    router.mergeParams({ view: "files", preview: path });
  }, [router]);

  const closeFile = useCallback((path: string) => {
    setOpenFiles((current) => current.filter((candidate) => candidate !== path));
    if (surface === path) {
      setSurface(primarySurface);
      router.removeParams(["preview"]);
    }
  }, [primarySurface, router, surface]);

  if (!sessionId) {
    return (
      <Box sx={{ height: "100%", display: "grid", placeItems: "center" }}>
        <Typography variant="body2" color="text.secondary">No active session.</Typography>
      </Box>
    );
  }

  const activeSurface = primarySurface === "changes" ? "changes" : surface;
  const selectedFile = activeSurface !== "changes" && activeSurface !== "files" ? activeSurface : null;

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
      {primarySurface === "files" && (openFiles.length > 0 || (workspacesQuery.data?.workspaces?.length || 0) > 1) && (
        <Box sx={{ display: "flex", alignItems: "center", minHeight: 36, borderBottom: "1px solid", borderColor: "divider" }}>
          {openFiles.length > 0 && (
            <Tabs
              value={selectedFile || false}
              onChange={(_, value) => {
                setSurface(value);
                router.mergeParams({ view: "files", preview: value });
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
          )}
          {(workspacesQuery.data?.workspaces?.length || 0) > 1 && (
            <Select
              size="small"
              value={workspace || ""}
              onChange={(event) => {
                setWorkspace(event.target.value);
                setOpenFiles([]);
                setSurface(primarySurface);
                router.removeParams(["preview", "file"]);
              }}
              aria-label="Workspace"
              sx={{ height: 26, ml: "auto", mr: 1, minWidth: 120, fontSize: 11 }}
            >
              {workspacesQuery.data?.workspaces?.map((candidate) => (
                <MenuItem key={candidate.name} value={candidate.name} sx={{ fontSize: 12 }}>
                  {candidate.name}{candidate.is_primary ? " · primary" : ""}
                </MenuItem>
              ))}
            </Select>
          )}
        </Box>
      )}
      <Box sx={{ flex: 1, minHeight: 0 }}>
        {activeSurface === "changes" ? (
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
