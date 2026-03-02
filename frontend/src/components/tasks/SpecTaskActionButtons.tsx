import React, { RefObject } from "react";
import {
  Box,
  Button,
  CircularProgress,
  Tooltip,
  Typography,
} from "@mui/material";
import {
  PlayArrow as PlayIcon,
  Description as SpecIcon,
  CheckCircle as ApproveIcon,
  Close as CloseIcon,
  RocketLaunch as LaunchIcon,
} from "@mui/icons-material";
import {
  useApproveImplementation,
  useStopAgent,
} from "../../services/specTaskWorkflowService";

export interface SpecTaskForActions {
  id: string;
  status: string;
  design_docs_pushed_at?: string;
  pull_request_url?: string;
  base_branch?: string;
  branch_name?: string;
  archived?: boolean;
  just_do_it_mode?: boolean;
  planning_session_id?: string;
  metadata?: { error?: string };
}

interface SpecTaskActionButtonsProps {
  task: SpecTaskForActions;
  /** Whether to use compact/inline layout (for header bars) vs stacked layout (for cards) */
  variant?: "inline" | "stacked";
  /** Called when Start Planning is clicked */
  onStartPlanning?: () => Promise<void>;
  /** Ref for the Start Planning button (for focus management) */
  startPlanningButtonRef?: RefObject<HTMLButtonElement>;
  /** Called when Review Spec is clicked */
  onReviewSpec?: () => void;
  /** Called when Reject/Archive is clicked */
  onReject?: (shiftKey?: boolean) => void;
  /** Whether the connected project has an external repo (affects Accept vs Open PR text) */
  hasExternalRepo?: boolean;
  /** Whether archive/reject is in progress */
  isArchiving?: boolean;
  /** Whether start planning is in progress */
  isStartingPlanning?: boolean;
  /** Whether the task is queued (for planning or implementation) */
  isQueued?: boolean;
  /** Whether planning column is full */
  isPlanningFull?: boolean;
  /** Planning column limit for error message */
  planningLimit?: number;
  /** Whether task has unfinished dependencies that block planning start */
  isBlockedByDependencies?: boolean;
  /** Tooltip text explaining why start action is blocked */
  blockedReason?: string;
}

interface CompactActionButtonProps {
  tooltip?: string;
  color?:
    | "inherit"
    | "primary"
    | "secondary"
    | "success"
    | "error"
    | "info"
    | "warning";
  variant?: "text" | "outlined" | "contained";
  disabled?: boolean;
  fullWidth?: boolean;
  icon: React.ReactNode;
  label: string;
  onClick: (e: React.MouseEvent<HTMLButtonElement>) => void;
  sx?: object;
}

function CompactActionButton({
  tooltip = "",
  color = "primary",
  variant = "contained",
  disabled = false,
  fullWidth = false,
  icon,
  label,
  onClick,
  sx,
}: CompactActionButtonProps) {
  return (
    <Tooltip title={tooltip} placement="top">
      <span style={{ width: fullWidth ? "100%" : "auto", display: "block" }}>
        <Button
          size="small"
          color={color}
          variant={variant}
          disabled={disabled}
          fullWidth={fullWidth}
          onClick={onClick}
          sx={{
            minWidth: 72,
            px: 1,
            py: 0.5,
            lineHeight: 1,
            textTransform: "none",
            ...sx,
          }}
        >
          <Box
            sx={{
              display: "flex",
              flexDirection: "column",
              alignItems: "center",
              gap: 0.2,
            }}
          >
            {icon}
            <Typography
              sx={{
                fontSize: "0.65rem",
                lineHeight: 1,
                fontWeight: 400,
                textTransform: "none",
              }}
            >
              {label}
            </Typography>
          </Box>
        </Button>
      </span>
    </Tooltip>
  );
}

/**
 * Shared action buttons for spec tasks.
 * Displays appropriate action buttons based on task status.
 * Used in both TaskCard (Kanban) and SpecTaskDetailContent (detail view).
 */
export default function SpecTaskActionButtons({
  task,
  variant = "stacked",
  onStartPlanning,
  startPlanningButtonRef,
  onReviewSpec,
  onReject,
  hasExternalRepo = false,
  isArchiving = false,
  isStartingPlanning = false,
  isQueued = false,
  isPlanningFull = false,
  planningLimit,
  isBlockedByDependencies = false,
  blockedReason = "",
}: SpecTaskActionButtonsProps) {
  const approveImplementationMutation = useApproveImplementation(task.id);
  const stopAgentMutation = useStopAgent(task.id);

  const isArchived = task.archived ?? false;
  const isInline = variant === "inline";

  // Determine if this is a direct-push scenario (branch same as base) vs PR workflow
  const isDirectPush =
    !hasExternalRepo || task.base_branch === task.branch_name;

  // Button size based on variant
  const buttonSize = "small";
  const buttonSx = isInline
    ? { fontSize: "0.75rem", whiteSpace: "nowrap" }
    : { whiteSpace: "nowrap" };

  // Backlog phase: Start Planning button
  if (task.status === "backlog") {
    const isStartDisabled =
      isArchived || isPlanningFull || isStartingPlanning || isQueued;
    const startTooltip = isArchived
      ? "Task is archived"
      : isBlockedByDependencies
        ? blockedReason
        : isPlanningFull
          ? `Planning column at capacity (${planningLimit})`
          : "";

    const startLabel = isQueued
      ? "Queued"
      : isStartingPlanning
        ? "Starting..."
        : isBlockedByDependencies
          ? "Queue Planning"
          : task.metadata?.error
            ? task.just_do_it_mode
              ? "Retry"
              : "Retry Planning"
            : task.just_do_it_mode
              ? "Just Do It"
              : "Start Planning";

    if (isInline) {
      return (
        <Box sx={{ display: "flex", gap: 1 }}>
          <CompactActionButton
            tooltip={startTooltip}
            color={task.just_do_it_mode ? "success" : "warning"}
            icon={
              isQueued || isStartingPlanning ? (
                <CircularProgress size={18} color="inherit" />
              ) : (
                <PlayIcon sx={{ fontSize: 18 }} />
              )
            }
            label={startLabel}
            onClick={(e) => {
              e.stopPropagation();
              onStartPlanning?.();
            }}
            disabled={isStartDisabled}
          />
        </Box>
      );
    }

    return (
      <Box sx={isInline ? { display: "flex", gap: 1 } : { mt: 1.5 }}>
        <Tooltip title={startTooltip} placement="top">
          <span style={{ width: isInline ? "auto" : "100%" }}>
            <Button
              ref={startPlanningButtonRef}
              size={buttonSize}
              variant="contained"
              color={task.just_do_it_mode ? "success" : "warning"}
              startIcon={
                isQueued || isStartingPlanning ? (
                  <CircularProgress size={16} color="inherit" />
                ) : (
                  <PlayIcon />
                )
              }
              onClick={(e) => {
                e.stopPropagation();
                onStartPlanning?.();
              }}
              disabled={isStartDisabled}
              fullWidth={!isInline}
              sx={buttonSx}
            >
              {startLabel}
            </Button>
          </span>
        </Tooltip>
      </Box>
    );
  }

  // Review phase: Review Spec button (when design docs have been pushed)
  if (
    task.status === "spec_review" &&
    task.design_docs_pushed_at &&
    onReviewSpec
  ) {
    if (isInline) {
      return (
        <Box sx={{ display: "flex", gap: 1 }}>
          <CompactActionButton
            tooltip={isArchived ? "Task is archived" : ""}
            color="info"
            icon={
              isQueued ? (
                <CircularProgress size={18} color="inherit" />
              ) : (
                <SpecIcon sx={{ fontSize: 18 }} />
              )
            }
            label={isQueued ? "Queued" : "Review Spec"}
            onClick={(e) => {
              e.stopPropagation();
              onReviewSpec();
            }}
            disabled={isArchived || isQueued}
            sx={{
              animation: "pulse-glow 2s infinite",
              "@keyframes pulse-glow": {
                "0%, 100%": { boxShadow: "0 0 5px rgba(41, 182, 246, 0.5)" },
                "50%": { boxShadow: "0 0 15px rgba(41, 182, 246, 0.8)" },
              },
            }}
          />
        </Box>
      );
    }

    return (
      <Box sx={isInline ? { display: "flex", gap: 1 } : { mt: 1.5 }}>
        <Tooltip title={isArchived ? "Task is archived" : ""} placement="top">
          <span style={{ width: isInline ? "auto" : "100%", display: "block" }}>
            <Button
              size={buttonSize}
              variant="contained"
              color="info"
              startIcon={
                isQueued ? (
                  <CircularProgress size={16} color="inherit" />
                ) : (
                  <SpecIcon />
                )
              }
              onClick={(e) => {
                e.stopPropagation();
                onReviewSpec();
              }}
              disabled={isArchived || isQueued}
              fullWidth={!isInline}
              sx={{
                ...buttonSx,
                animation: "pulse-glow 2s infinite",
                "@keyframes pulse-glow": {
                  "0%, 100%": { boxShadow: "0 0 5px rgba(41, 182, 246, 0.5)" },
                  "50%": { boxShadow: "0 0 15px rgba(41, 182, 246, 0.8)" },
                },
              }}
            >
              {isQueued ? "Queued" : "Review Spec"}
            </Button>
          </span>
        </Tooltip>
      </Box>
    );
  }

  // Implementation phase: Reject + Open PR + View Spec buttons
  if (task.status === "implementation") {
    const hasDesignDocs = !!task.design_docs_pushed_at;

    if (isInline) {
      return (
        <Box sx={{ display: "flex", gap: 1 }}>
          <CompactActionButton
            tooltip={isArchived ? "Task is archived" : ""}
            variant="outlined"
            color="error"
            disabled={isArchived || isArchiving}
            icon={
              isArchiving ? (
                <CircularProgress size={16} color="inherit" />
              ) : (
                <CloseIcon sx={{ fontSize: 18 }} />
              )
            }
            label={isArchiving ? "Rejecting..." : "Reject"}
            onClick={(e) => {
              e.stopPropagation();
              onReject?.(e.shiftKey);
            }}
          />
          <CompactActionButton
            tooltip={isArchived ? "Task is archived" : ""}
            variant="contained"
            color="success"
            disabled={isArchived || approveImplementationMutation.isPending}
            icon={
              approveImplementationMutation.isPending ? (
                <CircularProgress size={16} color="inherit" />
              ) : (
                <ApproveIcon sx={{ fontSize: 18 }} />
              )
            }
            label={
              approveImplementationMutation.isPending
                ? isDirectPush
                  ? "Merging..."
                  : "Opening PR..."
                : isDirectPush
                  ? "Accept"
                  : "Open PR"
            }
            onClick={(e) => {
              e.stopPropagation();
              approveImplementationMutation.mutate();
            }}
          />
          {hasDesignDocs && onReviewSpec && (
            <CompactActionButton
              tooltip={isArchived ? "Task is archived" : ""}
              variant="outlined"
              color="primary"
              disabled={isArchived}
              icon={<SpecIcon sx={{ fontSize: 18 }} />}
              label="View Spec"
              onClick={(e) => {
                e.stopPropagation();
                onReviewSpec();
              }}
            />
          )}
        </Box>
      );
    }

    return (
      <Box
        sx={
          isInline
            ? { display: "flex", gap: 1 }
            : { mt: 1.5, display: "flex", flexDirection: "column", gap: 1 }
        }
      >
        <Box sx={{ display: "flex", gap: 1 }}>
          <Tooltip title={isArchived ? "Task is archived" : ""} placement="top">
            <span style={{ flex: 1 }}>
              <Button
                size={buttonSize}
                variant="outlined"
                color="error"
                disabled={isArchived || isArchiving}
                startIcon={
                  isArchiving ? (
                    <CircularProgress size={14} color="inherit" />
                  ) : (
                    <CloseIcon />
                  )
                }
                onClick={(e) => {
                  e.stopPropagation();
                  onReject?.(e.shiftKey);
                }}
                fullWidth
                sx={buttonSx}
              >
                {isArchiving ? "Rejecting..." : "Reject"}
              </Button>
            </span>
          </Tooltip>

          <Tooltip title={isArchived ? "Task is archived" : ""} placement="top">
            <span style={{ flex: 1 }}>
              <Button
                size={buttonSize}
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
                disabled={isArchived || approveImplementationMutation.isPending}
                fullWidth
                sx={buttonSx}
              >
                {approveImplementationMutation.isPending
                  ? isDirectPush
                    ? "Merging..."
                    : "Opening PR..."
                  : isDirectPush
                    ? "Accept"
                    : "Open PR"}
              </Button>
            </span>
          </Tooltip>

          {hasDesignDocs && onReviewSpec && (
            <Tooltip
              title={isArchived ? "Task is archived" : ""}
              placement="top"
            >
              <span>
                <Button
                  size={buttonSize}
                  variant="outlined"
                  startIcon={<SpecIcon />}
                  onClick={(e) => {
                    e.stopPropagation();
                    onReviewSpec();
                  }}
                  disabled={isArchived}
                  sx={buttonSx}
                >
                  View Spec
                </Button>
              </span>
            </Tooltip>
          )}
        </Box>
      </Box>
    );
  }

  // Pull Request phase: View Pull Request button
  if (task.status === "pull_request" && task.pull_request_url) {
    if (isInline) {
      return (
        <Box sx={{ display: "flex", gap: 1 }}>
          <CompactActionButton
            tooltip={isArchived ? "Task is archived" : ""}
            variant="contained"
            color="secondary"
            disabled={isArchived}
            icon={<LaunchIcon sx={{ fontSize: 18 }} />}
            label="Pull Request"
            onClick={(e) => {
              e.stopPropagation();
              window.open(task.pull_request_url, "_blank");
            }}
          />
        </Box>
      );
    }

    return (
      <Box sx={isInline ? { display: "flex", gap: 1 } : { mt: 1.5 }}>
        <Tooltip title={isArchived ? "Task is archived" : ""} placement="top">
          <span style={{ width: isInline ? "auto" : "100%", display: "block" }}>
            <Button
              size={buttonSize}
              variant="contained"
              color="secondary"
              startIcon={<LaunchIcon />}
              onClick={(e) => {
                e.stopPropagation();
                window.open(task.pull_request_url, "_blank");
              }}
              disabled={isArchived}
              fullWidth={!isInline}
              sx={buttonSx}
            >
              View Pull Request
            </Button>
          </span>
        </Tooltip>
      </Box>
    );
  }

  // No action buttons for other statuses
  return null;
}
