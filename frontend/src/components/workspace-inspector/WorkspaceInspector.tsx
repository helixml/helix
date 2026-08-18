import React, { FC, useCallback, useEffect, useRef, useState } from "react";
import { Box, Divider, IconButton, Menu, MenuItem, Select, Tab, Tabs, Tooltip, Typography } from "@mui/material";
import { Code2, X } from "lucide-react";
import useLightTheme from "../../hooks/useLightTheme";
import useRouter from "../../hooks/useRouter";
import useSnackbar from "../../hooks/useSnackbar";
import { copyTextToClipboard, workspaceFilePath } from "./clipboard";
import WorkspaceDiffSurface from "./WorkspaceDiffSurface";
import WorkspaceFileSurface from "./WorkspaceFileSurface";
import { closeWorkspaceTabs, type WorkspaceTabCloseAction } from "./workspaceTabs";
import TaskSessionPlaceholder from "../tasks/TaskSessionPlaceholder";
import {
  isDesktopUnavailableError,
  useDesktopReachability,
  useWorkspaces,
} from "./workspaceReviewService";
import type { WorkspaceReviewComment } from "./workspaceReviewComments";

const NO_COMMENTS: readonly WorkspaceReviewComment[] = [];
const NOOP_COMMENT = () => undefined;

interface WorkspaceInspectorProps {
  sessionId: string | undefined;
  baseBranch?: string;
  pollInterval?: number;
  primarySurface?: "changes" | "files";
  onPrimarySurfaceChange?: (surface: "changes" | "files") => void;
  onStartDesktop?: () => void;
  isDesktopStarting?: boolean;
  /**
   * False when the task's sandbox is known to be stopped. The inspector then
   * issues no workspace requests at all rather than discovering the same 503
   * on every query.
   */
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

type Surface = "changes" | "files" | string;

interface TabContextMenu {
  left: number;
  path: string;
  top: number;
}

const WorkspaceInspector: FC<WorkspaceInspectorProps> = ({
  sessionId,
  baseBranch = "main",
  pollInterval = 3_000,
  primarySurface = "changes",
  onPrimarySurfaceChange,
  onStartDesktop,
  isDesktopStarting,
  desktopRunning = true,
  connectSubscriptionLabel,
  onConnectSubscription,
  desktopUnavailableDetail,
  desktopUnavailableTitle,
  desktopUnavailableDescription,
  comments = NO_COMMENTS,
  onUpsertComment,
  onRemoveComment,
}) => {
  const lightTheme = useLightTheme();
  const router = useRouter();
  const snackbar = useSnackbar();
  const onPrimarySurfaceChangeRef = useRef(onPrimarySurfaceChange);
  onPrimarySurfaceChangeRef.current = onPrimarySurfaceChange;
  const workspacesQuery = useWorkspaces(sessionId, desktopRunning);
  const reachability = useDesktopReachability({
    unavailable: desktopRunning && isDesktopUnavailableError(workspacesQuery.error),
    settled: !desktopRunning || workspacesQuery.isSuccess,
  });
  const [workspace, setWorkspace] = useState<string>();
  const [openFiles, setOpenFiles] = useState<string[]>(() =>
    router.params.preview ? [router.params.preview] : [],
  );
  const [surface, setSurface] = useState<Surface>(() =>
    router.params.preview || primarySurface,
  );
  const [treeRevealPath, setTreeRevealPath] = useState<string | null>(() => router.params.preview || null);
  const [tabContextMenu, setTabContextMenu] = useState<TabContextMenu | null>(null);

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
    setTreeRevealPath(requestedFile);
  }, [primarySurface, router.params.preview]);

  useEffect(() => {
    if (workspace || !workspacesQuery.data?.workspaces?.length) return;
    const primary = workspacesQuery.data.workspaces.find((candidate) => candidate.is_primary);
    setWorkspace(primary?.name || workspacesQuery.data.workspaces[0].name);
  }, [workspace, workspacesQuery.data?.workspaces]);

  const openFile = useCallback((path: string, revealInTree: boolean) => {
    setOpenFiles((current) => current.includes(path) ? current : [...current, path]);
    setSurface(path);
    setTreeRevealPath(revealInTree ? path : null);
    onPrimarySurfaceChangeRef.current?.("files");
    router.mergeParams({ view: "files", preview: path });
  }, [router]);
  const openFileFromTree = useCallback((path: string) => openFile(path, false), [openFile]);
  const openFileFromDiff = useCallback((path: string) => openFile(path, true), [openFile]);

  const closeTabs = useCallback((path: string, action: WorkspaceTabCloseAction) => {
    const activeFile = surface !== "changes" && surface !== "files" ? surface : null;
    const result = closeWorkspaceTabs(openFiles, activeFile, path, action);
    setOpenFiles(result.openFiles);
    setTabContextMenu(null);
    if (result.activeFile) {
      setSurface(result.activeFile);
      setTreeRevealPath(result.activeFile);
      router.mergeParams({ view: "files", preview: result.activeFile });
      return;
    }
    setSurface(primarySurface);
    router.removeParams(["preview"]);
  }, [openFiles, primarySurface, router, surface]);

  const copyTabPath = async () => {
    const path = tabContextMenu?.path;
    const workspacePath = workspacesQuery.data?.workspaces?.find(
      (candidate) => candidate.name === workspace,
    )?.path;
    if (!path || !workspacePath) return;
    try {
      await copyTextToClipboard(workspaceFilePath(workspacePath, path));
      snackbar.success("Path copied to clipboard");
    } catch {
      snackbar.error("Could not copy path");
    } finally {
      setTabContextMenu(null);
    }
  };

  if (!sessionId) {
    return (
      <Box sx={{ height: "100%", display: "grid", placeItems: "center" }}>
        <Typography variant="body2" color="text.secondary">No active session.</Typography>
      </Box>
    );
  }

  // A task whose session is running still answers 503 for the first seconds
  // after launch, while its container registers the bridge these endpoints
  // are dialled through. Saying "sandbox is stopped" there is simply wrong —
  // and offering a start button invites the user to restart a healthy task.
  if (reachability === "connecting") {
    return (
      <Box sx={{ height: "100%", display: "flex", flexDirection: "column" }}>
        <TaskSessionPlaceholder
          tone="connecting"
          title="Connecting to the sandbox"
          description={
            primarySurface === "files"
              ? "This task's sandbox is still starting. Its workspace files will appear as soon as it connects."
              : "This task's sandbox is still starting. Its workspace changes will appear as soon as it connects."
          }
        />
      </Box>
    );
  }

  // Every workspace endpoint answers 503 while the sandbox is stopped, which
  // is the normal resting state of a finished or paused task. Show the same
  // start-desktop placeholder the Desktop tab uses rather than reporting a
  // failure to load changes.
  if (!desktopRunning || reachability === "unreachable") {
    return (
      <Box sx={{ height: "100%", display: "flex", flexDirection: "column" }}>
        <TaskSessionPlaceholder
          tone="paused"
          title={desktopUnavailableTitle || "Desktop not running"}
          description={
            desktopUnavailableDescription ||
            (primarySurface === "files"
              ? "This task's sandbox is stopped. Start the desktop to browse its workspace files."
              : "This task's sandbox is stopped. Start the desktop to load its workspace changes.")
          }
          detail={desktopUnavailableDetail}
          primaryAction={
            connectSubscriptionLabel && onConnectSubscription
              ? { label: connectSubscriptionLabel, onClick: onConnectSubscription }
              : undefined
          }
          onStart={onStartDesktop}
          starting={isDesktopStarting}
        />
      </Box>
    );
  }

  const activeSurface = primarySurface === "changes" ? "changes" : surface;
  const selectedFile = activeSurface !== "changes" && activeSurface !== "files" ? activeSurface : null;
  const workspacePath = workspacesQuery.data?.workspaces?.find(
    (candidate) => candidate.name === workspace,
  )?.path;

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
                setTreeRevealPath(value);
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
                  onContextMenu={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    setTabContextMenu({ left: event.clientX, path, top: event.clientY });
                  }}
                  icon={<Code2 size={13} />}
                  iconPosition="start"
                  label={
                    <Box sx={{ display: "flex", alignItems: "center", gap: 0.5, minWidth: 0 }}>
                      <Tooltip title={path}><Box component="span" sx={{ maxWidth: 150, overflow: "hidden", textOverflow: "ellipsis" }}>{path.split("/").at(-1)}</Box></Tooltip>
                      <IconButton component="span" size="small" aria-label={`Close ${path}`} onClick={(event) => { event.stopPropagation(); closeTabs(path, "close"); }} sx={{ p: 0.15 }}>
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
                setTreeRevealPath(null);
                setTabContextMenu(null);
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
      <Menu
        open={tabContextMenu !== null}
        onClose={() => setTabContextMenu(null)}
        anchorReference="anchorPosition"
        anchorPosition={tabContextMenu ? { left: tabContextMenu.left, top: tabContextMenu.top } : undefined}
        MenuListProps={{
          "aria-label": tabContextMenu ? `Tab options for ${tabContextMenu.path}` : "Tab options",
          dense: true,
        }}
      >
        <MenuItem onClick={copyTabPath} disabled={!workspacePath}>Copy full path</MenuItem>
        <Divider />
        <MenuItem onClick={() => tabContextMenu && closeTabs(tabContextMenu.path, "close")}>Close</MenuItem>
        <MenuItem
          onClick={() => tabContextMenu && closeTabs(tabContextMenu.path, "close_others")}
          disabled={openFiles.length <= 1}
        >
          Close others
        </MenuItem>
        <MenuItem
          onClick={() => tabContextMenu && closeTabs(tabContextMenu.path, "close_right")}
          disabled={!tabContextMenu || openFiles.indexOf(tabContextMenu.path) === openFiles.length - 1}
        >
          Close to the right
        </MenuItem>
        <MenuItem onClick={() => tabContextMenu && closeTabs(tabContextMenu.path, "close_all")}>Close all</MenuItem>
      </Menu>
      <Box sx={{ flex: 1, minHeight: 0 }}>
        {activeSurface === "changes" ? (
          <WorkspaceDiffSurface
            sessionId={sessionId}
            workspace={workspace}
            workspacePath={workspacePath}
            baseBranch={baseBranch}
            pollInterval={pollInterval}
            interactionId={router.params.interaction}
            selectedFile={router.params.file}
            onOpenFile={openFileFromDiff}
            onExitTurn={() => router.removeParams(["interaction", "file"])}
            onWorkspaceResolved={setWorkspace}
            onStartDesktop={onStartDesktop}
            isDesktopStarting={isDesktopStarting}
            desktopUnavailableTitle={desktopUnavailableTitle}
            desktopUnavailableDescription={desktopUnavailableDescription}
          />
        ) : (
          <WorkspaceFileSurface
            sessionId={sessionId}
            workspace={workspace}
            workspacePath={workspacePath}
            path={selectedFile}
            revealPath={treeRevealPath}
            onOpenFile={openFileFromTree}
            comments={comments}
            onUpsertComment={onUpsertComment || NOOP_COMMENT}
            onRemoveComment={onRemoveComment || NOOP_COMMENT}
          />
        )}
      </Box>
    </Box>
  );
};

export default WorkspaceInspector;
