import React, { useState, useMemo, useEffect, useRef } from "react";
import {
  Card,
  CardContent,
  Box,
  Typography,
  Button,
  IconButton,
  Tooltip,
  Alert,
  CircularProgress,
  LinearProgress,
  keyframes,
  Dialog,
  DialogTitle,
  DialogContent,
  Menu,
  MenuItem,
  ListItemIcon,
  ListItemText,
  Avatar,
  Chip,
} from "@mui/material";
import {
  Description as SpecIcon,
  Visibility as ViewIcon,
  CheckCircle as ApproveIcon,
  Stop as StopIcon,
  RocketLaunch as LaunchIcon,
  Close as CloseIcon,
  Circle as CircleIcon,
  CheckCircle as CheckCircleIcon,
  RadioButtonUnchecked as UncheckedIcon,
  AccountTree as BatchIcon,
  Delete as DeleteIcon,
  Archive as ArchiveIcon,
  Unarchive as UnarchiveIcon,
  RemoveCircleOutline as RemoveFromQueueIcon,
  Undo as UndoIcon,
} from "@mui/icons-material";
import { EllipsisVertical, Wand2, UserCircle2 } from "lucide-react";
import {
  useApproveImplementation,
  useStopAgent,
  useMoveToBacklog,
} from "../../services/specTaskWorkflowService";
import {
  useUpdateSpecTask,
  useDeleteSpecTask,
} from "../../services/specTaskService";
import {
  ServerTaskProgressResponse,
  TypesSpecTaskStatus,
  ServerBatchTaskUsageMetric,
} from "../../api/api";
import UsagePulseChart from "./UsagePulseChart";
import ExternalAgentDesktopViewer from "../external-agent/ExternalAgentDesktopViewer";
import CloneTaskDialog from "../specTask/CloneTaskDialog";
import CloneGroupProgressFull from "../specTask/CloneGroupProgress";
import SpecTaskActionButtons from "./SpecTaskActionButtons";
import AssigneeSelector from "./AssigneeSelector";
import useAccount from "../../hooks/useAccount";
import { TypesOrganizationMembership, TypesUser } from "../../api/api";
import { useAttentionEvents, AttentionEvent } from "../../hooks/useAttentionEvents";

// Pulse animation for the active task spinner
const pulseRing = keyframes`
  0% {
    transform: scale(0.8);
    opacity: 1;
  }
  50% {
    transform: scale(1.2);
    opacity: 0.5;
  }
  100% {
    transform: scale(0.8);
    opacity: 1;
  }
`;

const spin = keyframes`
  0% {
    transform: rotate(0deg);
  }
  100% {
    transform: rotate(360deg);
  }
`;


type SpecTaskPhase =
  | "backlog"
  | "planning"
  | "review"
  | "implementation"
  | "pull_request"
  | "completed";

export interface SpecTaskWithExtras {
  id: string;
  name: string;
  status: string;
  phase: SpecTaskPhase;
  planningStatus?:
    | "none"
    | "active"
    | "pending_review"
    | "completed"
    | "failed"
    | "queued";
  planning_session_id?: string;
  archived?: boolean;
  metadata?: { error?: string; error_timestamp?: string };
  merged_to_main?: boolean;
  just_do_it_mode?: boolean;
  started_at?: string;
  design_docs_pushed_at?: string;
  clone_group_id?: string;
  cloned_from_id?: string;
  repo_pull_requests?: Array<{
    repository_id?: string;
    repository_name?: string;
    pr_id?: string;
    pr_number?: number;
    pr_url?: string;
    pr_state?: string;
  }>;
  implementation_approved_at?: string;
  // Branch tracking for direct-push detection
  base_branch?: string;
  branch_name?: string;
  // Agent activity tracking
  session_updated_at?: string;
  agent_work_state?: "idle" | "working" | "done"; // Backend-tracked work state
  // Sandbox state — populated by the listTasks backend handler, avoids per-card session polling
  sandbox_state?: string; // "absent" | "running" | "starting"
  sandbox_status_message?: string; // Transient startup message
  // Status tracking
  status_updated_at?: string;
  // Task number for display
  task_number?: number;
  description?: string;
  depends_on?: TaskDependency[];
  // Assignee tracking
  assignee_id?: string;
  labels?: string[];
  updated_at?: string;
  last_push_at?: string;
}

export interface TaskDependency {
  id?: string;
  task_number?: number;
  status?: string;
  archived?: boolean;
}

export interface KanbanColumn {
  id: SpecTaskPhase;
  limit?: number;
  tasks: SpecTaskWithExtras[];
}

interface TaskCardProps {
  task: SpecTaskWithExtras;
  index: number;
  columns: KanbanColumn[];
  onStartPlanning?: (task: SpecTaskWithExtras) => Promise<void>;
  onArchiveTask?: (
    task: SpecTaskWithExtras,
    archived: boolean,
    shiftKey?: boolean,
  ) => Promise<void>;
  onTaskClick?: (task: SpecTaskWithExtras) => void;
  onReviewDocs?: (task: SpecTaskWithExtras) => void;
  projectId?: string;
  isArchiving?: boolean;
  hasExternalRepo?: boolean;
  externalRepoType?: string;
  showMetrics?: boolean;
  /** Hide the "Clone to other projects" menu option (used in clone batch progress view) */
  hideCloneOption?: boolean;
  /** Pre-fetched progress data from batch endpoint - required */
  progressData?: ServerTaskProgressResponse;
  /** Pre-fetched usage data from batch endpoint - required */
  usageData?: ServerBatchTaskUsageMetric[];
  highlightedTaskIds?: string[] | null;
  onDependencyHoverStart?: (taskIds: string[]) => void;
  onDependencyHoverEnd?: () => void;
  /** Whether to focus the Start Planning button (for newly created tasks) */
  focusStartPlanning?: boolean;
  /** Whether the card is currently visible (for virtualization) */
  isVisible?: boolean;
  /** Unread attention events for this task (agent_interaction_completed events without acknowledged_at) */
  attentionEvents?: AttentionEvent[];
}

// Interface for checklist items from API
interface ChecklistItem {
  index: number;
  description: string;
  status: string;
}

interface ChecklistProgress {
  tasks: ChecklistItem[];
  total_tasks: number;
  completed_tasks: number;
  in_progress_task?: ChecklistItem;
  progress_pct: number;
}

const formatDuration = (startedAt: string): string => {
  const start = new Date(startedAt).getTime();
  const now = Date.now();
  const diffMs = now - start;

  if (diffMs < 0) return "0s";

  const totalSeconds = Math.floor(diffMs / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) {
    return `${hours}h${minutes}m`;
  }
  return `${minutes}m${seconds}s`;
};

const useRunningDuration = (
  startedAt: string | undefined,
  enabled: boolean,
): string | null => {
  const [duration, setDuration] = useState<string | null>(null);

  useEffect(() => {
    if (!enabled || !startedAt) {
      setDuration(null);
      return;
    }

    setDuration(formatDuration(startedAt));

    const interval = setInterval(() => {
      setDuration(formatDuration(startedAt));
    }, 1000);

    return () => clearInterval(interval);
  }, [startedAt, enabled]);

  return duration;
};

// Gorgeous task progress display with fade effect and spinner
const TaskProgressDisplay: React.FC<{
  checklist: ChecklistProgress;
  phaseColor: string;
}> = React.memo(({ checklist, phaseColor }) => {
  // Find the in-progress task index
  const inProgressIndex = checklist.in_progress_task?.index ?? -1;

  // Get visible tasks: 1 before, in-progress, 2 after (or adjust based on available)
  const visibleTasks = useMemo(() => {
    if (!checklist.tasks || checklist.tasks.length === 0) return [];

    const tasks = checklist.tasks;
    const activeIdx =
      inProgressIndex >= 0
        ? inProgressIndex
        : tasks.findIndex((t) => t.status === "pending");

    if (activeIdx < 0) {
      // All completed - show last 3
      return tasks
        .slice(-3)
        .map((t, i) => ({ ...t, fadeLevel: i === 2 ? 0 : 1 }));
    }

    // Show 1 before, active, 2 after with fade levels
    const start = Math.max(0, activeIdx - 1);
    const end = Math.min(tasks.length, activeIdx + 3);

    return tasks.slice(start, end).map((t, i, arr) => {
      const relativePos = t.index - activeIdx;
      let fadeLevel = 0;
      if (relativePos < 0)
        fadeLevel = 2; // Before: more faded
      else if (relativePos > 1)
        fadeLevel = 2; // Far after: more faded
      else if (relativePos === 1) fadeLevel = 1; // Just after: slightly faded
      return { ...t, fadeLevel };
    });
  }, [checklist.tasks, inProgressIndex]);

  if (visibleTasks.length === 0) return null;

  return (
    <Box
      sx={{
        mt: 1.5,
        mb: 0.5,
        background:
          "linear-gradient(145deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%)",
        borderRadius: 2,
        border: "1px solid rgba(255,255,255,0.06)",
        overflow: "hidden",
      }}
    >
      {/* Progress bar header */}
      <Box
        sx={{
          px: 1.5,
          py: 0.75,
          background:
            "linear-gradient(90deg, rgba(0,0,0,0.15) 0%, rgba(0,0,0,0.05) 100%)",
          borderBottom: "1px solid rgba(255,255,255,0.04)",
          display: "flex",
          alignItems: "center",
          gap: 1,
        }}
      >
        <LinearProgress
          variant="determinate"
          value={checklist.progress_pct}
          sx={{
            flex: 1,
            height: 4,
            borderRadius: 2,
            backgroundColor: "rgba(255,255,255,0.08)",
            "& .MuiLinearProgress-bar": {
              background: `linear-gradient(90deg, ${phaseColor}99 0%, ${phaseColor} 100%)`,
              borderRadius: 2,
            },
          }}
        />
        <Typography
          variant="caption"
          sx={{
            fontSize: "0.65rem",
            color: "rgba(255,255,255,0.5)",
            fontWeight: 600,
            letterSpacing: "0.02em",
            minWidth: 32,
            textAlign: "right",
          }}
        >
          {checklist.completed_tasks}/{checklist.total_tasks}
        </Typography>
      </Box>

      {/* Task list with fade effect */}
      <Box sx={{ py: 0.5 }}>
        {visibleTasks.map((task, idx) => {
          const isActive = task.status === "in_progress";
          const isCompleted =
            task.status === "done" || task.status === "completed";
          const opacity =
            task.fadeLevel === 2 ? 0.35 : task.fadeLevel === 1 ? 0.6 : 1;

          return (
            <Box
              key={task.index}
              sx={{
                px: 1.5,
                py: 0.5,
                display: "flex",
                alignItems: "flex-start",
                gap: 0.75,
                opacity,
                transition: "opacity 0.3s ease",
              }}
            >
              {/* Status indicator */}
              <Box
                sx={{
                  width: 16,
                  height: 16,
                  display: "flex",
                  alignItems: "center",
                  justifyContent: "center",
                  flexShrink: 0,
                  mt: 0.1,
                }}
              >
                {isActive ? (
                  // Animated spinner for active task
                  <Box sx={{ position: "relative", width: 14, height: 14 }}>
                    <Box
                      sx={{
                        position: "absolute",
                        inset: 0,
                        borderRadius: "50%",
                        border: `2px solid ${phaseColor}`,
                        borderTopColor: "transparent",
                        animation: `${spin} 1s linear infinite`,
                      }}
                    />
                    <Box
                      sx={{
                        position: "absolute",
                        inset: 2,
                        borderRadius: "50%",
                        backgroundColor: phaseColor,
                        animation: `${pulseRing} 2s ease-in-out infinite`,
                      }}
                    />
                  </Box>
                ) : isCompleted ? (
                  <CheckCircleIcon sx={{ fontSize: 14, color: "#10b981" }} />
                ) : (
                  <UncheckedIcon
                    sx={{ fontSize: 14, color: "rgba(255,255,255,0.25)" }}
                  />
                )}
              </Box>

              {/* Task description */}
              <Typography
                variant="caption"
                sx={{
                  fontSize: "0.68rem",
                  lineHeight: 1.35,
                  color: isActive
                    ? "rgba(255,255,255,0.9)"
                    : "rgba(255,255,255,0.6)",
                  fontWeight: isActive ? 500 : 400,
                  overflow: "hidden",
                  textOverflow: "ellipsis",
                  display: "-webkit-box",
                  WebkitLineClamp: 2,
                  WebkitBoxOrient: "vertical",
                  textDecoration: isCompleted ? "line-through" : "none",
                  textDecorationColor: "rgba(255,255,255,0.3)",
                }}
              >
                {task.description}
              </Typography>
            </Box>
          );
        })}
      </Box>
    </Box>
  );
});

const LiveAgentScreenshot: React.FC<{
  sessionId: string;
  projectId?: string;
  onClick?: () => void;
  startupErrorMessage?: string;
  sandboxState?: string;
  sandboxStatusMessage?: string;
}> = React.memo(({ sessionId, projectId, onClick, startupErrorMessage, sandboxState, sandboxStatusMessage }) => {
  return (
    <Box
      onClick={(e) => {
        e.stopPropagation(); // Prevent card click
        if (onClick) onClick();
      }}
      sx={{
        mt: 1.5,
        mb: 0.5,
        mx: -1, // Extend slightly beyond content using negative margins
        width: "calc(100% + 16px)", // Compensate for negative margins
        position: "relative",
        borderRadius: 1.5,
        overflow: "hidden",
        border: "1px solid",
        borderColor: "rgba(0, 0, 0, 0.08)",
        aspectRatio: "16 / 9", // 16:9 aspect ratio based on card width
        cursor: "pointer",
        transition: "all 0.15s ease",
        "&:hover": {
          borderColor: "primary.main",
          boxShadow: "0 0 0 1px rgba(33, 150, 243, 0.3)",
        },
      }}
    >
      <Box sx={{ position: "relative", height: "100%" }}>
        <ExternalAgentDesktopViewer
          sessionId={sessionId}
          mode="screenshot"
          startupErrorMessage={startupErrorMessage}
          initialSandboxState={sandboxState}
          initialSandboxStatusMessage={sandboxStatusMessage}
        />
      </Box>
      <Box
        sx={{
          position: "absolute",
          bottom: 0,
          left: 0,
          right: 0,
          background:
            "linear-gradient(to top, rgba(0,0,0,0.6) 0%, transparent 100%)",
          color: "white",
          py: 0.75,
          px: 1.5,
          display: "flex",
          alignItems: "flex-end",
          justifyContent: "flex-end",
          pointerEvents: "none",
        }}
      >
        <Typography
          variant="caption"
          sx={{ fontWeight: 500, fontSize: "0.65rem", opacity: 0.8 }}
        >
          Click to view
        </Typography>
      </Box>
    </Box>
  );
});

function TaskCardInner({
  task,
  index,
  columns,
  onStartPlanning,
  onArchiveTask,
  onTaskClick,
  onReviewDocs,
  projectId,
  isArchiving = false,
  hasExternalRepo = false,
  externalRepoType,
  showMetrics = true,
  hideCloneOption = false,
  progressData,
  usageData,
  highlightedTaskIds,
  onDependencyHoverStart,
  onDependencyHoverEnd,
  focusStartPlanning = false,
  attentionEvents = [],
}: TaskCardProps) {
  const [isStartingPlanning, setIsStartingPlanning] = useState(false);
  const [showCloneDialog, setShowCloneDialog] = useState(false);
  const [showCloneBatchProgress, setShowCloneBatchProgress] = useState(false);
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [isRemovingFromQueue, setIsRemovingFromQueue] = useState(false);
  const [assigneeAnchorEl, setAssigneeAnchorEl] = useState<null | HTMLElement>(null);
  const startPlanningButtonRef = useRef<HTMLButtonElement>(null);

  // Get account context for organization members
  const account = useAccount();
  const orgMembers = account.organizationTools.organization?.memberships || [];

  // Find the assigned user from org members
  const assignedMember = useMemo(() => {
    if (!task.assignee_id) return null;
    return orgMembers.find((m) => m.user_id === task.assignee_id);
  }, [task.assignee_id, orgMembers]);

  const assignedUser = assignedMember?.user as TypesUser | undefined;

  // Get initials for avatar
  const getAssigneeInitials = (user: TypesUser | undefined): string => {
    if (!user) return "?";
    const name = user.full_name || user.username || user.email || "";
    const parts = name.split(" ");
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase();
    }
    return name.slice(0, 2).toUpperCase();
  };

  // Handle assignee change
  const handleAssigneeChange = (userId: string | null) => {
    if (task.id) {
      updateSpecTask.mutate({
        taskId: task.id,
        updates: { assignee_id: userId || "" },
      });
    }
  };

  // Focus the Start Planning button when focusStartPlanning is true
  useEffect(() => {
    if (focusStartPlanning && task.status === "backlog") {
      setTimeout(() => {
        startPlanningButtonRef.current?.focus();
      }, 100);
    }
  }, [focusStartPlanning, task.status]);
  const approveImplementationMutation = useApproveImplementation(task.id!);
  const stopAgentMutation = useStopAgent(task.id!);
  const moveToBacklogMutation = useMoveToBacklog(task.id!);
  const updateSpecTask = useUpdateSpecTask();
  const deleteSpecTask = useDeleteSpecTask();

  // Check if we should show progress (planning/implementation phases)
  const showProgress =
    task.phase === "planning" || task.phase === "implementation";

  const { acknowledge } = useAttentionEvents();
  const hasUnreadNotification = attentionEvents.length > 0;

  const runningDuration = useRunningDuration(
    task.started_at,
    task.status === "implementation",
  );
  const taskError = task.metadata?.error?.trim();

  // Check if planning column is full
  const planningColumn = columns.find((col) => col.id === "planning");
  const isPlanningFull =
    planningColumn && planningColumn.limit
      ? planningColumn.tasks.length >= planningColumn.limit
      : false;

  const isQueued =
    task.status === "queued_implementation" ||
    task.status === "queued_spec_generation" ||
    task.status === "spec_approved";

  // Can move to backlog from any phase except backlog itself and queued states
  const canMoveToBacklog =
    !isQueued && task.phase !== "backlog" && task.status !== "backlog";

  const handleRemoveFromQueue = async () => {
    if (!task.id) return;
    setIsRemovingFromQueue(true);
    try {
      await updateSpecTask.mutateAsync({
        taskId: task.id,
        updates: { status: TypesSpecTaskStatus.TaskStatusBacklog },
      });
    } finally {
      setIsRemovingFromQueue(false);
    }
  };

  // Get phase-based accent color for cards
  const getPhaseAccent = (phase: string) => {
    switch (phase) {
      case "planning":
        return "#f59e0b";
      case "review":
        return "#3b82f6";
      case "implementation":
        return "#10b981";
      case "pull_request":
        return "#8b5cf6"; // Purple for PR
      case "completed":
        return "#6b7280";
      default:
        return "#e5e7eb";
    }
  };

  const accentColor = getPhaseAccent(task.phase);
  const isArchived = task.archived ?? false;
  const isDependencyHighlighted =
    !!task.id && !!highlightedTaskIds?.includes(task.id);
  const unfinishedDependencies = useMemo(() => {
    const dependencies = task.depends_on || [];
    return dependencies.filter((dependency) => {
      const dependencyStatus = dependency.status || "";
      const isCompleted =
        dependencyStatus === "done" || dependencyStatus === "completed";
      return !dependency.archived && !isCompleted;
    });
  }, [task.depends_on]);
  const blockingDependency = unfinishedDependencies[0];

  const formatDependencyTaskRef = (dependency: TaskDependency) => {
    if (dependency.task_number && dependency.task_number > 0) {
      return String(dependency.task_number).padStart(6, "0");
    }
    if (dependency.id) {
      return dependency.id.slice(0, 8);
    }
    return "?";
  };

  // Handle card click - always open task detail view (session viewer)
  const handleCardClick = () => {
    attentionEvents.forEach((event) => acknowledge(event.id));
    if (onTaskClick) {
      onTaskClick(task);
    }
  };

  return (
    <Card
      onClick={handleCardClick}
      sx={{
        mb: 1.5,
        backgroundColor: isArchived
          ? "rgba(156, 163, 175, 0.08)"
          : "background.paper",
        cursor: isArchiving ? "default" : "pointer",
        border: isArchived ? "1px dashed" : "1px solid",
        borderColor: isArchived
          ? "rgba(156, 163, 175, 0.4)"
          : "rgba(0, 0, 0, 0.08)",
        borderLeft: `3px ${isArchived ? "dashed" : "solid"} ${
          isArchived
            ? "rgba(156, 163, 175, 0.5)"
            : isDependencyHighlighted
              ? "#ef4444"
              : accentColor
        }`,
        boxShadow: "none",
        transition: "all 0.15s ease-in-out",
        opacity: isArchiving ? 0.5 : isArchived ? 0.7 : 1,
        pointerEvents: isArchiving ? "none" : "auto",
        position: "relative",
        "&:hover": {
          borderColor: isArchived
            ? "rgba(156, 163, 175, 0.5)"
            : "rgba(0, 0, 0, 0.12)",
          backgroundColor: isArchived
            ? "rgba(156, 163, 175, 0.12)"
            : "rgba(0, 0, 0, 0.01)",
        },
      }}
    >
      {hasUnreadNotification && (
        <Tooltip title="Agent finished - click to dismiss">
          <Box
            sx={{
              position: "absolute",
              top: 8,
              right: 8,
              width: 10,
              height: 10,
              borderRadius: "50%",
              backgroundColor: "error.main",
              zIndex: 1,
              border: "2px solid",
              borderColor: "background.paper",
            }}
          />
        </Tooltip>
      )}
      <CardContent sx={{ p: 2, "&:last-child": { pb: 2 } }}>
        {/* Task name */}
        <Box
          sx={{
            display: "flex",
            justifyContent: "space-between",
            alignItems: "flex-start",
            mb: 1,
            minWidth: 0,
          }}
        >
          <Tooltip
            title={
              <span style={{ whiteSpace: "pre-wrap" }}>
                {task.description || task.name}
              </span>
            }
            placement="top"
            enterDelay={500}
            arrow
          >
            <Typography
              variant="body2"
              sx={{
                fontWeight: 500,
                flex: 1,
                minWidth: 0,
                lineHeight: 1.4,
                color: "text.primary",
                wordBreak: "break-word",
              }}
            >
              {task.name}
            </Typography>
          </Tooltip>
          <IconButton
            size="small"
            onClick={(e) => {
              e.stopPropagation();
              setMenuAnchorEl(e.currentTarget);
            }}
            sx={{
              width: 24,
              height: 24,
              color: "text.secondary",
              ml: 0.5,
              flexShrink: 0,
              "&:hover": {
                color: "text.primary",
                backgroundColor: "rgba(0, 0, 0, 0.04)",
              },
            }}
          >
            <EllipsisVertical size={16} />
          </IconButton>
          <Menu
            anchorEl={menuAnchorEl}
            open={Boolean(menuAnchorEl)}
            onClose={(e: React.SyntheticEvent) => {
              e.stopPropagation?.();
              setMenuAnchorEl(null);
            }}
            onClick={(e) => e.stopPropagation()}
            anchorOrigin={{ vertical: "bottom", horizontal: "right" }}
            transformOrigin={{ vertical: "top", horizontal: "right" }}
            slotProps={{
              paper: {
                sx: {
                  minWidth: 160,
                  boxShadow: "0 4px 20px rgba(0,0,0,0.15)",
                  "& .MuiMenuItem-root": {
                    py: 0.75,
                    minHeight: "unset",
                  },
                  "& .MuiListItemIcon-root": {
                    minWidth: 28,
                  },
                  "& .MuiListItemText-primary": {
                    fontSize: "0.8rem",
                  },
                },
              },
            }}
          >
            {task.design_docs_pushed_at && onReviewDocs && (
              <MenuItem
                onClick={() => {
                  setMenuAnchorEl(null);
                  onReviewDocs(task);
                }}
              >
                <ListItemIcon>
                  <SpecIcon sx={{ fontSize: 16 }} />
                </ListItemIcon>
                <ListItemText>Review Spec</ListItemText>
              </MenuItem>
            )}
            {task.clone_group_id && (
              <MenuItem
                onClick={() => {
                  setMenuAnchorEl(null);
                  setShowCloneBatchProgress(true);
                }}
              >
                <ListItemIcon>
                  <BatchIcon sx={{ fontSize: 16 }} />
                </ListItemIcon>
                <ListItemText>Batch Progress</ListItemText>
              </MenuItem>
            )}
            {isQueued && (
              <MenuItem
                disabled={isRemovingFromQueue}
                onClick={() => {
                  setMenuAnchorEl(null);
                  handleRemoveFromQueue();
                }}
              >
                <ListItemIcon>
                  <RemoveFromQueueIcon sx={{ fontSize: 16 }} />
                </ListItemIcon>
                <ListItemText>
                  {isRemovingFromQueue ? "Removing..." : "Remove from queue"}
                </ListItemText>
              </MenuItem>
            )}
            {canMoveToBacklog && (
              <MenuItem
                disabled={moveToBacklogMutation.isPending}
                onClick={() => {
                  setMenuAnchorEl(null);
                  moveToBacklogMutation.mutate();
                }}
              >
                <ListItemIcon>
                  {moveToBacklogMutation.isPending ? (
                    <CircularProgress size={16} />
                  ) : (
                    <UndoIcon sx={{ fontSize: 16 }} />
                  )}
                </ListItemIcon>
                <ListItemText>
                  {moveToBacklogMutation.isPending
                    ? "Moving..."
                    : "Move to Backlog"}
                </ListItemText>
              </MenuItem>
            )}
            {!hideCloneOption && task.design_docs_pushed_at && (
              <MenuItem
                onClick={() => {
                  setMenuAnchorEl(null);
                  setShowCloneDialog(true);
                }}
              >
                <ListItemIcon>
                  <Wand2 size={16} />
                </ListItemIcon>
                <ListItemText>Clone to projects</ListItemText>
              </MenuItem>
            )}
            <MenuItem
              disabled={isArchiving}
              onClick={(e) => {
                setMenuAnchorEl(null);
                if (onArchiveTask) {
                  onArchiveTask(task, !task.archived, e.shiftKey);
                }
              }}
            >
              <ListItemIcon>
                {isArchiving ? (
                  <CircularProgress size={16} />
                ) : task.archived ? (
                  <UnarchiveIcon sx={{ fontSize: 16 }} />
                ) : (
                  <ArchiveIcon sx={{ fontSize: 16 }} />
                )}
              </ListItemIcon>
              <ListItemText>
                {isArchiving
                  ? "Archiving..."
                  : task.archived
                    ? "Restore"
                    : "Archive"}
              </ListItemText>
            </MenuItem>
            {task.archived && (
              <MenuItem
                disabled={deleteSpecTask.isPending}
                onClick={() => {
                  setMenuAnchorEl(null);
                  if (task.id) {
                    deleteSpecTask.mutate(task.id);
                  }
                }}
              >
                <ListItemIcon>
                  <DeleteIcon sx={{ fontSize: 16, color: "error.main" }} />
                </ListItemIcon>
                <ListItemText sx={{ color: "error.main" }}>
                  {deleteSpecTask.isPending ? "Deleting..." : "Delete"}
                </ListItemText>
              </MenuItem>
            )}
          </Menu>
        </Box>

        {/* Status row */}
        <Box sx={{ display: "flex", gap: 1.5, alignItems: "center", mb: 1.5 }}>
          <Box sx={{ display: "flex", alignItems: "center", gap: 0.5 }}>
            <CircleIcon
              sx={{
                fontSize: 8,
                color:
                  task.phase === "planning"
                    ? "#f59e0b"
                    : task.phase === "review"
                      ? "#3b82f6"
                      : task.phase === "implementation"
                        ? "#10b981"
                        : task.phase === "pull_request"
                          ? "#8b5cf6"
                          : task.phase === "completed"
                            ? "#6b7280"
                            : "#9ca3af",
              }}
            />
            <Typography
              variant="caption"
              sx={{
                fontSize: "0.7rem",
                color: "text.secondary",
                fontWeight: 500,
              }}
            >
              {task.phase === "backlog"
                ? "Backlog"
                : task.phase === "planning"
                  ? "Planning"
                  : task.phase === "review"
                    ? "Review"
                    : task.phase === "implementation"
                      ? "In Progress"
                      : task.phase === "pull_request"
                        ? "Pull Request"
                        : "Merged"}
            </Typography>
            {runningDuration && (
              <Typography
                variant="caption"
                sx={{ fontSize: "0.7rem", color: "text.secondary" }}
              >
                • {runningDuration}
              </Typography>
            )}
          </Box>

          {/* Assignee avatar */}
          <Tooltip title={assignedUser ? `Assigned to ${assignedUser.full_name || assignedUser.username || assignedUser.email}` : "Unassigned - click to assign"}>
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation();
                setAssigneeAnchorEl(e.currentTarget);
              }}
              sx={{
                width: 24,
                height: 24,
                ml: "auto",
              }}
            >
              {assignedUser ? (
                <Avatar
                  sx={{
                    width: 20,
                    height: 20,
                    fontSize: "0.6rem",
                  }}
                >
                  {getAssigneeInitials(assignedUser)}
                </Avatar>
              ) : (
                <UserCircle2 size={18} style={{ opacity: 0.4 }} />
              )}
            </IconButton>
          </Tooltip>
        </Box>

        {/* Assignee selector popover */}
        <AssigneeSelector
          assigneeId={task.assignee_id}
          members={orgMembers}
          currentUserId={account.user?.id}
          onAssigneeChange={handleAssigneeChange}
          isLoading={updateSpecTask.isPending}
          anchorEl={assigneeAnchorEl}
          onClose={() => setAssigneeAnchorEl(null)}
        />

        {/* Label chips */}
        {task.labels && task.labels.length > 0 && (
          <Box sx={{ display: "flex", flexWrap: "wrap", gap: 0.5, mb: 1 }}>
            {task.labels.map((label) => (
              <Chip key={label} label={label} size="small" variant="outlined" sx={{ height: 18, fontSize: "0.65rem" }} />
            ))}
          </Box>
        )}

        {/* Usage pulse chart - shows activity over last 3 days (only for active phases) */}
        {showMetrics &&
          (task.phase === "planning" ||
            task.phase === "review" ||
            task.phase === "implementation" ||
            task.phase === "pull_request") && (
            <UsagePulseChart accentColor={accentColor} usageData={usageData} />
          )}

        {/* Gorgeous checklist progress for active tasks */}
        {progressData?.checklist && progressData.checklist.total_tasks > 0 && (
          <TaskProgressDisplay
            checklist={progressData.checklist as ChecklistProgress}
            phaseColor={accentColor}
          />
        )}

        {/* Show task error prominently and avoid desktop viewer when errored */}
        {taskError && task.phase !== "completed" && (
          <Box
            sx={{
              mb: 1,
              px: 1.5,
              py: 1,
              backgroundColor: "rgba(239, 68, 68, 0.08)",
              borderRadius: 1,
              border: "1px solid rgba(239, 68, 68, 0.2)",
            }}
          >
            <Typography
              variant="caption"
              sx={{ fontWeight: 500, color: "#ef4444", fontSize: "0.7rem" }}
            >
              ⚠ {taskError}
            </Typography>
          </Box>
        )}

        {/* Queued state: waiting for orchestrator to create session */}
        {isQueued && !task.planning_session_id && (
          <Box
            sx={{
              display: "flex",
              alignItems: "center",
              gap: 1,
              mt: 1,
              px: 0.5,
            }}
          >
            <CircularProgress size={10} thickness={5} />
            <Typography
              variant="caption"
              sx={{ color: "text.secondary", fontSize: "0.7rem" }}
            >
              Starting desktop...
            </Typography>
          </Box>
        )}

        {/* Live screenshot for active sessions - click opens desktop viewer */}
        {/* Don't show for completed/merged tasks - the container is shut down */}
        {task.planning_session_id &&
          task.phase !== "completed" &&
          !task.merged_to_main &&
          !taskError && (
            <LiveAgentScreenshot
              sessionId={task.planning_session_id}
              projectId={projectId}
              startupErrorMessage={
                typeof task.metadata?.error === "string"
                  ? task.metadata.error
                  : undefined
              }
              sandboxState={task.sandbox_state}
              sandboxStatusMessage={task.sandbox_status_message}
              onClick={() => onTaskClick?.(task)}
            />
          )}

        {/* Backlog phase */}
        {task.phase === "backlog" && (
          <Box sx={{ mt: 1.5 }}>
            {blockingDependency && blockingDependency.id && (
              <Typography
                variant="caption"
                onMouseEnter={() =>
                  onDependencyHoverStart?.(
                    unfinishedDependencies
                      .map((dependency) => dependency.id)
                      .filter((id): id is string => !!id),
                  )
                }
                onMouseLeave={() => onDependencyHoverEnd?.()}
                sx={{
                  mb: 0.75,
                  display: "inline-flex",
                  fontSize: "0.72rem",
                  color: "text.secondary",
                  fontWeight: 500,
                  cursor: "pointer",
                  "&:hover": {
                    color: "primary.main",
                    textDecoration: "underline",
                  },
                }}
              >
                Depends on: #{formatDependencyTaskRef(blockingDependency)}
              </Typography>
            )}
            <SpecTaskActionButtons
              task={{
                id: task.id,
                status: "backlog",
                just_do_it_mode: task.just_do_it_mode,
                archived: task.archived,
                metadata: task.metadata,
              }}
              variant="stacked"
              startPlanningButtonRef={startPlanningButtonRef}
              onStartPlanning={async () => {
                if (onStartPlanning) {
                  setIsStartingPlanning(true);
                  try {
                    await onStartPlanning(task);
                  } finally {
                    setIsStartingPlanning(false);
                  }
                }
              }}
              isStartingPlanning={isStartingPlanning}
              isQueued={task.planningStatus === "queued"}
              isPlanningFull={isPlanningFull}
              planningLimit={planningColumn?.limit}
              isBlockedByDependencies={unfinishedDependencies.length > 0}
              blockedReason={
                blockingDependency
                  ? `Depends on: #${formatDependencyTaskRef(blockingDependency)}`
                  : ""
              }
            />
            {isPlanningFull && (
              <Typography
                variant="caption"
                sx={{
                  mt: 0.75,
                  display: "block",
                  textAlign: "center",
                  color: "#ef4444",
                  fontSize: "0.7rem",
                }}
              >
                Planning column at capacity ({planningColumn?.limit})
              </Typography>
            )}
          </Box>
        )}

        {/* Planning phase - waiting for agent to push specs */}
        {task.status === "spec_generation" && (
          <Box sx={{ mt: 1.5 }}>
            {(() => {
              // Calculate seconds since spec generation started
              const statusUpdatedAt = task.status_updated_at
                ? new Date(task.status_updated_at).getTime()
                : 0;
              const secondsSinceStart = statusUpdatedAt
                ? (Date.now() - statusUpdatedAt) / 1000
                : 0;
              const isWaitingTooLong = secondsSinceStart > 120; // 2 minutes

              return isWaitingTooLong ? (
                <Alert severity="warning" sx={{ py: 0.5 }}>
                  <Typography
                    variant="caption"
                    sx={{ fontSize: "0.7rem", display: "block" }}
                  >
                    Agent hasn't pushed specs yet. Please check if the agent is
                    having trouble.
                  </Typography>
                </Alert>
              ) : (
                <Box
                  sx={{
                    display: "flex",
                    flexDirection: "column",
                    alignItems: "center",
                    gap: 1,
                  }}
                >
                  <CircularProgress size={20} />
                  <Typography
                    variant="caption"
                    sx={{
                      fontSize: "0.7rem",
                      color: "text.secondary",
                      fontStyle: "italic",
                      textAlign: "center",
                    }}
                  >
                    Waiting for agent to push specs...
                  </Typography>
                </Box>
              );
            })()}
            <Box sx={{ mt: 1 }}>
              <SpecTaskActionButtons
                task={{
                  id: task.id,
                  status: "spec_generation",
                  archived: task.archived,
                }}
                variant="stacked"
              />
            </Box>
          </Box>
        )}

        {/* Review phase - only show button if design docs have been pushed */}
        {task.phase === "review" &&
          task.design_docs_pushed_at &&
          onReviewDocs && (
            <SpecTaskActionButtons
              task={{
                id: task.id,
                status: "spec_review",
                design_docs_pushed_at: task.design_docs_pushed_at,
                archived: task.archived,
              }}
              variant="stacked"
              onReviewSpec={() => onReviewDocs(task)}
              isQueued={task.status === "spec_approved"}
            />
          )}

        {/* Implementation phase */}
        {task.status === "implementation" && (
          <SpecTaskActionButtons
            task={{
              id: task.id,
              status: "implementation",
              base_branch: task.base_branch,
              branch_name: task.branch_name,
              archived: task.archived,
              last_push_at: task.last_push_at,
            }}
            variant="stacked"
            onReject={(shiftKey) => {
              if (onArchiveTask) {
                onArchiveTask(task, true, shiftKey);
              }
            }}
            hasExternalRepo={hasExternalRepo}
            externalRepoType={externalRepoType}
            isArchiving={isArchiving}
          />
        )}

        {/* Implementation review phase */}
        {task.status === "implementation_review" && (
          <Box
            sx={{ mt: 1.5, display: "flex", flexDirection: "column", gap: 1 }}
          >
            <Tooltip
              title={isArchived ? "Task is archived" : ""}
              placement="top"
            >
              <span style={{ width: "100%", display: "block" }}>
                <Button
                  size="small"
                  variant="contained"
                  color="secondary"
                  startIcon={<ViewIcon />}
                  onClick={(e) => {
                    e.stopPropagation();
                    console.log("[TaskCard] Review Implementation clicked", {
                      onTaskClick: !!onTaskClick,
                      taskId: task.id,
                    });
                    if (onTaskClick) onTaskClick(task);
                  }}
                  disabled={isArchived}
                  fullWidth
                >
                  Review Implementation
                </Button>
              </span>
            </Tooltip>
            <Tooltip
              title={isArchived ? "Task is archived" : ""}
              placement="top"
            >
              <span style={{ width: "100%", display: "block" }}>
                <Button
                  size="small"
                  variant="contained"
                  color="success"
                  startIcon={
                    approveImplementationMutation.isPending ? (
                      <CircularProgress size={14} color="inherit" />
                    ) : (
                      <ApproveIcon />
                    )
                  }
                  onClick={(e) => {
                    e.stopPropagation();
                    approveImplementationMutation.mutate();
                  }}
                  disabled={
                    isArchived || approveImplementationMutation.isPending
                  }
                  fullWidth
                >
                  {approveImplementationMutation.isPending
                    ? "Approving..."
                    : "Approve Implementation"}
                </Button>
              </span>
            </Tooltip>
            <Tooltip
              title={isArchived ? "Task is archived" : ""}
              placement="top"
            >
              <span style={{ width: "100%", display: "block" }}>
                <Button
                  size="small"
                  variant="outlined"
                  color="error"
                  startIcon={
                    stopAgentMutation.isPending ? (
                      <CircularProgress size={14} color="inherit" />
                    ) : (
                      <StopIcon />
                    )
                  }
                  onClick={(e) => {
                    e.stopPropagation();
                    stopAgentMutation.mutate();
                  }}
                  disabled={isArchived || stopAgentMutation.isPending}
                  fullWidth
                >
                  {stopAgentMutation.isPending ? "Stopping..." : "Stop Agent"}
                </Button>
              </span>
            </Tooltip>
          </Box>
        )}

        {/* Pull Request phase - awaiting merge in external repo */}
        {task.phase === "pull_request" && (
          <Box sx={{ mt: 1.5 }}>
            {task.repo_pull_requests && task.repo_pull_requests.length > 0 ? (
              <>
                <Box sx={{ mb: 1 }}>
                  <SpecTaskActionButtons
                    task={{
                      id: task.id,
                      status: "pull_request",
                      repo_pull_requests: task.repo_pull_requests,
                      archived: task.archived,
                    }}
                    variant="stacked"
                  />
                </Box>
                <Typography
                  variant="caption"
                  sx={{
                    fontSize: "0.7rem",
                    color: "text.secondary",
                    fontStyle: "italic",
                    display: "block",
                    textAlign: "center",
                  }}
                >
                  Address review comments with agent.
                  <br />
                  Moves to Merged when PR closes.
                </Typography>
              </>
            ) : (
              (() => {
                // Show specific error from task metadata if available
                if (taskError) {
                  return (
                    <Alert severity="error" sx={{ py: 0.5 }}>
                      <Typography
                        variant="caption"
                        sx={{ fontSize: "0.7rem", display: "block" }}
                      >
                        {taskError}
                      </Typography>
                    </Alert>
                  );
                }

                // Calculate seconds since approval
                const approvedAt = task.implementation_approved_at
                  ? new Date(task.implementation_approved_at).getTime()
                  : 0;
                const secondsSinceApproval = approvedAt
                  ? (Date.now() - approvedAt) / 1000
                  : 0;
                const isWaitingTooLong = secondsSinceApproval > 30;

                return isWaitingTooLong ? (
                  <Alert severity="warning" sx={{ py: 0.5 }}>
                    <Typography
                      variant="caption"
                      sx={{ fontSize: "0.7rem", display: "block" }}
                    >
                      Agent hasn't pushed feature branch yet. Please check if
                      the agent is having trouble.
                    </Typography>
                  </Alert>
                ) : (
                  <Box
                    sx={{
                      display: "flex",
                      flexDirection: "column",
                      alignItems: "center",
                      gap: 1,
                    }}
                  >
                    <CircularProgress size={20} />
                    <Typography
                      variant="caption"
                      sx={{
                        fontSize: "0.7rem",
                        color: "text.secondary",
                        fontStyle: "italic",
                        textAlign: "center",
                      }}
                    >
                      Waiting for agent to push branch to create PR...
                    </Typography>
                  </Box>
                );
              })()
            )}
          </Box>
        )}

        {/* Completed tasks */}
        {(task.status === "done" || task.phase === "completed") &&
          task.merged_to_main && (
            <Box sx={{ mt: 1.5 }}>
              <Alert severity="success" sx={{ py: 1 }}>
                <Typography variant="body2" sx={{ fontWeight: 500 }}>
                  Task finished
                </Typography>
                <Typography
                  variant="caption"
                  sx={{
                    fontSize: "0.7rem",
                    display: "block",
                    color: "text.secondary",
                  }}
                >
                  Merged to default branch
                </Typography>
              </Alert>
            </Box>
          )}
      </CardContent>

      {/* Task Number Badge */}
      {task.task_number && task.task_number > 0 && (
        <Typography
          variant="caption"
          sx={{
            position: "absolute",
            bottom: 8,
            right: 8,
            fontSize: "0.65rem",
            color: "text.disabled",
            fontFamily: "monospace",
            opacity: 0.7,
          }}
        >
          #{String(task.task_number).padStart(6, "0")}
        </Typography>
      )}

      {/* Clone Task Dialog */}
      <CloneTaskDialog
        open={showCloneDialog}
        onClose={() => setShowCloneDialog(false)}
        taskId={task.id}
        taskName={task.name}
        sourceProjectId={projectId || ""}
      />

      {/* Clone Batch Progress Dialog */}
      {task.clone_group_id && (
        <Dialog
          open={showCloneBatchProgress}
          onClose={() => setShowCloneBatchProgress(false)}
          maxWidth="md"
          fullWidth
          onClick={(e) => e.stopPropagation()}
          sx={{ "& .MuiDialog-paper": { zIndex: 1300 } }}
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
              onClick={() => setShowCloneBatchProgress(false)}
            >
              <CloseIcon />
            </IconButton>
          </DialogTitle>
          <DialogContent onClick={(e) => e.stopPropagation()}>
            <CloneGroupProgressFull
              groupId={task.clone_group_id}
              onTaskClick={(taskId, projectId) => {
                setShowCloneBatchProgress(false);
                if (onTaskClick) {
                  onTaskClick({ ...task, id: taskId });
                }
              }}
            />
          </DialogContent>
        </Dialog>
      )}
    </Card>
  );
}

// Memoized TaskCard to prevent unnecessary re-renders
const TaskCard = React.memo(TaskCardInner, (prevProps, nextProps) => {
  // Only re-render when meaningful props change
  return (
    prevProps.task.id === nextProps.task.id &&
    prevProps.task.status === nextProps.task.status &&
    prevProps.task.updated_at === nextProps.task.updated_at &&
    prevProps.task.agent_work_state === nextProps.task.agent_work_state &&
    prevProps.task.sandbox_state === nextProps.task.sandbox_state &&
    prevProps.task.sandbox_status_message === nextProps.task.sandbox_status_message &&
    prevProps.isArchiving === nextProps.isArchiving &&
    prevProps.isVisible === nextProps.isVisible &&
    prevProps.focusStartPlanning === nextProps.focusStartPlanning &&
    prevProps.usageData === nextProps.usageData &&
    prevProps.progressData === nextProps.progressData &&
    prevProps.attentionEvents?.length === nextProps.attentionEvents?.length
  );
});

export default TaskCard;
