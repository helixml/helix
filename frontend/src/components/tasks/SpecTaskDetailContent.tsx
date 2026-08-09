import React, {
  FC,
  useState,
  useEffect,
  useCallback,
  useMemo,
  useRef,
} from "react";
import {
  Alert,
  Box,
  Typography,
  Chip,
  Divider,
  IconButton,
  TextField,
  Button,
  Tooltip,
  Select,
  CircularProgress,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Dialog,
  DialogTitle,
  DialogContent,
  DialogContentText,
  DialogActions,
  ToggleButton,
  ToggleButtonGroup,
  Switch,
  Autocomplete,
  ClickAwayListener,
} from "@mui/material";
import type { Theme } from "@mui/material";
import CloseIcon from "@mui/icons-material/Close";
import PlayArrow from "@mui/icons-material/PlayArrow";
import RestartAltIcon from "@mui/icons-material/RestartAlt";
import StopIcon from "@mui/icons-material/Stop";
import LaunchIcon from "@mui/icons-material/Launch";
import LinkIcon from "@mui/icons-material/Link";
import ArchiveIcon from "@mui/icons-material/Archive";
import AccountTree from "@mui/icons-material/AccountTree";
import UndoIcon from "@mui/icons-material/Undo";
import { TypesSpecTaskStatus } from "../../api/api";
import ExternalAgentDesktopViewer, {
  useSandboxState,
} from "../external-agent/ExternalAgentDesktopViewer";
import DiffViewer from "./DiffViewer";
import TaskSessionPlaceholder from "./TaskSessionPlaceholder";
import {
  subscriptionRequirementFromTask,
  subscriptionRequirementMessage,
} from "./taskLaunchFailure";
import { getCSRFToken } from "../../utils/csrf";
import SpecTaskActionButtons from "./SpecTaskActionButtons";
import TaskAttachmentsPanel from "./TaskAttachmentsPanel";
import useSnackbar from "../../hooks/useSnackbar";
import useAccount from "../../hooks/useAccount";
import useApi from "../../hooks/useApi";
import useRouter from "../../hooks/useRouter";
import { useOAuthFlow } from "../../hooks/useOAuthFlow";
import { useListOAuthProviders } from "../../services/oauthProvidersService";
import { findOAuthProviderForType, vcsScopesForProvider } from "../../utils/oauthProviders";
import { getBrowserLocale } from "../../hooks/useBrowserLocale";
import useApps from "../../hooks/useApps";
import { deriveDisplaySettings } from "../../services/externalAgentDisplay";
import { useStreaming } from "../../contexts/streaming";
import { useQueryClient, useMutation } from "@tanstack/react-query";
import {
  useGetSession,
  GET_SESSION_QUERY_KEY,
} from "../../services/sessionService";
import { selectCodingAgents } from "../../utils/apps";
import {
  useUpdateSpecTask,
  useSpecTask,
  useCloneGroups,
  useZedThreads,
  useSpecTasks,
  useProjectLabels,
  useAddLabel,
  useRemoveLabel,
} from "../../services/specTaskService";
import {
  useGetProject,
  useGetProjectRepositories,
} from "../../services/projectService";
import { useMoveToBacklog } from "../../services/specTaskWorkflowService";
import { getUserById } from "../../services/userService";
import CloneTaskDialog from "../specTask/CloneTaskDialog";
import SpecTaskShareDialog from "./SpecTaskShareDialog";
import AgentDropdown from "../agent/AgentDropdown";
import AssigneeSelector from "./AssigneeSelector";
import OrganizationUserAvatar, { resolveOrganizationUser } from "../widgets/OrganizationUserAvatar";
import CloneGroupProgressFull from "../specTask/CloneGroupProgress";
import ArchiveConfirmDialog from "./ArchiveConfirmDialog";
import { optimisticallyMarkSessionStarting } from "../../utils/optimisticSessionStarting";
import AgentChat from "../session/AgentChat";
import { getChatColors } from "../session/chatStyles";
import SwitchAgentControl from "../session/SwitchAgentControl";
import SharePreviewSection from "./SharePreviewSection";
import SandboxBrowser from "./SandboxBrowser";
import SpecTaskLaunchWindow, {
  getSpecTaskLaunchPhase,
} from "./SpecTaskLaunchWindow";
import TaskChatMetadata from "./TaskChatMetadata";
import {
  Panel,
  Group as PanelGroup,
  Separator as PanelResizeHandle,
} from "react-resizable-panels";
import type { PanelImperativeHandle } from "react-resizable-panels";
import useIsBigScreen from "../../hooks/useIsBigScreen";
import useLightTheme from "../../hooks/useLightTheme";
import { useClaudeSubscriptions } from "../account/ClaudeSubscriptionConnect";
import ClaudeSubscriptionConnect from "../account/ClaudeSubscriptionConnect";
import { getTokenExpiryStatus } from "../account/claudeSubscriptionUtils";
import {
  CloudUpload as CloudUploadLucide,
  FileText,
  Files,
  SlidersHorizontal,
  GitCompare,
  Globe2,
  Lock as LockLucide,
  LockOpen as LockOpenLucide,
  MonitorPlay,
  MessageSquare,
  PanelBottom,
  PanelLeft,
  PanelRight,
  Play as PlayLucide,
  RotateCw,
  Square,
  EllipsisVertical,
  Wand2,
  Share,
  X,
} from "lucide-react";

import { getAutoOpenedSpecTasks, addAutoOpenedSpecTask } from "../../lib/specTaskAutoOpen";
import { loadPanelLayout, savePanelLayout } from "../../lib/panelLayoutStorage";
import SpecTaskTerminalDrawer from "./SpecTaskTerminalDrawer";
import {
  isSpecTaskTerminalToggleShortcut,
  loadSpecTaskTerminalDrawerState,
  saveSpecTaskTerminalDrawerState,
} from "./specTaskTerminalDrawerState";

const SPEC_TASK_CHAT_PANEL_IDS = ["spec-task-chat", "spec-task-content"] as const;
const SPEC_TASK_CHAT_LAYOUT_KEY = "helix.specTaskChat.layout";
const taskToolbarIconButtonSx = {
  width: 30,
  height: 30,
  minWidth: 30,
  minHeight: 30,
  p: 0.75,
  flexShrink: 0,
  color: "text.secondary",
  "& svg": {
    width: 18,
    height: 18,
  },
  "&:hover": {
    color: "text.primary",
    backgroundColor: "action.hover",
  },
  '&[aria-pressed="true"]': {
    backgroundColor: "action.selected",
  },
} as const;

const taskDetailsSectionSx = {
  border: "1px solid",
  borderColor: "divider",
  borderRadius: 2,
  backgroundColor: "background.paper",
  p: 2,
} as const;

const taskActionButtonSx = {
  fontSize: "0.75rem",
  textTransform: "none",
} as const;

const taskDetailsTextSx = {
  color: (theme: Theme) => getChatColors(theme).assistantForeground,
  fontSize: "0.875rem",
  lineHeight: 1.625,
} as const;

const taskDetailsTextFieldSx = {
  "& .MuiInputBase-input": taskDetailsTextSx,
} as const;

type TaskTextField = "name" | "description";
type TaskTextSaveStatus = "idle" | "saving" | "saved" | "error";
type TaskView = "chat" | "desktop" | "browser" | "changes" | "files" | "details";

interface SpecTaskDetailContentProps {
  taskId: string;
  /** Keep standalone task content inset while allowing sibling drawers to span the workspace. */
  padContent?: boolean;
  /** Whether tasks awaiting spec review should open the review automatically. */
  autoOpenReview?: boolean;
  /** Replace route close with reversible content-panel collapse. */
  allowContentCollapse?: boolean;
  onClose?: () => void;
  /** Called when user clicks "Review Spec" - if provided, opens in workspace pane instead of navigating */
  onOpenReview?: (
    taskId: string,
    reviewId: string,
    reviewTitle?: string,
  ) => void;
  /** Called when task is archived - parent should close all tabs showing this task */
  onTaskArchived?: (taskId: string) => void;
  /**
   * Whether to sync the active view (chat/desktop/browser/changes/details) with the URL `view` query param.
   * Defaults to true. Set to false when this component is rendered inside a multi-panel container
   * (e.g. TabsView split-screen) where each panel must own its view independently — otherwise all
   * visible instances mirror the same URL param.
   */
  syncViewWithUrl?: boolean;
}

const SpecTaskDetailContent: FC<SpecTaskDetailContentProps> = ({
  taskId,
  padContent = false,
  autoOpenReview = true,
  allowContentCollapse = false,
  onClose,
  onOpenReview,
  onTaskArchived,
  syncViewWithUrl = true,
}) => {
  const api = useApi();
  const snackbar = useSnackbar();
  const account = useAccount();
  const streaming = useStreaming();
  const apps = useApps();
  const updateSpecTask = useUpdateSpecTask();
  const autoSaveSpecTask = useUpdateSpecTask();
  const moveToBacklogMutation = useMoveToBacklog(taskId);
  const queryClient = useQueryClient();
  const router = useRouter();

  // OAuth flow is needed when a user lacks the repository provider connection.
  const { startOAuthFlow } = useOAuthFlow();
  const { data: oauthProviders } = useListOAuthProviders();

  // Use md breakpoint (900px) to enable split view on tablets
  const isBigScreen = useIsBigScreen({ breakpoint: "md" });
  const lightTheme = useLightTheme();
  const savedSpecTaskChatLayout = loadPanelLayout(
    SPEC_TASK_CHAT_LAYOUT_KEY,
    SPEC_TASK_CHAT_PANEL_IDS,
  );
  const lastExpandedContentSizeRef = useRef(
    savedSpecTaskChatLayout?.["spec-task-content"] ?? 50,
  );

  // Fetch task data
  const { data: task } = useSpecTask(taskId, {
    enabled: !!taskId,
    refetchInterval: 2300, // 2.3s - prime to avoid sync with other polling
  });
  const { data: projectTasks = [] } = useSpecTasks({
    projectId: task?.project_id,
    withDependsOn: true,
    enabled: !!task?.project_id,
  });

  const { data: taskAuthor } = getUserById(task?.created_by || "", !!task?.created_by);
  const authorDisplay =
    taskAuthor?.full_name ||
    taskAuthor?.username ||
    taskAuthor?.email ||
    task?.created_by;

  // Label state
  const { data: projectLabels = [] } = useProjectLabels(
    task?.project_id || "",
  );
  const addLabelMutation = useAddLabel();
  const removeLabelMutation = useRemoveLabel();
  const [labelInput, setLabelInput] = useState("");

  // Fetch zed threads for thread switching
  const { data: zedThreadsData } = useZedThreads(taskId);

  // Fetch project and repositories to get default branch
  const { data: project } = useGetProject(
    task?.project_id || "",
    !!task?.project_id,
  );
  const { data: projectRepositories = [] } = useGetProjectRepositories(
    task?.project_id || "",
    !!task?.project_id,
  );

  const primaryRepository = useMemo(
    () => projectRepositories.find(
      (r) => r.id === project?.default_repo_id,
    ),
    [projectRepositories, project?.default_repo_id],
  );

  const defaultBranchName = useMemo(() => {
    return primaryRepository?.default_branch || "main";
  }, [primaryRepository?.default_branch]);

  // Name and description edit independently and save when focus leaves the field.
  const [editingTextField, setEditingTextField] =
    useState<TaskTextField | null>(null);
  const [editFormData, setEditFormData] = useState({
    name: "",
    description: "",
  });
  const [textSaveStatus, setTextSaveStatus] = useState<
    Record<TaskTextField, TaskTextSaveStatus>
  >({ name: "idle", description: "idle" });
  const textSaveInFlightRef = useRef<Set<TaskTextField>>(new Set());

  // Name and description are task metadata, so their editability should not
  // depend on the task's workflow stage.
  const isTaskDetailsEditable = Boolean(task && !task.archived);

  useEffect(() => {
    if (editingTextField) return;
    setEditFormData({
      name: task?.user_short_title || task?.name || "",
      description: task?.description || task?.original_prompt || "",
    });
  }, [
    editingTextField,
    task?.user_short_title,
    task?.name,
    task?.description,
    task?.original_prompt,
  ]);

  // Agent selection state
  const [selectedAgent, setSelectedAgent] = useState("");
  const [updatingAgent, setUpdatingAgent] = useState(false);
  const [assigneeAnchorEl, setAssigneeAnchorEl] = useState<HTMLElement | null>(null);
  const orgMembers = account.organizationTools.organization?.memberships || [];
  const assignedUser = resolveOrganizationUser(task?.assignee_id, orgMembers, account.user);

  // Start planning state - prevents double-click
  const [isStartingPlanning, setIsStartingPlanning] = useState(false);

  // Chat panel collapse state - when true, uses mobile-style tab layout even on desktop
  const [chatCollapsed, setChatCollapsed] = useState(false);
  const [contentCollapsed, setContentCollapsed] = useState(false);
  const contentPanelRef = useRef<PanelImperativeHandle>(null);
  const collapseContentAfterSplitRef = useRef(false);

  const collapseContentPanel = useCallback(() => {
    if (chatCollapsed) {
      collapseContentAfterSplitRef.current = true;
      setChatCollapsed(false);
      return;
    }
    const currentSize = contentPanelRef.current?.getSize().asPercentage;
    if (currentSize && currentSize > 0) {
      lastExpandedContentSizeRef.current = currentSize;
    }
    contentPanelRef.current?.collapse();
  }, [chatCollapsed]);

  const showContentPanel = useCallback(() => {
    const panel = contentPanelRef.current;
    if (!panel) return;
    const restoredSize = lastExpandedContentSizeRef.current || 50;
    panel.expand();
    panel.resize(`${restoredSize}%`);
  }, []);

  useEffect(() => {
    if (chatCollapsed || !collapseContentAfterSplitRef.current) return;
    collapseContentAfterSplitRef.current = false;
    contentPanelRef.current?.collapse();
  }, [chatCollapsed]);

  const [terminalDrawerState, setTerminalDrawerState] = useState(() =>
    loadSpecTaskTerminalDrawerState(taskId),
  );

  useEffect(() => {
    setTerminalDrawerState(loadSpecTaskTerminalDrawerState(taskId));
  }, [taskId]);

  const toggleTerminalDrawer = useCallback(() => {
    setTerminalDrawerState((current) => {
      const next = { ...current, open: !current.open };
      saveSpecTaskTerminalDrawerState(taskId, next);
      return next;
    });
  }, [taskId]);

  const setTerminalDrawerHeight = useCallback((height: number) => {
    setTerminalDrawerState((current) => {
      const next = { ...current, height };
      saveSpecTaskTerminalDrawerState(taskId, next);
      return next;
    });
  }, [taskId]);

  const closeTerminalDrawer = useCallback(() => {
    setTerminalDrawerState((current) => {
      const next = { ...current, open: false };
      saveSpecTaskTerminalDrawerState(taskId, next);
      return next;
    });
  }, [taskId]);

  // Spec tasks can only select coding agents.
  const eligibleApps = useMemo(() => {
    if (!apps.apps) return [];
    return selectCodingAgents(apps.apps);
  }, [apps.apps]);

  // Get display settings from the task's app configuration
  const displaySettings = useMemo(() => {
    const taskApp = apps.apps?.find((a) => a.id === task?.helix_app_id);
    return deriveDisplaySettings(taskApp);
  }, [task?.helix_app_id, apps.apps]);

  // Check if the task's app uses Claude Code with subscription credentials
  const { data: claudeSubscriptions } = useClaudeSubscriptions();
  const claudeTokenExpiry = useMemo(() => {
    if (!task?.helix_app_id || !apps.apps) return null;
    const taskApp = apps.apps.find((a) => a.id === task.helix_app_id);
    const assistant = taskApp?.config?.helix?.assistants?.[0];
    if (
      assistant?.code_agent_runtime !== "claude_code" ||
      assistant?.code_agent_credential_type !== "subscription"
    )
      return null;
    const sub = claudeSubscriptions?.[0];
    if (!sub) return null;
    if (sub.credential_type === 'setup_token') return null; // Setup tokens don't expire
    return getTokenExpiryStatus(sub.access_token_expires_at);
  }, [task?.helix_app_id, apps.apps, claudeSubscriptions]);

  // Sync selected agent when task changes
  useEffect(() => {
    if (task?.helix_app_id) {
      setSelectedAgent(task.helix_app_id);
    }
  }, [task?.helix_app_id]);

  // Load apps on mount
  useEffect(() => {
    apps.loadApps();
  }, []);

  // On mobile, 'chat' is a separate tab; on desktop, chat is always visible
  // Initialize from URL query param 'view' if present (only when syncing with URL)
  const getInitialView = (): TaskView => {
    if (!syncViewWithUrl) {
      return "desktop";
    }
    const viewParam = router.params.view;
    if (
      viewParam === "chat" ||
      viewParam === "desktop" ||
      viewParam === "browser" ||
      viewParam === "changes" ||
      viewParam === "files" ||
      viewParam === "details"
    ) {
      return viewParam;
    }
    // On mobile (below md breakpoint / 900px), default to chat view
    // since the desktop stream is less useful on small screens.
    // This matches the breakpoint used for split-view switching (isBigScreen).
    const isMobile = window.matchMedia("(max-width: 899.95px)").matches;
    return isMobile ? "chat" : "desktop";
  };
  const [currentView, setCurrentView] = useState<TaskView>(getInitialView);
  const [clientUniqueId, setClientUniqueId] = useState<string>("");

  // Sync currentView with URL query param (only when syncing with URL).
  // When rendered inside a multi-panel container (split-screen), this sync is
  // disabled so each panel owns its own view independently.
  useEffect(() => {
    if (!syncViewWithUrl) return;
    const viewParam = router.params.view;
    if (
      viewParam &&
      (viewParam === "chat" ||
        viewParam === "desktop" ||
        viewParam === "browser" ||
        viewParam === "changes" ||
        viewParam === "files" ||
        viewParam === "details")
    ) {
      if (viewParam !== currentView) {
        setCurrentView(viewParam);
      }
    }
  }, [router.params.view, syncViewWithUrl]);

  // Update URL when view changes (only when syncing with URL)
  const handleViewChange = useCallback(
    (newView: TaskView | null) => {
      if (newView && newView !== currentView) {
        setCurrentView(newView);
        if (syncViewWithUrl) {
          router.mergeParams({ view: newView });
        }
      }
    },
    [currentView, router, syncViewWithUrl],
  );


  // Design review state
  const [docViewerOpen, setDocViewerOpen] = useState(false);
  const [implementationReviewMessageSent, setImplementationReviewMessageSent] =
    useState(false);

  // Session restart state
  const [restartConfirmOpen, setRestartConfirmOpen] = useState(false);
  const [isRestarting, setIsRestarting] = useState(false);

  // Session stop state
  const [stopConfirmOpen, setStopConfirmOpen] = useState(false);
  const [isStopping, setIsStopping] = useState(false);
  const [isStarting, setIsStarting] = useState(false);

  // Just Do It mode state
  const [justDoItMode, setJustDoItMode] = useState(false);
  const [updatingJustDoIt, setUpdatingJustDoIt] = useState(false);

  // File upload state
  const fileInputRef = useRef<HTMLInputElement>(null);
  const [isUploading, setIsUploading] = useState(false);

  // (auto-open tracking is handled by sessionStorage so it persists across page refreshes)

  // Clone dialog state
  const [showCloneDialog, setShowCloneDialog] = useState(false);
  const [selectedCloneGroupId, setSelectedCloneGroupId] = useState<
    string | null
  >(null);
  const [actionMenuAnchorEl, setActionMenuAnchorEl] =
    useState<HTMLElement | null>(null);

  // Archive state
  const [archiveConfirmOpen, setArchiveConfirmOpen] = useState(false);
  const [isArchiving, setIsArchiving] = useState(false);

  // Public design docs state
  const [isPublicDesignDocs, setIsPublicDesignDocs] = useState(
    task?.public_design_docs ?? false,
  );
  const [updatingPublic, setUpdatingPublic] = useState(false);
  const [shareDialogOpen, setShareDialogOpen] = useState(false);

  // Sync public state when task data changes
  useEffect(() => {
    if (task?.public_design_docs !== undefined) {
      setIsPublicDesignDocs(task.public_design_docs);
    }
  }, [task?.public_design_docs]);

  // Points at the unauthenticated, server-rendered public viewer
  // (subRouter route GET /api/v1/spec-tasks/{id}/view). The /api/v1 prefix is
  // required — without it the URL hits the SPA and forces an OIDC login.
  const publicLink = `${window.location.origin}/api/v1/spec-tasks/${taskId}/view`;

  const handlePublicToggle = async (
    event: React.ChangeEvent<HTMLInputElement>,
  ) => {
    const newValue = event.target.checked;
    setUpdatingPublic(true);
    try {
      await api.getApiClient().v1SpecTasksUpdate(taskId, {
        public_design_docs: newValue,
      });
      setIsPublicDesignDocs(newValue);
      snackbar.success(
        newValue ? "Design docs are now public" : "Design docs are now private",
      );
    } catch (err: any) {
      snackbar.error(err.message || "Failed to update visibility");
    } finally {
      setUpdatingPublic(false);
    }
  };

  // Fetch clone groups where this task was the source
  const { data: cloneGroups } = useCloneGroups(taskId);

  const currentTaskDependencies = useMemo(
    () =>
      projectTasks.find((projectTask) => projectTask.id === task?.id)
        ?.depends_on ||
      task?.depends_on ||
      [],
    [projectTasks, task?.id, task?.depends_on],
  );

  // Check if task is completed/merged - container is shut down so desktop view won't work
  const isTaskCompleted = task?.status === "done" || task?.merged_to_main;

  // Check if task is archived/rejected - container is shut down so desktop view won't work
  const isTaskArchived = task?.archived;

  // Check if task can be moved to backlog (not in backlog or queued)
  const isQueued =
    task?.status === "queued_implementation" ||
    task?.status === "queued_spec_generation" ||
    task?.status === "spec_approved";
  const canMoveToBacklog =
    task && !isQueued && task.status !== "backlog" && !isTaskArchived;

  // Thread selection state for switching between planning and implementation threads
  const [selectedThreadSessionId, setSelectedThreadSessionId] = useState<
    string | null
  >(null);

  // Get the active session ID - keep it available for chat history even when task is completed
  const activeSessionId = selectedThreadSessionId || task?.planning_session_id;

  useEffect(() => {
    const handleTerminalShortcut = (event: KeyboardEvent) => {
      if (!activeSessionId || !isSpecTaskTerminalToggleShortcut(event)) return;
      event.preventDefault();
      event.stopPropagation();
      setTerminalDrawerState((current) => {
        const next = { ...current, open: !current.open };
        saveSpecTaskTerminalDrawerState(taskId, next);
        return next;
      });
    };
    window.addEventListener("keydown", handleTerminalShortcut);
    return () => window.removeEventListener("keydown", handleTerminalShortcut);
  }, [activeSessionId, taskId]);

  // Track sandbox/desktop state for stop/start buttons
  const {
    isRunning: isDesktopRunning,
    isPaused: isDesktopPaused,
    isStarting: isDesktopStarting,
    hasDesktopLifecycleState,
  } = useSandboxState(activeSessionId || "");

  const launchPhase = getSpecTaskLaunchPhase({
    status: task?.status,
    queueReason: task?.queue_reason,
    activeSessionId,
    hasDesktopLifecycleState,
  });

  // When the task is queued for planning, the backend hasn't created the session yet (or the
  // planning_session_id still points to a previously-stopped session). In either case, suppress
  // the "paused/stopped" UI and treat the desktop as starting so the user sees "Starting Desktop"
  // immediately after clicking "Start Planning" rather than a confusing flash of the stopped state.
  const isQueuedForPlanning = task?.status === "queued_spec_generation";
  const effectiveIsDesktopPaused = isDesktopPaused && !isQueuedForPlanning;

  // Subscribe to WebSocket updates for the active session when chat is visible
  // On big screens: chat is visible unless collapsed
  // On mobile: chat is visible when currentView === 'chat'
  const isChatVisible = isBigScreen ? !chatCollapsed : currentView === "chat";

  useEffect(() => {
    if (activeSessionId && isChatVisible) {
      streaming.setCurrentSessionId(activeSessionId);
    } else {
      // Clear subscription when chat is hidden to disconnect WebSocket
      streaming.setCurrentSessionId(null);
    }
  }, [activeSessionId, isChatVisible]);

  // Optimistic UI hook fired the moment the user hits Send: flips the cached
  // session config to external_agent_status="starting" so a paused desktop
  // shows the spinner immediately instead of waiting up to 3s for the next
  // session poll. Polling reconciles to the authoritative backend value.
  const handleWillSend = useCallback(() => {
    if (!activeSessionId) return;
    optimisticallyMarkSessionStarting(queryClient, activeSessionId);
  }, [queryClient, activeSessionId]);

  // Default to appropriate view based on session state and screen size
  useEffect(() => {
    if (activeSessionId && currentView === "details") {
      // If there's an active session and we're on details, switch to appropriate view
      // On mobile, default to chat; on desktop, default to desktop (chat is always visible)
      const newView = isBigScreen ? "desktop" : "chat";
      setCurrentView(newView);
      if (syncViewWithUrl) {
        router.mergeParams({ view: newView });
      }
    } else if (!activeSessionId && currentView !== "details") {
      // If no active session, switch to details view
      setCurrentView("details");
      if (syncViewWithUrl) {
        router.mergeParams({ view: "details" });
      }
    }
  }, [activeSessionId, isBigScreen]);

  // Fetch session data
  const { data: sessionResponse } = useGetSession(activeSessionId || "", {
    enabled: !!activeSessionId,
    refetchInterval: 3000,
  });
  const sessionData = sessionResponse?.data;

  // Keep the streamed Zed desktop foregrounded on the thread of the session the
  // user is actually viewing. A spec task can have multiple sessions/threads
  // sharing ONE desktop; the chat panel and message routing are session-scoped,
  // but selecting a thread previously only re-bound the chat panel — nothing told
  // the desktop to open that session's thread. So the desktop could show one
  // thread while messages went to another ("opened != sent-to") with no session
  // switch. Driving this off the derived activeSessionId covers both thread
  // selectors and initial mount. Backend no-ops (and never auto-starts a
  // container) when the desktop isn't connected. See
  // design/2026-06-22-zed-open-thread-send-mismatch.md.
  const foregroundThread = useMutation({
    mutationFn: (sessionId: string) =>
      api.getApiClient().v1SessionsForegroundThreadCreate(sessionId),
  });
  const isSessionPaused = !!sessionData?.config?.paused;
  useEffect(() => {
    if (!activeSessionId) return;
    if (!isDesktopRunning) return; // nothing to foreground; also avoids waking a stopped desktop
    if (isSessionPaused) return;
    // foregroundThread.mutate is a stable React Query fn; per frontend/CLAUDE.md
    // the dependency array carries only the primitives that should retrigger this.
    foregroundThread.mutate(activeSessionId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [activeSessionId, isDesktopRunning, isSessionPaused]);

  const taskMetadataError =
    typeof task?.metadata?.error === "string" ? task.metadata.error : "";

  // A launch refused for a missing subscription is not retryable: send the user
  // to the provider login instead of offering a start button that fails again.
  const subscriptionRequirement = subscriptionRequirementFromTask(
    task?.metadata as Record<string, unknown> | undefined,
  );
  const desktopStartupMessage = subscriptionRequirement
    ? subscriptionRequirementMessage(subscriptionRequirement)
    : taskMetadataError;
  const connectSubscriptionLabel = subscriptionRequirement
    ? `Connect ${subscriptionRequirement.label}`
    : undefined;
  const connectSubscription = useCallback(() => {
    const organizationId = project?.organization_id;
    if (organizationId) router.navigate("org_providers", { org_id: organizationId });
  }, [project?.organization_id, router]);

  // Sync justDoItMode when task changes
  useEffect(() => {
    if (task?.just_do_it_mode !== undefined) {
      setJustDoItMode(task.just_do_it_mode);
    }
  }, [task?.just_do_it_mode]);

  const getPriorityColor = (priority: string) => {
    switch (priority?.toLowerCase()) {
      case "critical":
        return "error";
      case "high":
        return "warning";
      case "medium":
        return "info";
      case "low":
        return "success";
      default:
        return "default";
    }
  };

  const getStatusColor = (status: string) => {
    switch (status) {
      case "spec_approved":
      case "implementation_complete":
      case "completed":
        return "success";
      case "in_progress":
      case "spec_generation":
      case "implementation_in_progress":
        return "primary";
      case "spec_review":
        return "warning";
      case "backlog":
        return "default";
      default:
        return "default";
    }
  };

  const formatStatus = (status: string) => {
    return status
      ?.split("_")
      .map((word) => word.charAt(0).toUpperCase() + word.slice(1))
      .join(" ");
  };

  const handleStartPlanning = async () => {
    if (!task?.id || isStartingPlanning) return;

    setIsStartingPlanning(true);
    try {
      const { keyboardLayout, timezone, isOverridden } = getBrowserLocale();
      const queryParams = new URLSearchParams();
      if (keyboardLayout) queryParams.set("keyboard", keyboardLayout);
      if (timezone) queryParams.set("timezone", timezone);
      const queryString = queryParams.toString();
      const url = `/api/v1/spec-tasks/${task.id}/start-planning${queryString ? `?${queryString}` : ""}`;

      const csrfToken = getCSRFToken();
      const response = await fetch(url, {
        method: "POST",
        headers: {
          "Content-Type": "application/json",
          ...(csrfToken && { "X-CSRF-Token": csrfToken }),
        },
        credentials: "include",
      });
      if (!response.ok) {
        const errorData = await response.json().catch(() => ({}));
        // Open the matching provider connection flow on OAuth enforcement.
        if (
          response.status === 422 &&
          errorData?.error === "oauth_required"
        ) {
          const providerType = errorData?.provider_type === "gitlab" ? "gitlab" : "github";
          const providerName = providerType === "gitlab" ? "GitLab" : "GitHub";
          const oauthProvider = findOAuthProviderForType(oauthProviders, providerType);
          if (oauthProvider?.id) {
            snackbar.info(`Connect ${providerName} to start planning this task.`);
            startOAuthFlow({
              providerId: oauthProvider.id,
              scopes: vcsScopesForProvider(oauthProvider.type, oauthProvider.name),
              onSuccess: () => {
                snackbar.success(
                  `${providerName} connected. Click Start Planning again to continue.`,
                );
              },
              onError: (oauthError) => {
                snackbar.error(`${providerName} connection failed: ${oauthError}`);
              },
            });
          } else {
            // No GitHub provider is configured system-wide. The backend's
            // error message is PR-centric and actionless for this user, so
            // override it with admin-direction guidance.
            snackbar.error(
              `${providerName} OAuth is not configured on this Helix instance. Ask your administrator to set it up before starting planning.`,
            );
          }
          return;
        }
        throw new Error(
          errorData.message ||
            errorData.error ||
            `Failed to start planning: ${response.statusText}`,
        );
      }

      snackbar.success("Planning started! Agent session will begin shortly.");
      handleViewChange("desktop");
    } catch (err: any) {
      console.error("Failed to start planning:", err);
      snackbar.error(
        err?.message || "Failed to start planning. Please try again.",
      );
    } finally {
      setIsStartingPlanning(false);
    }
  };

  // Handle session restart
  const handleRestartSession = useCallback(async () => {
    if (!activeSessionId || isRestarting) return;

    setIsRestarting(true);
    setRestartConfirmOpen(false);

    try {
      snackbar.info("Restarting agent session...");
      // Single backend call: the restart-agent endpoint tears down the
      // desktop container and recreates it (preserving thread context and
      // resetting crashed prompts). No frontend stop/sleep/resume dance.
      await api.getApiClient().v1SessionsRestartAgentCreate(activeSessionId);
      queryClient.invalidateQueries({
        queryKey: GET_SESSION_QUERY_KEY(activeSessionId),
      });
      snackbar.success("Session restarted successfully");
    } catch (err: any) {
      console.error("Failed to restart session:", err);
      snackbar.error(err?.message || "Failed to restart session");
    } finally {
      setIsRestarting(false);
    }
  }, [activeSessionId, isRestarting, api, snackbar, queryClient]);

  // Handle session stop
  const handleStopSession = useCallback(async () => {
    if (!activeSessionId || isStopping) return;

    setIsStopping(true);
    setStopConfirmOpen(false);

    try {
      snackbar.info("Stopping desktop...");
      await api
        .getApiClient()
        .v1SessionsStopExternalAgentDelete(activeSessionId);
      queryClient.invalidateQueries({
        queryKey: GET_SESSION_QUERY_KEY(activeSessionId),
      });
      snackbar.success("Desktop stopped");
    } catch (err: any) {
      console.error("Failed to stop session:", err);
      snackbar.error(err?.message || "Failed to stop desktop");
    } finally {
      setIsStopping(false);
    }
  }, [activeSessionId, isStopping, api, snackbar, queryClient]);

  // Handle session start (resume from stopped state)
  const handleStartSession = useCallback(async () => {
    if (!activeSessionId || isStarting) return;

    setIsStarting(true);

    try {
      snackbar.info("Starting desktop...");
      await api.getApiClient().v1SessionsResumeCreate(activeSessionId);
      queryClient.invalidateQueries({
        queryKey: GET_SESSION_QUERY_KEY(activeSessionId),
      });
      snackbar.success("Desktop started");
    } catch (err: any) {
      console.error("Failed to start session:", err);
      snackbar.error(err?.message || "Failed to start desktop");
    } finally {
      setIsStarting(false);
    }
  }, [activeSessionId, isStarting, api, snackbar, queryClient]);

  // Toggle keep alive (prevent auto-idle-shutdown)
  const handleToggleKeepAlive = useCallback(async () => {
    if (!task?.id) return;

    const newValue = !task.keep_alive;
    try {
      await updateSpecTask.mutateAsync({
        taskId: task.id,
        updates: { keep_alive: newValue },
      });
      snackbar.success(
        newValue
          ? "Keep Alive enabled — container won't auto-sleep"
          : "Keep Alive disabled — container will auto-sleep when idle",
      );
    } catch (err) {
      console.error("Failed to toggle Keep Alive:", err);
      snackbar.error("Failed to toggle Keep Alive");
    }
  }, [task?.id, task?.keep_alive, updateSpecTask, snackbar]);

  // Toggle Just Do It mode
  const handleToggleJustDoIt = useCallback(async () => {
    if (!task?.id || updatingJustDoIt) return;

    const newValue = !justDoItMode;
    setUpdatingJustDoIt(true);

    try {
      await updateSpecTask.mutateAsync({
        taskId: task.id,
        updates: { just_do_it_mode: newValue },
      });
      setJustDoItMode(newValue);
      snackbar.success(
        newValue ? "Just Do It mode enabled" : "Just Do It mode disabled",
      );
    } catch (err) {
      console.error("Failed to update Just Do It mode:", err);
      snackbar.error("Failed to update Just Do It mode");
    } finally {
      setUpdatingJustDoIt(false);
    }
  }, [task?.id, justDoItMode, updatingJustDoIt, updateSpecTask, snackbar]);

  // Handle agent change
  const handleAgentChange = useCallback(
    async (newAgentId: string) => {
      if (!task?.id || updatingAgent || newAgentId === selectedAgent) return;

      setUpdatingAgent(true);
      const previousAgent = selectedAgent;
      setSelectedAgent(newAgentId);

      try {
        await updateSpecTask.mutateAsync({
          taskId: task.id,
          updates: { helix_app_id: newAgentId },
        });
        snackbar.success("Agent updated");
      } catch (err) {
        console.error("Failed to update agent:", err);
        snackbar.error("Failed to update agent");
        setSelectedAgent(previousAgent);
      } finally {
        setUpdatingAgent(false);
      }
    },
    [task?.id, selectedAgent, updatingAgent, updateSpecTask, snackbar],
  );

  const handleAssigneeChange = useCallback(
    async (userId: string | null) => {
      if (!task?.id || updateSpecTask.isPending) return;

      try {
        await updateSpecTask.mutateAsync({
          taskId: task.id,
          updates: { assignee_id: userId || "" },
        });
        snackbar.success(userId ? "Assignee updated" : "Task unassigned");
      } catch (err) {
        console.error("Failed to update assignee:", err);
        snackbar.error("Failed to update assignee");
      }
    },
    [task?.id, updateSpecTask.isPending, updateSpecTask, snackbar],
  );

  const handleTextFieldEdit = (field: TaskTextField) => {
    if (!task || !isTaskDetailsEditable) return;

    setEditFormData((current) => ({
      ...current,
      [field]:
        field === "name"
          ? task.user_short_title || task.name || ""
          : task.description || task.original_prompt || "",
    }));
    setTextSaveStatus((current) => ({ ...current, [field]: "idle" }));
    setEditingTextField(field);
  };

  const handleTextFieldCancel = (field: TaskTextField) => {
    const currentValue =
      field === "name"
        ? task?.user_short_title || task?.name || ""
        : task?.description || task?.original_prompt || "";
    setEditFormData((current) => ({ ...current, [field]: currentValue }));
    setTextSaveStatus((current) => ({ ...current, [field]: "idle" }));
    setEditingTextField((current) => (current === field ? null : current));
  };

  const handleTextFieldBlur = async (field: TaskTextField) => {
    if (!task?.id) return;

    if (textSaveInFlightRef.current.has(field)) {
      setEditingTextField((current) => (current === field ? null : current));
      return;
    }

    const currentValue =
      field === "name"
        ? task.user_short_title || task.name || ""
        : task.description || task.original_prompt || "";
    const nextValue =
      field === "name"
        ? editFormData.name.trim()
        : editFormData.description;

    if (field === "name" && !nextValue) {
      setEditFormData((current) => ({ ...current, name: currentValue }));
      setTextSaveStatus((current) => ({ ...current, name: "error" }));
      setEditingTextField((current) => (current === field ? null : current));
      snackbar.error("Task name cannot be empty");
      return;
    }

    // Leaving the field ends editing immediately. Persistence continues in the
    // background and its state is shown alongside the field label.
    setEditingTextField((current) => (current === field ? null : current));

    if (nextValue === currentValue) {
      setTextSaveStatus((current) => ({ ...current, [field]: "idle" }));
      return;
    }

    setTextSaveStatus((current) => ({ ...current, [field]: "saving" }));
    textSaveInFlightRef.current.add(field);
    try {
      await autoSaveSpecTask.mutateAsync({
        taskId: task.id,
        updates:
          field === "name"
            ? { name: nextValue, user_short_title: nextValue }
            : { description: nextValue },
      });
      setTextSaveStatus((current) => ({ ...current, [field]: "saved" }));
    } catch (err) {
      console.error(`Failed to auto-save task ${field}:`, err);
      setEditFormData((current) => ({
        ...current,
        [field]: currentValue,
      }));
      setTextSaveStatus((current) => ({ ...current, [field]: "error" }));
      snackbar.error(`Failed to save task ${field}`);
    } finally {
      textSaveInFlightRef.current.delete(field);
    }
  };

  const getTextSaveHelper = (field: TaskTextField) => {
    switch (textSaveStatus[field]) {
      case "saving":
        return "Saving…";
      case "saved":
        return "Saved";
      case "error":
        return "Not saved";
      default:
        return "Saves when you leave the field";
    }
  };

  // Handle review spec navigation
  const handleReviewSpec = useCallback(async () => {
    if (!task?.id) return;

    // Mark immediately (before async) so the auto-open effect won't re-trigger
    // if the user returns to the chat view after visiting spec.
    addAutoOpenedSpecTask(task.id);

    try {
      const response = await api
        .getApiClient()
        .v1SpecTasksDesignReviewsDetail(task.id);
      const reviews = response.data?.reviews || [];
      if (reviews.length > 0) {
        const latestReview =
          reviews.find((r: any) => r.status !== "superseded") || reviews[0];
        if (onOpenReview) {
          onOpenReview(task.id, latestReview.id, task.name || "Spec Review");
        } else {
          account.orgNavigate("project-task-review", {
            id: task.project_id,
            taskId: task.id,
            reviewId: latestReview.id,
          });
        }
      } else {
        snackbar.error("No design review found");
      }
    } catch (error) {
      console.error("Failed to fetch design reviews:", error);
      snackbar.error("Failed to load design review");
    }
  }, [task?.id, task?.name, task?.project_id, onOpenReview, account]);

  // Auto-open spec review when enabled and the task is ready for review.
  // The Chat task route disables this so selecting a task preserves the Chat context.
  // handleReviewSpec writes to sessionStorage before the async call, limiting auto-open
  // to once per SPA session per task ID in views where it remains enabled.
  // The spec_approved_at guard prevents bouncing the user back to the review page in the
  // brief window between approval and the cached task.status transitioning away from spec_review.
  useEffect(() => {
    if (
      autoOpenReview &&
      task?.id &&
      !getAutoOpenedSpecTasks().has(task.id) &&
      !task?.spec_approved_at &&
      task?.design_docs_pushed_at &&
      account.organizationTools.organization?.name &&
      (task?.status === TypesSpecTaskStatus.TaskStatusSpecReview ||
        task?.status === TypesSpecTaskStatus.TaskStatusSpecRevision)
    ) {
      handleReviewSpec();
    }
  }, [autoOpenReview, task?.id, task?.status, task?.spec_approved_at, task?.design_docs_pushed_at, handleReviewSpec, account.organizationTools.organization?.name]);

  // Handle file upload to sandbox
  const handleUploadClick = useCallback(() => {
    if (!activeSessionId) {
      snackbar.error("Please start the task first before uploading files");
      return;
    }
    fileInputRef.current?.click();
  }, [activeSessionId, snackbar]);

  const handleFileChange = useCallback(
    async (e: React.ChangeEvent<HTMLInputElement>) => {
      const files = e.target.files;
      if (!files || files.length === 0 || !activeSessionId) return;

      setIsUploading(true);
      let successCount = 0;
      let errorCount = 0;

      for (const file of Array.from(files)) {
        try {
          const formData = new FormData();
          formData.append("file", file);

          const csrfToken = getCSRFToken();
          const response = await fetch(
            `/api/v1/external-agents/${activeSessionId}/upload?open_file_manager=false`,
            {
              method: "POST",
              body: formData,
              headers: csrfToken ? { "X-CSRF-Token": csrfToken } : undefined,
            },
          );

          if (response.ok) {
            successCount++;
          } else {
            errorCount++;
          }
        } catch (error) {
          console.error(`Failed to upload ${file.name}:`, error);
          errorCount++;
        }
      }

      setIsUploading(false);

      if (successCount > 0 && errorCount === 0) {
        snackbar.success(
          `Uploaded ${successCount} file${successCount > 1 ? "s" : ""} to ~/work/incoming`,
        );
      } else if (successCount > 0 && errorCount > 0) {
        snackbar.info(`Uploaded ${successCount}, ${errorCount} failed`);
      } else if (errorCount > 0) {
        snackbar.error(
          `Failed to upload ${errorCount} file${errorCount > 1 ? "s" : ""}`,
        );
      }

      // Clear the input so the same file can be uploaded again
      if (fileInputRef.current) {
        fileInputRef.current.value = "";
      }
    },
    [activeSessionId, snackbar],
  );

  // Handle archive task
  const handleArchiveClick = useCallback((e: React.MouseEvent) => {
    if (e.shiftKey) {
      // Shift+click bypasses confirmation
      performArchive();
    } else {
      setArchiveConfirmOpen(true);
    }
  }, []);

  const performArchive = useCallback(async () => {
    if (!task?.id || isArchiving) return;

    setIsArchiving(true);
    setArchiveConfirmOpen(false);

    try {
      await api
        .getApiClient()
        .v1SpecTasksArchivePartialUpdate(task.id, { archived: true });
      snackbar.success("Task archived");
      // If parent handles task archival (TabsView), let it close all tabs
      if (onTaskArchived) {
        onTaskArchived(task.id);
      } else {
        // Fallback: navigate back to the project specs page (standalone usage)
        if (task.project_id) {
          account.orgNavigate("project-specs", { id: task.project_id });
        }
        if (onClose) onClose();
      }
    } catch (err) {
      console.error("Failed to archive task:", err);
      snackbar.error("Failed to archive task");
    } finally {
      setIsArchiving(false);
    }
  }, [
    task?.id,
    task?.project_id,
    isArchiving,
    api,
    snackbar,
    account,
    onClose,
    onTaskArchived,
  ]);

  const renderTaskActions = (variant: "inline" | "stacked") => {
    if (!task) return null;

    return (
      <SpecTaskActionButtons
        task={{
          id: task.id || "",
          status: task.status || "",
          design_docs_pushed_at: task.design_docs_pushed_at,
          repo_pull_requests: task.repo_pull_requests,
          base_branch: task.base_branch,
          branch_name: task.branch_name,
          archived: task.archived,
          just_do_it_mode: justDoItMode,
          planning_session_id: task.planning_session_id,
          metadata: task.metadata as { error?: string },
          last_push_at: task.last_push_at,
        }}
        variant={variant}
        onStartPlanning={handleStartPlanning}
        onReviewSpec={handleReviewSpec}
        onReject={(shiftKey) => {
          if (shiftKey) {
            performArchive();
          } else {
            setArchiveConfirmOpen(true);
          }
        }}
        hasExternalRepo={projectRepositories.some(
          (repository) =>
            repository.is_external ||
            repository.external_type ||
            repository.external_url,
        )}
        externalRepoType={projectRepositories.find(
          (repository) => repository.external_type,
        )?.external_type}
        isStartingPlanning={isStartingPlanning}
        isArchiving={isArchiving}
      />
    );
  };

  // Render the details content (used in both desktop left panel and mobile/no-session view)
  const renderDetailsContent = () => (
    <Box sx={{ containerType: "inline-size", maxWidth: 1180, mx: "auto" }}>
      {/* Queued task block reason — explains why a queued task hasn't started yet
          (WIP capacity / dependency). Recomputed server-side each read, so it
          clears automatically as the queue drains. */}
      {(task?.status === "queued_spec_generation" ||
        task?.status === "queued_implementation") &&
        task?.queue_reason && (
          <Alert severity="info" sx={{ mb: 3 }}>
            <Typography variant="body2" sx={{ fontWeight: 500 }}>
              Waiting to start
            </Typography>
            <Typography
              variant="caption"
              sx={{ display: "block", color: "text.secondary" }}
            >
              {task.queue_reason}
            </Typography>
          </Alert>
        )}

      {/* Completed task message */}
      {isTaskCompleted && (
        <Alert severity="success" sx={{ mb: 3 }}>
          <Typography variant="body2" sx={{ fontWeight: 500 }}>
            Task finished
          </Typography>
          <Typography
            variant="caption"
            sx={{ display: "block", color: "text.secondary" }}
          >
            Merged to default branch
          </Typography>
        </Alert>
      )}

      {/* Archived/rejected task message */}
      {isTaskArchived && !isTaskCompleted && (
        <Alert severity="warning" sx={{ mb: 3 }}>
          <Typography variant="body2" sx={{ fontWeight: 500 }}>
            Task rejected
          </Typography>
          <Typography
            variant="caption"
            sx={{ display: "block", color: "text.secondary" }}
          >
            This task has been archived
          </Typography>
        </Alert>
      )}

      {!activeSessionId && task?.status === "backlog" && (
        <Box
          sx={{
            ...taskDetailsSectionSx,
            mb: 2,
            p: { xs: 2, sm: 2.5 },
            display: "flex",
            flexDirection: { xs: "column", sm: "row" },
            alignItems: { xs: "stretch", sm: "center" },
            justifyContent: "space-between",
            gap: 2,
            borderColor: "warning.main",
            backgroundColor: "action.hover",
          }}
        >
          <Box sx={{ minWidth: 0 }}>
            <Typography variant="subtitle1" sx={{ fontWeight: 700 }}>
              {justDoItMode ? "Ready for implementation" : "Ready for planning"}
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
              {justDoItMode
                ? "Launch the selected agent to begin implementing this task."
                : "Start a planning session to turn this task into an implementation-ready spec."}
            </Typography>
          </Box>
          <Box
            sx={{
              width: { xs: "100%", sm: 240 },
              flexShrink: 0,
              "& .MuiButton-root": {
                minHeight: 40,
                fontWeight: 600,
                textTransform: "none",
              },
            }}
          >
            {renderTaskActions("stacked")}
          </Box>
        </Box>
      )}

      <Box
        sx={{
          display: "grid",
          gridTemplateColumns: "minmax(0, 1fr)",
          gap: 2,
          alignItems: "start",
          "@container (min-width: 520px)": {
            gridTemplateColumns: "minmax(0, 1.45fr) minmax(210px, 0.9fr)",
          },
        }}
      >
        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            gap: 2,
            minWidth: 0,
            "& > .MuiBox-root": { mb: 0 },
          }}
        >
          {/* Task name and description */}
          <Box sx={taskDetailsSectionSx}>
            <Box
              sx={{ display: "flex", justifyContent: "space-between", gap: 1 }}
            >
              <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                Task name
              </Typography>
              {editingTextField !== "name" &&
                textSaveStatus.name !== "idle" && (
                  <Typography
                    variant="caption"
                    color={
                      textSaveStatus.name === "error"
                        ? "error.main"
                        : "text.secondary"
                    }
                  >
                    {getTextSaveHelper("name")}
                  </Typography>
                )}
            </Box>
            {editingTextField === "name" ? (
              <ClickAwayListener
                onClickAway={() => void handleTextFieldBlur("name")}
              >
                <Box>
                  <TextField
                    fullWidth
                    size="small"
                    value={editFormData.name}
                    onChange={(e) => {
                      setEditFormData((current) => ({
                        ...current,
                        name: e.target.value,
                      }));
                      setTextSaveStatus((current) => ({
                        ...current,
                        name: "idle",
                      }));
                    }}
                    onBlur={() => void handleTextFieldBlur("name")}
                    onKeyDown={(event) => {
                      if (event.key === "Enter") {
                        event.currentTarget.querySelector("input")?.blur();
                      } else if (event.key === "Escape") {
                        handleTextFieldCancel("name");
                      }
                    }}
                    error={textSaveStatus.name === "error"}
                    helperText={getTextSaveHelper("name")}
                    autoFocus
                    placeholder="Task name"
                    sx={taskDetailsTextFieldSx}
                  />
                </Box>
              </ClickAwayListener>
            ) : (
              <Box
                role={isTaskDetailsEditable ? "button" : undefined}
                tabIndex={isTaskDetailsEditable ? 0 : undefined}
                onClick={() => handleTextFieldEdit("name")}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    handleTextFieldEdit("name");
                  }
                }}
                sx={{
                  cursor: isTaskDetailsEditable ? "text" : "default",
                  borderRadius: 1,
                  mx: -1,
                  px: 1,
                  py: 0.5,
                  "&:hover": isTaskDetailsEditable
                    ? { backgroundColor: "action.hover" }
                    : {},
                }}
              >
                <Typography variant="body2" sx={taskDetailsTextSx}>
                  {textSaveStatus.name === "saving" ||
                  textSaveStatus.name === "saved"
                    ? editFormData.name.trim()
                    : task?.user_short_title || task?.name || "Untitled task"}
                </Typography>
              </Box>
            )}

            <Divider sx={{ my: 2 }} />

            <Box
              sx={{ display: "flex", justifyContent: "space-between", gap: 1 }}
            >
              <Typography variant="subtitle2" color="text.secondary" gutterBottom>
                Description
              </Typography>
              {editingTextField !== "description" &&
                textSaveStatus.description !== "idle" && (
                  <Typography
                    variant="caption"
                    color={
                      textSaveStatus.description === "error"
                        ? "error.main"
                        : "text.secondary"
                    }
                  >
                    {getTextSaveHelper("description")}
                  </Typography>
                )}
            </Box>
            {editingTextField === "description" ? (
              <ClickAwayListener
                onClickAway={() => void handleTextFieldBlur("description")}
              >
                <Box>
                  <TextField
                    fullWidth
                    multiline
                    minRows={4}
                    maxRows={20}
                    value={editFormData.description}
                    onChange={(e) => {
                      setEditFormData((prev) => ({
                        ...prev,
                        description: e.target.value,
                      }));
                      setTextSaveStatus((current) => ({
                        ...current,
                        description: "idle",
                      }));
                    }}
                    onBlur={() => void handleTextFieldBlur("description")}
                    onKeyDown={(event) => {
                      if (event.key === "Escape") {
                        handleTextFieldCancel("description");
                      }
                    }}
                    error={textSaveStatus.description === "error"}
                    helperText={getTextSaveHelper("description")}
                    placeholder="Task description"
                    sx={taskDetailsTextFieldSx}
                  />
                </Box>
              </ClickAwayListener>
            ) : (
              <Box
                role={isTaskDetailsEditable ? "button" : undefined}
                tabIndex={isTaskDetailsEditable ? 0 : undefined}
                onClick={() => handleTextFieldEdit("description")}
                onKeyDown={(event) => {
                  if (event.key === "Enter" || event.key === " ") {
                    event.preventDefault();
                    handleTextFieldEdit("description");
                  }
                }}
                sx={{
                  cursor: isTaskDetailsEditable ? "text" : "default",
                  borderRadius: 1,
                  mx: -1,
                  px: 1,
                  py: 0.5,
                  transition: "background-color 0.15s ease",
                  "&:hover": isTaskDetailsEditable
                    ? {
                        backgroundColor: "action.hover",
                      }
                    : {},
                }}
              >
                <Typography
                  variant="body2"
                  sx={{
                    ...taskDetailsTextSx,
                    whiteSpace: "pre-wrap",
                    wordBreak: "break-word",
                    overflow: "visible",
                  }}
                >
                  {textSaveStatus.description === "saving" ||
                  textSaveStatus.description === "saved"
                    ? editFormData.description
                    : task?.description ||
                      task?.original_prompt ||
                      "No description provided"}
                </Typography>
              </Box>
            )}
          </Box>

          {task?.id && (
            <TaskAttachmentsPanel
              taskId={task.id}
              status={task.status as TypesSpecTaskStatus}
            />
          )}

          {/* Share preview URLs — only meaningful once a session exists */}
          {activeSessionId && (
            <Box
              sx={{
                ...taskDetailsSectionSx,
                "& > .MuiBox-root": { mb: 0 },
              }}
            >
              <SharePreviewSection sessionId={activeSessionId} />
            </Box>
          )}
        </Box>

        <Box
          sx={{
            display: "flex",
            flexDirection: "column",
            gap: 2,
            minWidth: 0,
          }}
        >
          <Box sx={{ ...taskDetailsSectionSx, p: 1.5 }}>
            <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: 1.5 }}>
              Task setup
            </Typography>

      {/* Priority */}
      <Box sx={{ mb: 2 }}>
        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
          Priority
        </Typography>
        <Chip
          label={task?.priority || "Medium"}
          color={getPriorityColor(task?.priority)}
          size="small"
        />
      </Box>

      {/* Labels */}
      <Box sx={{ mb: 2 }}>
        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
          Labels
        </Typography>
        <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mb: 1 }}>
          {(task?.labels || []).map((label) => (
            <Chip
              key={label}
              label={label}
              size="small"
              onDelete={() =>
                removeLabelMutation.mutate({ taskId, label })
              }
            />
          ))}
        </Box>
        <Autocomplete
          freeSolo
          options={projectLabels.filter(
            (l) => !(task?.labels || []).includes(l),
          )}
          inputValue={labelInput}
          onInputChange={(_, value) => setLabelInput(value)}
          filterOptions={(options, params) => {
            const filtered = options.filter((o) =>
              o.toLowerCase().includes(params.inputValue.toLowerCase()),
            );
            const trimmed = params.inputValue.trim();
            if (
              trimmed &&
              !options.some((o) => o.toLowerCase() === trimmed.toLowerCase())
            ) {
              filtered.push(`__create__:${trimmed}`);
            }
            return filtered;
          }}
          onChange={(_, value) => {
            if (value && typeof value === "string") {
              const label = value.startsWith("__create__:")
                ? value.slice("__create__:".length)
                : value.trim();
              if (label) {
                addLabelMutation.mutate({ taskId, label });
                setLabelInput("");
              }
            }
          }}
          getOptionLabel={(option) =>
            option.startsWith("__create__:")
              ? option.slice("__create__:".length)
              : option
          }
          renderOption={(props, option) => {
            if (option.startsWith("__create__:")) {
              const label = option.slice("__create__:".length);
              return (
                <li {...props} key="__create__">
                  <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
                    <Typography variant="body2" color="primary">
                      + Create &ldquo;{label}&rdquo;
                    </Typography>
                  </Box>
                </li>
              );
            }
            return <li {...props} key={option}>{option}</li>;
          }}
          renderInput={(params) => (
            <TextField
              {...params}
              size="small"
              placeholder="Add label..."
            />
          )}
        />
      </Box>

      <Box sx={{ mb: 2 }}>
        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
          Depends on
        </Typography>
        {currentTaskDependencies.length > 0 ? (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.75 }}>
            {currentTaskDependencies.map((dependencyTask) => (
              <Chip
                key={dependencyTask.id}
                size="small"
                clickable={!!dependencyTask.id && !!task?.project_id}
                onClick={() => {
                  if (!dependencyTask.id || !task?.project_id) {
                    return;
                  }
                  account.orgNavigate("project-task-detail", {
                    id: task.project_id,
                    taskId: dependencyTask.id,
                  });
                }}
                label={
                  dependencyTask.name ||
                  dependencyTask.short_title ||
                  `Task #${dependencyTask.task_number || "?"}`
                }
              />
            ))}
          </Box>
        ) : (
          <Typography variant="body2" color="text.secondary">
            No task dependencies
          </Typography>
        )}
      </Box>

      {/* Assignee */}
      <Box sx={{ mb: 2 }}>
        <Typography variant="subtitle2" color="text.secondary" gutterBottom>
          Assignee
        </Typography>
        <Button
          variant="outlined"
          fullWidth
          disabled={updateSpecTask.isPending || !isTaskDetailsEditable}
          onClick={(event) => setAssigneeAnchorEl(event.currentTarget)}
          sx={{
            justifyContent: "flex-start",
            textTransform: "none",
            color: "text.primary",
            minHeight: 40,
            px: 1.25,
            gap: 1,
          }}
        >
          <OrganizationUserAvatar
            userId={task?.assignee_id}
            members={orgMembers}
            currentUser={account.user}
            size={24}
            fontSize="0.7rem"
            iconSize={20}
          />
          <Typography variant="body2" noWrap>
            {assignedUser?.full_name ||
              assignedUser?.username ||
              assignedUser?.email ||
              "Unassigned"}
          </Typography>
        </Button>
        <AssigneeSelector
          assigneeId={task?.assignee_id}
          members={orgMembers}
          currentUser={account.user}
          onAssigneeChange={handleAssigneeChange}
          isLoading={updateSpecTask.isPending}
          anchorEl={assigneeAnchorEl}
          onClose={() => setAssigneeAnchorEl(null)}
        />
      </Box>

      {/* Agent Selection */}
      <Box sx={{ mb: 2 }}>
        <AgentDropdown
          value={selectedAgent}
          onChange={handleAgentChange}
          agents={eligibleApps}
          label="Agent"
          disabled={updatingAgent}
          size="small"
        />
      </Box>

      {/* Timestamps */}
      <Box sx={{ mt: 2 }}>
        {task?.created_by && (
          <Typography variant="caption" color="text.secondary" display="block">
            Author: {authorDisplay}
          </Typography>
        )}
        <Typography variant="caption" color="text.secondary" display="block">
          Created:{" "}
          {task?.created_at
            ? new Date(task.created_at).toLocaleString()
            : "N/A"}
        </Typography>
        <Typography variant="caption" color="text.secondary" display="block">
          Updated:{" "}
          {task?.updated_at
            ? new Date(task.updated_at).toLocaleString()
            : "N/A"}
        </Typography>
      </Box>

      {/* Clone Info - Bidirectional links */}
      {(task?.cloned_from_id || (cloneGroups && cloneGroups.length > 0)) && (
        <Box sx={{ mt: 2 }}>
          <Typography variant="subtitle2" color="text.secondary" gutterBottom>
            Clone Info
          </Typography>

          {/* Link to source task and batch progress if this was cloned */}
          {task?.cloned_from_id && (
            <Box sx={{ display: "flex", gap: 1, mb: 1 }}>
              <Box
                sx={{
                  flex: 1,
                  display: "flex",
                  alignItems: "center",
                  gap: 1,
                  p: 1,
                  bgcolor: "action.hover",
                  borderRadius: 1,
                  cursor: "pointer",
                  "&:hover": { bgcolor: "action.selected" },
                }}
                onClick={() => {
                  if (task.cloned_from_project_id && task.cloned_from_id) {
                    account.orgNavigate("project-task-detail", {
                      id: task.cloned_from_project_id,
                      taskId: task.cloned_from_id,
                    });
                  }
                }}
              >
                <LinkIcon sx={{ fontSize: 16, color: "text.secondary" }} />
                <Typography variant="caption" color="text.secondary">
                  Cloned from another task
                </Typography>
              </Box>
              {task.clone_group_id && (
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    p: 1,
                    bgcolor: "action.hover",
                    borderRadius: 1,
                    cursor: "pointer",
                    "&:hover": { bgcolor: "action.selected" },
                  }}
                  onClick={() =>
                    setSelectedCloneGroupId(task.clone_group_id || null)
                  }
                >
                  <AccountTree
                    sx={{ fontSize: 16, color: "inherit", opacity: 0.7 }}
                  />
                  <Typography variant="caption" color="text.secondary">
                    Batch Progress
                  </Typography>
                </Box>
              )}
            </Box>
          )}

          {/* Links to clone groups if this task was cloned to others */}
          {cloneGroups && cloneGroups.length > 0 && (
            <Box>
              <Typography
                variant="caption"
                color="text.secondary"
                display="block"
                sx={{ mb: 0.5 }}
              >
                Cloned to {cloneGroups.length} batch
                {cloneGroups.length > 1 ? "es" : ""}:
              </Typography>
              {cloneGroups.map((group) => (
                <Box
                  key={group.id}
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    p: 1,
                    bgcolor: "action.hover",
                    borderRadius: 1,
                    cursor: "pointer",
                    mb: 0.5,
                    "&:hover": { bgcolor: "action.selected" },
                  }}
                  onClick={() => setSelectedCloneGroupId(group.id || null)}
                >
                  <Wand2 size={16} style={{ color: "inherit", opacity: 0.7 }} />
                  <Typography variant="caption">
                    {group.total_targets} project
                    {group.total_targets !== 1 ? "s" : ""} •{" "}
                    {new Date(group.created_at || "").toLocaleDateString()}
                  </Typography>
                </Box>
              ))}
            </Box>
          )}
        </Box>
      )}

          </Box>

      {/* Debug Info */}
      <Box
        component="details"
        sx={{
          ...taskDetailsSectionSx,
          p: 0,
          overflow: "hidden",
          bgcolor: lightTheme.isLight ? "grey.50" : "grey.900",
          "&[open] > summary": {
            borderBottom: "1px solid",
            borderColor: "divider",
          },
        }}
      >
        <Box
          component="summary"
          sx={{
            px: 1.5,
            py: 1.25,
            cursor: "pointer",
            color: "text.secondary",
            fontSize: "0.8125rem",
            fontWeight: 600,
            userSelect: "none",
            "&:hover": { color: "text.primary" },
          }}
        >
          Debug information
        </Box>
        <Box sx={{ p: 1.5 }}>
        <Typography
          variant="caption"
          color={lightTheme.isLight ? "grey.800" : "grey.300"}
          sx={{ fontFamily: "monospace", display: "block" }}
        >
          Task ID: {task?.id || "N/A"}
        </Typography>
        <Typography
          variant="caption"
          color={lightTheme.isLight ? "grey.800" : "grey.300"}
          sx={{ fontFamily: "monospace", display: "block" }}
        >
          Task #:{" "}
          {task?.task_number
            ? `#${String(task.task_number).padStart(6, "0")}`
            : "N/A"}
        </Typography>
        {task?.branch_name && (
          <Tooltip title="Spectask branches push changes to upstream repository">
            <Typography
              variant="caption"
              color={lightTheme.isLight ? "grey.800" : "grey.300"}
              sx={{ fontFamily: "monospace", display: "block" }}
            >
              Branch: {task.branch_name}{" "}
              <Box component="span" sx={{ color: "success.main" }}>
                → PUSH
              </Box>
            </Typography>
          </Tooltip>
        )}
        {task?.base_branch && task.base_branch !== task.branch_name && (
          <Tooltip title="Base branch pulls updates from upstream repository">
            <Typography
              variant="caption"
              color={lightTheme.isLight ? "grey.800" : "grey.300"}
              sx={{ fontFamily: "monospace", display: "block" }}
            >
              Base: {task.base_branch}{" "}
              <Box component="span" sx={{ color: "info.main" }}>
                ← PULL
              </Box>
            </Typography>
          </Tooltip>
        )}
        <Typography
          variant="caption"
          color={lightTheme.isLight ? "grey.800" : "grey.300"}
          sx={{ fontFamily: "monospace", display: "block" }}
        >
          Specs Folder: {task?.design_doc_path || "N/A"}
        </Typography>
        {activeSessionId && (
          <Typography
            variant="caption"
            color={lightTheme.isLight ? "grey.800" : "grey.300"}
            sx={{ fontFamily: "monospace", display: "block" }}
          >
            Session ID: {activeSessionId}
          </Typography>
        )}
        {sessionData?.config?.sway_version && (
          <Typography
            variant="caption"
            color={lightTheme.isLight ? "grey.800" : "grey.300"}
            sx={{ fontFamily: "monospace", display: "block" }}
          >
            Desktop: {sessionData.config.sway_version}
          </Typography>
        )}
        {sessionData?.config?.gpu_vendor && (
          <Typography
            variant="caption"
            color={lightTheme.isLight ? "grey.800" : "grey.300"}
            sx={{ fontFamily: "monospace", display: "block" }}
          >
            GPU: {sessionData.config.gpu_vendor.toUpperCase()}
          </Typography>
        )}
        {sessionData?.config?.render_node && (
          <Typography
            variant="caption"
            color={lightTheme.isLight ? "grey.800" : "grey.300"}
            sx={{ fontFamily: "monospace", display: "block" }}
          >
            Render: {sessionData.config.render_node}
          </Typography>
        )}
        </Box>
      </Box>

        <Box sx={{ ...taskDetailsSectionSx, p: 1.5 }}>
        {/* Share Design Docs */}
        <Box
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
            flexWrap: "wrap",
            gap: 1,
            mb: 0.5,
          }}
        >
          <Box>
            <Typography variant="subtitle2">Share Design Docs</Typography>
            <Typography variant="caption" color="text.secondary">
              {isPublicDesignDocs
                ? "Anyone with the link can view"
                : "Only people with project access can view"}
            </Typography>
          </Box>
          <Button
            size="small"
            variant="outlined"
            startIcon={<Share size={14} />}
            onClick={() => setShareDialogOpen(true)}
            sx={{ ...taskActionButtonSx, ml: "auto" }}
          >
            Share
          </Button>
        </Box>

        {/* Move to Backlog button */}
        {canMoveToBacklog && (
          <Box sx={{ mt: 2, display: "flex", justifyContent: "flex-end" }}>
            <Button
              size="small"
              variant="outlined"
              color="warning"
              startIcon={
                moveToBacklogMutation.isPending ? (
                  <CircularProgress size={14} color="inherit" />
                ) : (
                  <UndoIcon />
                )
              }
              onClick={() => moveToBacklogMutation.mutate()}
              disabled={moveToBacklogMutation.isPending}
              sx={taskActionButtonSx}
            >
              {moveToBacklogMutation.isPending
                ? "Moving..."
                : "Move to Backlog"}
            </Button>
          </Box>
        )}

        {/* Archive button */}
        <Box sx={{ mt: 2, display: "flex", justifyContent: "flex-end" }}>
          <Tooltip title="Hold Shift to skip confirmation">
            <Button
              size="small"
              variant="outlined"
              color="error"
              startIcon={
                isArchiving ? (
                  <CircularProgress size={14} color="inherit" />
                ) : (
                  <ArchiveIcon />
                )
              }
              onClick={handleArchiveClick}
              disabled={isArchiving || task?.archived}
              sx={taskActionButtonSx}
            >
              {isArchiving ? "Archiving..." : "Archive Task"}
            </Button>
          </Tooltip>
        </Box>
      </Box>
        </Box>
      </Box>
    </Box>
  );

  const taskChatMetadata = task?.project_id ? (
    <TaskChatMetadata
      projectName={project?.name}
      onOpenProject={() => account.orgNavigate("project-specs", { id: task.project_id })}
      primaryRepository={primaryRepository}
      branchName={task.branch_name}
      pullRequests={task.repo_pull_requests}
    />
  ) : undefined;

  const terminalToggleButton = activeSessionId ? (
    <Tooltip title="Toggle terminal drawer (Ctrl/Cmd+J)">
      <IconButton
        size="small"
        onClick={toggleTerminalDrawer}
        sx={taskToolbarIconButtonSx}
        aria-label="Toggle terminal drawer"
        aria-pressed={terminalDrawerState.open}
      >
        <PanelBottom size={18} />
      </IconButton>
    </Tooltip>
  ) : null;

  if (!task) {
    return (
      <Box
        sx={{
          display: "flex",
          alignItems: "center",
          justifyContent: "center",
          height: "100%",
        }}
      >
        <CircularProgress />
      </Box>
    );
  }

  return (
    <Box
      sx={{
        display: "flex",
        flexDirection: "column",
        height: "100%",
        overflow: "hidden",
      }}
    >
      {/* Hidden file input for upload */}
      <input
        type="file"
        ref={fileInputRef}
        onChange={handleFileChange}
        style={{ display: "none" }}
        multiple
      />

      {/* Claude subscription token expiry warning */}
      {claudeTokenExpiry &&
        (claudeTokenExpiry.isExpired || claudeTokenExpiry.isExpiringSoon) && (
          <Alert
            severity="warning"
            sx={{ mx: 1, mt: 1, flexShrink: 0 }}
            action={
              <ClaudeSubscriptionConnect
                variant="button"
                orgId={project?.organization_id}
              />
            }
          >
            {claudeTokenExpiry.isExpired
              ? `Claude token expired (${claudeTokenExpiry.label}). It will auto-refresh next time a session uses Claude Code, or re-authenticate now.`
              : `Claude token expiring soon (${claudeTokenExpiry.label}). It will auto-refresh next time a session uses Claude Code, or re-authenticate now.`}
          </Alert>
        )}

      {/* Tab Content */}
      <Box
        sx={{
          flex: 1,
          overflow: "hidden",
          display: "flex",
          flexDirection: "column",
          px: padContent ? { xs: 0, sm: 3 } : 0,
        }}
      >
        {/* Desktop layout: left panel (chat always visible) + right panel (content toggleable) */}
        {/* When chatCollapsed is true, use mobile-style tab layout even on desktop */}
        {launchPhase ? (
          <SpecTaskLaunchWindow
            phase={launchPhase}
            mode={task.just_do_it_mode ? "implementation" : "planning"}
            queueReason={task.queue_reason}
            onMoveToBacklog={
              launchPhase === "queued"
                ? () => moveToBacklogMutation.mutate()
                : undefined
            }
            isMovingToBacklog={moveToBacklogMutation.isPending}
          />
        ) : activeSessionId && isBigScreen && !chatCollapsed ? (
          <PanelGroup
            key="spec-task-chat-layout"
            orientation="horizontal"
            defaultLayout={savedSpecTaskChatLayout ?? { "spec-task-chat": 50, "spec-task-content": 50 }}
            onLayoutChange={(layout) => {
              // A collapsed panel is transient UI state; retain the last useful
              // split so restoring or reloading does not produce a zero-width pane.
              if (layout["spec-task-chat"] > 0 && layout["spec-task-content"] > 0) {
                lastExpandedContentSizeRef.current = layout["spec-task-content"];
                savePanelLayout(SPEC_TASK_CHAT_LAYOUT_KEY, layout, SPEC_TASK_CHAT_PANEL_IDS);
              }
            }}
            style={{ height: "100%", flex: 1 }}
          >
            {/* Left: Chat panel - always visible on desktop */}
            <Panel id="spec-task-chat" defaultSize="50%" minSize="15%" style={{ overflow: "hidden" }}>
              <Box
                sx={{
                  height: "100%",
                  display: "flex",
                  flexDirection: "column",
                  minHeight: 0,
                }}
              >
                {/* Chat panel header with collapse button */}
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    px: 1,
                    minHeight: 48,
                    borderBottom: "1px solid",
                    borderColor: "divider",
                    backgroundColor: "background.paper",
                    flexShrink: 0,
                  }}
                >
                  <Box sx={{ display: "flex", alignItems: "center", gap: 1, flex: 1, minWidth: 0 }}>
                    {/* Spec is the only alternate view; the surrounding panel is already chat. */}
                    {task?.design_docs_pushed_at && (
                      <ToggleButtonGroup
                        value={null}
                        exclusive
                        onChange={(_, val) => {
                          if (val === "spec") handleReviewSpec();
                        }}
                        size="small"
                        sx={{
                          flexShrink: 0,
                          "& .MuiToggleButton-root": {
                            px: 1.25,
                            py: 0.25,
                            fontSize: "0.8rem",
                            fontWeight: 500,
                            textTransform: "none",
                            border: "1px solid",
                            borderColor: "divider",
                            color: "text.secondary",
                            "&.Mui-selected": {
                              color: "text.primary",
                              backgroundColor: "action.selected",
                            },
                          },
                        }}
                      >
                        <ToggleButton value="spec" disableRipple={false}>
                          <FileText size={14} style={{ marginRight: 4 }} />
                          Spec
                        </ToggleButton>
                      </ToggleButtonGroup>
                    )}
                    {/* Thread selector (shown alongside tabs when multiple threads exist) */}
                    {(() => {
                      // Filter out threads that point to the same session as planning (they're the same conversation)
                      const extraThreads = zedThreadsData?.zed_threads?.filter(
                        t => t.work_session?.helix_session_id && t.work_session.helix_session_id !== task?.planning_session_id
                      ) || [];
                      if (extraThreads.length === 0) return null;
                      return (
                        <Select
                          size="small"
                          variant="standard"
                          value={selectedThreadSessionId || "planning"}
                          onChange={(e) => {
                            const val = e.target.value as string;
                            setSelectedThreadSessionId(
                              val === "planning" ? null : val,
                            );
                          }}
                          sx={{
                            fontSize: "0.8rem",
                            fontWeight: 500,
                            color: "text.secondary",
                            minWidth: 80,
                            maxWidth: 140,
                            ml: 1,
                            "&:before": { display: "none" },
                            "&:after": { display: "none" },
                            "& .MuiSelect-select": { py: 0 },
                          }}
                        >
                          <MenuItem value="planning">Main thread</MenuItem>
                          {extraThreads.map((thread, index) => {
                            const sessionId = thread.work_session?.helix_session_id;
                            if (!sessionId) return null;
                            const label =
                              thread.work_session?.name ||
                              thread.work_session?.implementation_task_title ||
                              `Thread ${index + 2}`;
                            return (
                              <MenuItem key={sessionId} value={sessionId}>
                                {label}
                              </MenuItem>
                            );
                          })}
                        </Select>
                      );
                    })()}
                  </Box>
                  {contentCollapsed ? (
                    <Tooltip title="Show task panel">
                      <IconButton
                        size="small"
                        aria-label="Show task panel"
                        onClick={showContentPanel}
                        sx={taskToolbarIconButtonSx}
                      >
                        <PanelRight size={18} />
                      </IconButton>
                    </Tooltip>
                  ) : (
                    <Tooltip title="Collapse chat panel">
                      <IconButton
                        size="small"
                        aria-label="Collapse chat panel"
                        onClick={() => {
                          setChatCollapsed(true);
                          // Switch to desktop view when collapsing chat
                          if (currentView === "chat") {
                            handleViewChange("desktop");
                          }
                        }}
                        sx={taskToolbarIconButtonSx}
                      >
                        <PanelLeft size={18} />
                      </IconButton>
                    </Tooltip>
                  )}
                </Box>
                <AgentChat
                  sessionId={activeSessionId}
                  specTaskId={task.id}
                  projectId={task.project_id}
                  enableInteractionDebugCopy
                  onWillSend={handleWillSend}
                  leadingActions={(
                    <SwitchAgentControl sessionId={activeSessionId} displayMode="compact" />
                  )}
                  footerContent={taskChatMetadata}
                  placeholder={
                    sessionData?.config?.paused
                      ? "This session is paused — open the forked child to keep chatting"
                      : "Send message to agent..."
                  }
                  disabled={!!sessionData?.config?.paused}
                />
              </Box>
            </Panel>

            {/* Resize handle */}
            <PanelResizeHandle
              style={{
                width: contentCollapsed ? 0 : 6,
                background: lightTheme.isLight ? 'rgba(0, 0, 0, 0.06)' : 'rgba(255, 255, 255, 0.08)',
                cursor: contentCollapsed ? "default" : "col-resize",
                transition: "background 0.15s",
                overflow: "hidden",
              }}
            >
              <div
                style={{
                  width: 2,
                  height: "100%",
                  margin: "0 auto",
                  background: lightTheme.isLight ? 'rgba(0, 0, 0, 0.12)' : 'rgba(255, 255, 255, 0.12)',
                  borderRadius: 1,
                }}
              />
            </PanelResizeHandle>

            {/* Right: Content panel - switches between desktop/changes/details */}
            <Panel
              id="spec-task-content"
              defaultSize="50%"
              minSize="25%"
              collapsible={allowContentCollapse}
              collapsedSize={0}
              panelRef={contentPanelRef}
              onResize={(size) => setContentCollapsed(size.asPercentage === 0)}
              style={{ overflow: "hidden" }}
            >
              <Box
                sx={{
                  height: "100%",
                  display: "flex",
                  flexDirection: "column",
                  overflow: "hidden",
                }}
              >
                {/* View toggle header - above content area only */}
                <Box
                  sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    px: 1,
                    pt: 1,
                    pb: 0.5,
                    minHeight: 53,
                    flexShrink: 0,
                    boxSizing: "border-box",
                    borderBottom: "1px solid",
                    borderColor: "divider",
                    backgroundColor: "background.paper",
                    gap: 1,
                  }}
                >
                  {/* Left: View toggle icons */}
                  <ToggleButtonGroup
                    value={currentView}
                    exclusive
                    onChange={(_, newView) => handleViewChange(newView)}
                    size="small"
                    sx={{
                      "& .MuiToggleButton-root": {
                        width: 56,
                        height: 40,
                        minWidth: 56,
                        p: 0,
                        border: "none",
                        borderRadius: "4px !important",
                        textTransform: "none",
                        display: "flex",
                        flexDirection: "column",
                        alignItems: "center",
                        gap: 0.2,
                        "&.Mui-selected": {
                          backgroundColor: "action.selected",
                        },
                      },
                    }}
                  >
                    <ToggleButton value="desktop" aria-label="Desktop view">
                      <MonitorPlay size={18} />
                      <Typography
                        sx={{
                          fontSize: "0.65rem",
                          lineHeight: 1,
                          fontWeight: 400,
                          textTransform: "none",
                        }}
                      >
                        Desktop
                      </Typography>
                    </ToggleButton>
                    <ToggleButton value="browser" aria-label="Browser view">
                      <Globe2 size={18} />
                      <Typography
                        sx={{
                          fontSize: "0.65rem",
                          lineHeight: 1,
                          fontWeight: 400,
                          textTransform: "none",
                        }}
                      >
                        Browser
                      </Typography>
                    </ToggleButton>
                    <ToggleButton value="changes" aria-label="Diff view">
                      <GitCompare size={18} />
                      <Typography
                        sx={{
                          fontSize: "0.65rem",
                          lineHeight: 1,
                          fontWeight: 400,
                          textTransform: "none",
                        }}
                      >
                        Diff
                      </Typography>
                    </ToggleButton>
                    <ToggleButton value="files" aria-label="Files view">
                      <Files size={18} />
                      <Typography
                        sx={{
                          fontSize: "0.65rem",
                          lineHeight: 1,
                          fontWeight: 400,
                          textTransform: "none",
                        }}
                      >
                        Files
                      </Typography>
                    </ToggleButton>
                    <ToggleButton value="details" aria-label="Details view">
                      <SlidersHorizontal size={18} />
                      <Typography
                        sx={{
                          fontSize: "0.65rem",
                          lineHeight: 1,
                          fontWeight: 400,
                          textTransform: "none",
                        }}
                      >
                        Details
                      </Typography>
                    </ToggleButton>
                  </ToggleButtonGroup>

                  {/* Status-specific action buttons */}
                  {renderTaskActions("inline")}

                  {/* Spacer */}
                  <Box sx={{ flex: 1 }} />

                  {/* Right: Action buttons */}
                  <Box sx={{ display: "flex", gap: 0.5, alignItems: "center" }}>
                    <>
                        {terminalToggleButton}
                        {/* Show Start button when desktop is paused */}
                        {effectiveIsDesktopPaused && (
                          <Tooltip title="Start desktop">
                            <IconButton
                              size="small"
                              aria-label="Start desktop"
                              onClick={handleStartSession}
                              disabled={isStarting || isDesktopStarting}
                              sx={taskToolbarIconButtonSx}
                            >
                              {isStarting || isDesktopStarting ? (
                                <CircularProgress size={16} />
                              ) : (
                                <PlayLucide size={18} />
                              )}
                            </IconButton>
                          </Tooltip>
                        )}
                        {/* Show Stop button when desktop is running */}
                        {isDesktopRunning && (
                          <Tooltip title="Stop desktop">
                            <IconButton
                              size="small"
                              aria-label="Stop desktop"
                              onClick={() => setStopConfirmOpen(true)}
                              disabled={isStopping}
                              sx={taskToolbarIconButtonSx}
                            >
                              {isStopping ? (
                                <CircularProgress size={16} />
                              ) : (
                                <Square size={18} fill="currentColor" />
                              )}
                            </IconButton>
                          </Tooltip>
                        )}
                        {/* Show Restart button only when desktop is running */}
                        {isDesktopRunning && (
                          <Tooltip title="Restart agent session">
                            <IconButton
                              size="small"
                              aria-label="Restart agent session"
                              onClick={() => setRestartConfirmOpen(true)}
                              disabled={isRestarting}
                              sx={taskToolbarIconButtonSx}
                            >
                              {isRestarting ? (
                                <CircularProgress size={16} />
                              ) : (
                                <RotateCw size={18} />
                              )}
                            </IconButton>
                          </Tooltip>
                        )}
                        {/* Show Keep Alive toggle when desktop is running */}
                        {isDesktopRunning && (
                          <Tooltip
                            title={
                              task.keep_alive
                                ? "Keep Alive ON — won't auto-sleep"
                                : "Keep Alive OFF — will auto-sleep when idle"
                            }
                          >
                            <IconButton
                              size="small"
                              aria-label={task.keep_alive ? "Disable keep alive" : "Enable keep alive"}
                              onClick={handleToggleKeepAlive}
                              disabled={updateSpecTask.isPending}
                              sx={taskToolbarIconButtonSx}
                              aria-pressed={task.keep_alive}
                            >
                              {task.keep_alive ? (
                                <LockLucide size={18} />
                              ) : (
                                <LockOpenLucide size={18} />
                              )}
                            </IconButton>
                          </Tooltip>
                        )}
                        {/* Show Upload button only when desktop is running */}
                        {isDesktopRunning && (
                          <Tooltip title="Upload files to sandbox">
                            <IconButton
                              size="small"
                              aria-label="Upload files to sandbox"
                              onClick={handleUploadClick}
                              disabled={isUploading}
                              sx={taskToolbarIconButtonSx}
                            >
                              {isUploading ? (
                                <CircularProgress size={16} />
                              ) : (
                                <CloudUploadLucide size={18} />
                              )}
                            </IconButton>
                          </Tooltip>
                        )}
                        {(task.design_docs_pushed_at || task.clone_group_id) && (
                          <Tooltip title="More actions">
                            <IconButton
                              size="small"
                              aria-label="More actions"
                              onClick={(event) =>
                                setActionMenuAnchorEl(event.currentTarget)
                              }
                              sx={taskToolbarIconButtonSx}
                            >
                              <EllipsisVertical size={18} />
                            </IconButton>
                          </Tooltip>
                        )}
                    </>
                    {allowContentCollapse ? (
                      <Tooltip title="Collapse task panel">
                        <IconButton
                          size="small"
                          aria-label="Collapse task panel"
                          onClick={collapseContentPanel}
                          sx={taskToolbarIconButtonSx}
                        >
                          <PanelRight size={18} />
                        </IconButton>
                      </Tooltip>
                    ) : onClose ? (
                      <IconButton
                        size="small"
                        onClick={onClose}
                        sx={taskToolbarIconButtonSx}
                      >
                        <X size={18} />
                      </IconButton>
                    ) : null}
                  </Box>
                </Box>

                {/* In split-view layout, "chat" falls through to desktop since chat
                    is already visible in the left panel */}
                {(currentView === "desktop" || currentView === "chat") &&
                  (isTaskCompleted && isDesktopPaused ? (
                    <TaskSessionPlaceholder
                      tone="finished"
                      title="Task finished"
                      description="This task has been merged to the default branch. Its sandbox is stopped."
                      onStart={handleStartSession}
                      starting={isStarting || isDesktopStarting}
                    />
                  ) : isTaskArchived ? (
                    <TaskSessionPlaceholder
                      tone="archived"
                      title="Task rejected"
                      description="This task has been archived. The agent session has ended."
                    />
                  ) : (
                    <ExternalAgentDesktopViewer
                      sessionId={activeSessionId}
                      sandboxId={activeSessionId}
                      mode="stream"
                      onClientIdCalculated={setClientUniqueId}
                      displayWidth={displaySettings.width}
                      displayHeight={displaySettings.height}
                      displayFps={displaySettings.fps}
                      startupErrorMessage={desktopStartupMessage}
                      connectSubscriptionLabel={connectSubscriptionLabel}
                      onConnectSubscription={connectSubscription}
                      initialSandboxState={isQueuedForPlanning ? "starting" : undefined}
                    />
                  ))}
                {currentView === "browser" &&
                  (isTaskArchived ? (
                    <TaskSessionPlaceholder
                      tone="archived"
                      title="Task rejected"
                      description="This task has been archived. The agent session has ended."
                    />
                  ) : effectiveIsDesktopPaused ? (
                    <TaskSessionPlaceholder
                      tone={isTaskCompleted ? "finished" : "paused"}
                      title={isTaskCompleted ? "Task finished" : "Desktop not running"}
                      description={
                        isTaskCompleted
                          ? "This task has been merged to the default branch. Start the desktop to preview its web apps."
                          : "Start the desktop to preview a localhost web app from this sandbox."
                      }
                      detail={desktopStartupMessage}
                      onStart={handleStartSession}
                      starting={isStarting || isDesktopStarting}
                    />
                  ) : (
                    <SandboxBrowser sessionId={activeSessionId} />
                  ))}
                {(currentView === "changes" || currentView === "files") && (
                  <DiffViewer
                    sessionId={activeSessionId}
                    baseBranch={defaultBranchName}
                    pollInterval={3000}
                    primarySurface={currentView === "files" ? "files" : "changes"}
                    onPrimarySurfaceChange={handleViewChange}
                    onStartDesktop={handleStartSession}
                    connectSubscriptionLabel={connectSubscriptionLabel}
                    onConnectSubscription={connectSubscription}
                    desktopUnavailableDetail={desktopStartupMessage}
                    desktopRunning={!effectiveIsDesktopPaused}
                    isDesktopStarting={isStarting || isDesktopStarting}
                    desktopUnavailableTitle={isTaskCompleted ? "Task finished" : undefined}
                    desktopUnavailableDescription={
                      isTaskCompleted
                        ? "This task has been merged to the default branch. Start the desktop to review its workspace."
                        : undefined
                    }
                  />
                )}
                {currentView === "details" && (
                  <Box
                    sx={{
                      flex: 1,
                      overflow: "auto",
                      p: { xs: 1.5, sm: 2 },
                    }}
                  >
                    {renderDetailsContent()}
                  </Box>
                )}
              </Box>
            </Panel>
          </PanelGroup>
        ) : (
          <>
            {/* Mobile layout OR no active session: single view at a time */}
            {/* A toolbar is useful only when there are multiple session views. */}
            {activeSessionId && (
              <Box
                sx={{
                display: "flex",
                alignItems: "center",
                justifyContent: "space-between",
                flexWrap: "wrap",
                px: 1,
                pt: 1,
                pb: 0.5,
                minHeight: 53,
                flexShrink: 0,
                boxSizing: "border-box",
                borderBottom: "1px solid",
                borderColor: "divider",
                backgroundColor: "background.paper",
                gap: 0.5,
                }}
              >
              {/* Left: View toggle icons */}
              <ToggleButtonGroup
                value={currentView}
                exclusive
                onChange={(_, newView) => handleViewChange(newView)}
                size="small"
                sx={{
                  flexShrink: 0,
                  "& .MuiToggleButton-root": {
                    py: 0.35,
                    px: 0.7,
                    minWidth: 56,
                    border: "none",
                    borderRadius: "4px !important",
                    textTransform: "none",
                    display: "flex",
                    flexDirection: "column",
                    alignItems: "center",
                    gap: 0.15,
                    "&.Mui-selected": {
                      backgroundColor: "action.selected",
                    },
                  },
                }}
              >
                {/* Chat tab - only on mobile when there's an active session */}
                {activeSessionId && (
                  <ToggleButton value="chat" aria-label="Chat view">
                    <MessageSquare size={18} />
                    <Typography
                      sx={{
                        fontSize: "0.65rem",
                        lineHeight: 1,
                        fontWeight: 400,
                        textTransform: "none",
                      }}
                    >
                      Chat
                    </Typography>
                  </ToggleButton>
                )}
                {activeSessionId && (
                  <ToggleButton value="desktop" aria-label="Desktop view">
                    <MonitorPlay size={18} />
                    <Typography
                      sx={{
                        fontSize: "0.65rem",
                        lineHeight: 1,
                        fontWeight: 400,
                        textTransform: "none",
                      }}
                    >
                      Desktop
                    </Typography>
                  </ToggleButton>
                )}
                {activeSessionId && (
                  <ToggleButton value="browser" aria-label="Browser view">
                    <Globe2 size={18} />
                    <Typography
                      sx={{
                        fontSize: "0.65rem",
                        lineHeight: 1,
                        fontWeight: 400,
                        textTransform: "none",
                      }}
                    >
                      Browser
                    </Typography>
                  </ToggleButton>
                )}
                {activeSessionId && (
                  <ToggleButton value="changes" aria-label="Diff view">
                    <GitCompare size={18} />
                    <Typography
                      sx={{
                        fontSize: "0.65rem",
                        lineHeight: 1,
                        fontWeight: 400,
                        textTransform: "none",
                      }}
                    >
                      Diff
                    </Typography>
                  </ToggleButton>
                )}
                {activeSessionId && (
                  <ToggleButton value="files" aria-label="Files view">
                    <Files size={18} />
                    <Typography
                      sx={{
                        fontSize: "0.65rem",
                        lineHeight: 1,
                        fontWeight: 400,
                        textTransform: "none",
                      }}
                    >
                      Files
                    </Typography>
                  </ToggleButton>
                )}
                <ToggleButton value="details" aria-label="Details view">
                  <SlidersHorizontal size={18} />
                  <Typography
                    sx={{
                      fontSize: "0.65rem",
                      lineHeight: 1,
                      fontWeight: 400,
                      textTransform: "none",
                    }}
                  >
                    Details
                  </Typography>
                </ToggleButton>
              </ToggleButtonGroup>

              {/* Status-specific action buttons */}
              {renderTaskActions("inline")}

              {/* Spacer - hidden on very small screens to allow wrapping */}
              <Box sx={{ flex: 1, minWidth: { xs: 0, sm: 8 } }} />

              {/* Right: Action buttons */}
              <Box
                sx={{
                  display: "flex",
                  gap: 0.25,
                  alignItems: "center",
                  flexShrink: 0,
                }}
              >
                <>
                    {isBigScreen && chatCollapsed && (
                      <Tooltip title="Restore split view">
                        <IconButton
                          size="small"
                          aria-label="Restore split view"
                          onClick={() => setChatCollapsed(false)}
                          sx={taskToolbarIconButtonSx}
                        >
                          <PanelLeft size={18} />
                        </IconButton>
                      </Tooltip>
                    )}
                    {terminalToggleButton}
                    {/* Show Start button when desktop is paused */}
                    {activeSessionId && effectiveIsDesktopPaused && (
                      <Tooltip title="Start desktop">
                        <IconButton
                          size="small"
                          aria-label="Start desktop"
                          onClick={handleStartSession}
                          disabled={isStarting || isDesktopStarting}
                          sx={taskToolbarIconButtonSx}
                        >
                          {isStarting || isDesktopStarting ? (
                            <CircularProgress size={16} />
                          ) : (
                            <PlayLucide size={18} />
                          )}
                        </IconButton>
                      </Tooltip>
                    )}
                    {/* Show Stop button when desktop is running */}
                    {activeSessionId && isDesktopRunning && (
                      <Tooltip title="Stop desktop">
                        <IconButton
                          size="small"
                          aria-label="Stop desktop"
                          onClick={() => setStopConfirmOpen(true)}
                          disabled={isStopping}
                          sx={taskToolbarIconButtonSx}
                        >
                          {isStopping ? (
                            <CircularProgress size={16} />
                          ) : (
                            <Square size={18} fill="currentColor" />
                          )}
                        </IconButton>
                      </Tooltip>
                    )}
                    {/* Show Restart button only when desktop is running */}
                    {activeSessionId && isDesktopRunning && (
                      <Tooltip title="Restart agent session">
                        <IconButton
                          size="small"
                          aria-label="Restart agent session"
                          onClick={() => setRestartConfirmOpen(true)}
                          disabled={isRestarting}
                          sx={taskToolbarIconButtonSx}
                        >
                          {isRestarting ? (
                            <CircularProgress size={16} />
                          ) : (
                            <RotateCw size={18} />
                          )}
                        </IconButton>
                      </Tooltip>
                    )}
                    {/* Show Upload button only when desktop is running */}
                    {activeSessionId && isDesktopRunning && (
                      <Tooltip title="Upload files to sandbox">
                        <IconButton
                          size="small"
                          aria-label="Upload files to sandbox"
                          onClick={handleUploadClick}
                          disabled={isUploading}
                          sx={taskToolbarIconButtonSx}
                        >
                          {isUploading ? (
                            <CircularProgress size={16} />
                          ) : (
                            <CloudUploadLucide size={18} />
                          )}
                        </IconButton>
                      </Tooltip>
                    )}
                    {(task.design_docs_pushed_at || task.clone_group_id) && (
                      <Tooltip title="More actions">
                        <IconButton
                          size="small"
                          aria-label="More actions"
                          onClick={(event) =>
                            setActionMenuAnchorEl(event.currentTarget)
                          }
                          sx={taskToolbarIconButtonSx}
                        >
                          <EllipsisVertical size={18} />
                        </IconButton>
                      </Tooltip>
                    )}
                </>
                {allowContentCollapse && isBigScreen && activeSessionId ? (
                  <Tooltip title="Collapse task panel">
                    <IconButton
                      size="small"
                      aria-label="Collapse task panel"
                      onClick={collapseContentPanel}
                      sx={taskToolbarIconButtonSx}
                    >
                      <PanelRight size={18} />
                    </IconButton>
                  </Tooltip>
                ) : onClose ? (
                  <IconButton
                    size="small"
                    onClick={onClose}
                    sx={taskToolbarIconButtonSx}
                  >
                    <X size={18} />
                  </IconButton>
                ) : null}
              </Box>
              </Box>
            )}

            {/* Chat View - mobile only */}
            {activeSessionId && currentView === "chat" && (
              <Box
                sx={{
                  flex: 1,
                  display: "flex",
                  flexDirection: "column",
                  minHeight: 0,
                  overflow: "hidden",
                }}
              >
                {(() => {
                  const extraThreads = zedThreadsData?.zed_threads?.filter(
                    t => t.work_session?.helix_session_id && t.work_session.helix_session_id !== task?.planning_session_id
                  ) || [];
                  if (extraThreads.length === 0) return null;
                  return (
                    <Box
                      sx={{
                        px: 1.5,
                        py: 0.5,
                        borderBottom: "1px solid",
                        borderColor: "divider",
                        flexShrink: 0,
                      }}
                    >
                      <Select
                        size="small"
                        variant="standard"
                        value={selectedThreadSessionId || "planning"}
                        onChange={(e) => {
                          const val = e.target.value as string;
                          setSelectedThreadSessionId(
                            val === "planning" ? null : val,
                          );
                        }}
                        sx={{
                          fontSize: "0.875rem",
                          fontWeight: 500,
                          color: "text.secondary",
                          minWidth: 100,
                          "&:before": { display: "none" },
                          "&:after": { display: "none" },
                          "& .MuiSelect-select": { py: 0 },
                        }}
                      >
                        <MenuItem value="planning">Main thread</MenuItem>
                        {extraThreads.map((thread, index) => {
                          const sessionId =
                            thread.work_session?.helix_session_id;
                          if (!sessionId) return null;
                          const label =
                            thread.work_session?.name ||
                            thread.work_session?.implementation_task_title ||
                            `Thread ${index + 2}`;
                          return (
                            <MenuItem key={sessionId} value={sessionId}>
                              {label}
                            </MenuItem>
                          );
                        })}
                      </Select>
                    </Box>
                  );
                })()}
                <AgentChat
                  sessionId={activeSessionId}
                  specTaskId={task.id}
                  projectId={task.project_id}
                  enableInteractionDebugCopy
                  onWillSend={handleWillSend}
                  leadingActions={(
                    <SwitchAgentControl sessionId={activeSessionId} displayMode="compact" />
                  )}
                  footerContent={taskChatMetadata}
                  placeholder={
                    sessionData?.config?.paused
                      ? "This session is paused — open the forked child to keep chatting"
                      : "Send message to agent..."
                  }
                  disabled={!!sessionData?.config?.paused}
                />
              </Box>
            )}

            {/* Desktop View - mobile */}
            {activeSessionId && currentView === "desktop" && (
              <Box
                sx={{
                  flex: 1,
                  display: "flex",
                  flexDirection: "column",
                  overflow: "hidden",
                }}
              >
                {isTaskCompleted && isDesktopPaused ? (
                  <TaskSessionPlaceholder
                    tone="finished"
                    title="Task finished"
                    description="This task has been merged to the default branch. Its sandbox is stopped."
                    onStart={handleStartSession}
                    starting={isStarting || isDesktopStarting}
                  />
                ) : isTaskArchived ? (
                  <TaskSessionPlaceholder
                    tone="archived"
                    title="Task rejected"
                    description="This task has been archived. The agent session has ended."
                  />
                ) : (
                  <ExternalAgentDesktopViewer
                    sessionId={activeSessionId}
                    sandboxId={activeSessionId}
                    mode="stream"
                    onClientIdCalculated={setClientUniqueId}
                    displayWidth={displaySettings.width}
                    displayHeight={displaySettings.height}
                    displayFps={displaySettings.fps}
                    startupErrorMessage={desktopStartupMessage}
                    connectSubscriptionLabel={connectSubscriptionLabel}
                    onConnectSubscription={connectSubscription}
                    initialSandboxState={isQueuedForPlanning ? "starting" : undefined}
                  />
                )}
              </Box>
            )}

            {/* Browser View - mobile */}
            {activeSessionId && currentView === "browser" && (
              <Box
                sx={{
                  flex: 1,
                  display: "flex",
                  flexDirection: "column",
                  minHeight: 0,
                  overflow: "hidden",
                }}
              >
                {isTaskArchived ? (
                  <TaskSessionPlaceholder
                    tone="archived"
                    title="Task rejected"
                    description="This task has been archived. The agent session has ended."
                  />
                ) : effectiveIsDesktopPaused ? (
                  <TaskSessionPlaceholder
                    tone={isTaskCompleted ? "finished" : "paused"}
                    title={isTaskCompleted ? "Task finished" : "Desktop not running"}
                    description={
                      isTaskCompleted
                        ? "This task has been merged to the default branch. Start the desktop to preview its web apps."
                        : "Start the desktop to preview a localhost web app from this sandbox."
                    }
                    detail={desktopStartupMessage}
                    onStart={handleStartSession}
                    starting={isStarting || isDesktopStarting}
                  />
                ) : (
                  <SandboxBrowser sessionId={activeSessionId} />
                )}
              </Box>
            )}

            {/* Workspace review - mobile */}
            {activeSessionId && (currentView === "changes" || currentView === "files") && (
              <Box sx={{ flex: 1, overflow: "hidden" }}>
                <DiffViewer
                  sessionId={activeSessionId}
                  baseBranch={defaultBranchName}
                  pollInterval={3000}
                  primarySurface={currentView === "files" ? "files" : "changes"}
                  onPrimarySurfaceChange={handleViewChange}
                  onStartDesktop={handleStartSession}
                  connectSubscriptionLabel={connectSubscriptionLabel}
                  onConnectSubscription={connectSubscription}
                  desktopUnavailableDetail={desktopStartupMessage}
                  desktopRunning={!effectiveIsDesktopPaused}
                  isDesktopStarting={isStarting || isDesktopStarting}
                  desktopUnavailableTitle={isTaskCompleted ? "Task finished" : undefined}
                  desktopUnavailableDescription={
                    isTaskCompleted
                      ? "This task has been merged to the default branch. Start the desktop to review its workspace."
                      : undefined
                  }
                />
              </Box>
            )}

            {/* Details View - mobile/no session */}
            {currentView === "details" && (
              <Box
                sx={{
                  flex: 1,
                  overflow: "auto",
                  p: { xs: 1.5, sm: 2 },
                }}
              >
                {renderDetailsContent()}
              </Box>
            )}
          </>
        )}
      </Box>

      {terminalDrawerState.open && activeSessionId && (
        <SpecTaskTerminalDrawer
          sessionId={activeSessionId}
          running={isDesktopRunning}
          height={terminalDrawerState.height}
          onHeightChange={setTerminalDrawerHeight}
          onClose={closeTerminalDrawer}
        />
      )}

      {/* Restart Session Confirmation */}
      <Dialog
        open={restartConfirmOpen}
        onClose={() => setRestartConfirmOpen(false)}
      >
        <DialogTitle>Restart Agent Session?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will stop the current agent container and start a fresh one.
            Any unsaved files in the sandbox may be lost.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setRestartConfirmOpen(false)}>Cancel</Button>
          <Button
            onClick={handleRestartSession}
            color="warning"
            variant="contained"
            disabled={isRestarting}
            startIcon={
              isRestarting ? <CircularProgress size={16} /> : <RestartAltIcon />
            }
          >
            Restart
          </Button>
        </DialogActions>
      </Dialog>

      {/* Stop Session Confirmation */}
      <Dialog open={stopConfirmOpen} onClose={() => setStopConfirmOpen(false)}>
        <DialogTitle>Stop Desktop?</DialogTitle>
        <DialogContent>
          <DialogContentText>
            This will stop the desktop environment. Any unsaved files in memory
            (e.g., IDE buffers) may be lost. You can start it again later.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => setStopConfirmOpen(false)}>Cancel</Button>
          <Button
            onClick={handleStopSession}
            color="error"
            variant="contained"
            disabled={isStopping}
            startIcon={
              isStopping ? <CircularProgress size={16} /> : <StopIcon />
            }
          >
            Stop
          </Button>
        </DialogActions>
      </Dialog>

      <Menu
        anchorEl={actionMenuAnchorEl}
        open={Boolean(actionMenuAnchorEl)}
        onClose={() => setActionMenuAnchorEl(null)}
      >
        {task?.design_docs_pushed_at && (
          <MenuItem
            onClick={() => {
              setActionMenuAnchorEl(null);
              setShareDialogOpen(true);
            }}
          >
            <ListItemIcon>
              <Share size={18} />
            </ListItemIcon>
            <ListItemText>Share Design Docs</ListItemText>
          </MenuItem>
        )}
        {task?.design_docs_pushed_at && (
          <MenuItem
            onClick={() => {
              setActionMenuAnchorEl(null);
              setShowCloneDialog(true);
            }}
          >
            <ListItemIcon>
              <Wand2 size={18} />
            </ListItemIcon>
            <ListItemText>Clone Task</ListItemText>
          </MenuItem>
        )}
        {task?.clone_group_id && (
          <MenuItem
            onClick={() => {
              setActionMenuAnchorEl(null);
              setSelectedCloneGroupId(task.clone_group_id || null);
            }}
          >
            <ListItemIcon>
              <AccountTree sx={{ fontSize: 18 }} />
            </ListItemIcon>
            <ListItemText>View Batch Clone Progress</ListItemText>
          </MenuItem>
        )}
      </Menu>

      {/* Clone Task Dialog */}
      <CloneTaskDialog
        open={showCloneDialog}
        onClose={() => setShowCloneDialog(false)}
        taskId={taskId}
        taskName={task?.name || ""}
        sourceProjectId={task?.project_id || ""}
      />

      <SpecTaskShareDialog
        open={shareDialogOpen}
        onClose={() => setShareDialogOpen(false)}
        shareUrl={publicLink}
        isPublic={isPublicDesignDocs}
        updating={updatingPublic}
        onToggle={handlePublicToggle}
      />

      {/* Clone Group Progress Dialog */}
      <Dialog
        open={selectedCloneGroupId !== null}
        onClose={() => setSelectedCloneGroupId(null)}
        maxWidth="md"
        fullWidth
      >
        <DialogTitle
          sx={{
            display: "flex",
            alignItems: "center",
            justifyContent: "space-between",
          }}
        >
          Clone Batch Progress
          <IconButton
            size="small"
            onClick={() => setSelectedCloneGroupId(null)}
          >
            <CloseIcon />
          </IconButton>
        </DialogTitle>
        <DialogContent>
          {selectedCloneGroupId && (
            <CloneGroupProgressFull groupId={selectedCloneGroupId} />
          )}
        </DialogContent>
      </Dialog>

      {/* Archive Confirmation Dialog */}
      <ArchiveConfirmDialog
        open={archiveConfirmOpen}
        onClose={() => setArchiveConfirmOpen(false)}
        onConfirm={performArchive}
        taskName={task?.name}
        isArchiving={isArchiving}
      />
    </Box>
  );
};

export default SpecTaskDetailContent;
