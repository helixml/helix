import React, { FC, useState, useMemo } from "react";
import Box from "@mui/material/Box";
import {
    prettyBytes,
    formatRelativeTime,
    formatDate,
} from "../../utils/format";
import IconButton from "@mui/material/IconButton";
import Button from "@mui/material/Button";
import VisibilityIcon from "@mui/icons-material/Visibility";
import ViewListIcon from "@mui/icons-material/ViewList";
import DeleteIcon from "@mui/icons-material/Delete";
import Typography from "@mui/material/Typography";
import SessionBadge from "./SessionBadge";
import JsonWindowLink from "../widgets/JsonWindowLink";
import Row from "../widgets/Row";
import Cell from "../widgets/Cell";
import ClickLink from "../widgets/ClickLink";
import Paper from "@mui/material/Paper";
import Grid from "@mui/material/Grid";
import CircularProgress from "@mui/material/CircularProgress";
import Dialog from "@mui/material/Dialog";
import DialogActions from "@mui/material/DialogActions";
import DialogContent from "@mui/material/DialogContent";
import DialogContentText from "@mui/material/DialogContentText";
import DialogTitle from "@mui/material/DialogTitle";
import Tooltip from "@mui/material/Tooltip";

import {
    IModelInstanceState,
    ISessionSummary,
    ISlot,
    SESSION_MODE_INFERENCE,
} from "../../types";
import { useFloatingModal } from "../../contexts/floatingModal";

import { TypesRunnerSlot, TypesRunnerModelStatus } from "../../api/api";
import { useDeleteSlot } from "../../services/dashboardService";

import {
    getColor,
    getHeadline,
    getSessionHeadline,
    getSummaryCaption,
    getModelInstanceIdleTime,
    shortID,
} from "../../utils/session";

export const ModelInstanceSummary: FC<{
    slot: TypesRunnerSlot;
    models?: TypesRunnerModelStatus[]; // Available models with memory info
    onViewSession: {
        (id: string): void;
    };
}> = ({ slot, models = [], onViewSession }) => {
    const floatingModal = useFloatingModal();
    const deleteSlot = useDeleteSlot();

    const [historyViewing, setHistoryViewing] = useState(false);
    const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

    // console.log(
    //     "ModelInstanceSummary rendering for slot:",
    //     slot.id,
    //     "with delete button",
    // );

    const statusColor = useMemo(() => {
        if (slot.active) {
            return "#F4D35E";
        }
        if (!slot.ready) {
            return "#E28000";
        }
        return "#e5e5e5";
    }, [slot.ready, slot.active]);

    // Get runtime specific color for the bullet
    const runtimeColor = useMemo(() => {
        // Convert runtime to lowercase to handle any case inconsistencies
        const runtime = slot.runtime?.toLowerCase() ?? "";

        // Match color based on runtime
        if (runtime.includes("vllm")) {
            return "#72C99A"; // Green for VLLM
        } else if (runtime.includes("ollama")) {
            return "#F4D35E"; // Yellow for Ollama
        } else if (runtime.includes("axolotl")) {
            return "#FF6B6B"; // Red for Axolotl
        } else if (runtime.includes("diffusers")) {
            return "#D183C9"; // Purple for Diffusers
        }

        // Default fallback color if runtime doesn't match
        return statusColor;
    }, [slot.runtime, statusColor]);

    // Look up memory for this slot's model
    const modelMemory = useMemo(() => {
        if (!slot.model || !models.length) return null;

        const modelData = models.find((m) => m.model_id === slot.model);
        return modelData?.memory || null;
    }, [slot.model, models]);

    // Create tooltip content for memory estimation (Ollama only)
    const memoryEstimationTooltip = useMemo(() => {
        console.log("Debug slot data:", {
            runtime: slot.runtime,
            model: slot.model,
            memory_estimation_meta: slot.memory_estimation_meta,
            model_memory_requirement: slot.model_memory_requirement,
            context_length: slot.context_length,
            tensor_parallel_size: slot.tensor_parallel_size,
        });

        if (slot.runtime !== "ollama" || !slot.memory_estimation_meta)
            return "";

        const meta = slot.memory_estimation_meta;
        const lines = [
            `Memory Estimation Details:`,
            `• Context Length: ${meta.context_length?.toLocaleString() || "N/A"}`,
            `• KV Cache Type: ${meta.kv_cache_type || "N/A"}`,
            `• Batch Size: ${meta.batch_size || "N/A"}`,
            `• Parallel Sequences: ${meta.parallel_sequences || "N/A"}`,
            `• GPU Allocation: ${meta.gpu_allocation_type || "N/A"}`,
            `• Estimation Source: ${meta.estimation_source || "N/A"}`,
        ];

        // Add detailed memory breakdown if available
        if (meta.detailed_breakdown) {
            const breakdown = meta.detailed_breakdown;
            lines.push(``, `Memory Breakdown:`);

            if (breakdown.total_memory) {
                lines.push(
                    `• Total Memory: ${breakdown.total_memory.gb?.toFixed(2)} GB (${breakdown.total_memory.mb?.toLocaleString()} MB)`,
                );
            }
            if (breakdown.weights_memory) {
                lines.push(
                    `• Model Weights: ${breakdown.weights_memory.gb?.toFixed(2)} GB (${breakdown.weights_memory.mb?.toLocaleString()} MB)`,
                );
            }
            if (breakdown.kv_cache_memory) {
                lines.push(
                    `• KV Cache: ${breakdown.kv_cache_memory.gb?.toFixed(2)} GB (${breakdown.kv_cache_memory.mb?.toLocaleString()} MB)`,
                );
            }
            if (breakdown.graph_memory) {
                lines.push(
                    `• Graph Memory: ${breakdown.graph_memory.gb?.toFixed(2)} GB (${breakdown.graph_memory.mb?.toLocaleString()} MB)`,
                );
            }
            if (breakdown.vram_used) {
                lines.push(
                    `• VRAM Used: ${breakdown.vram_used.gb?.toFixed(2)} GB (${breakdown.vram_used.mb?.toLocaleString()} MB)`,
                );
            }
            if (breakdown.layers_on_gpu !== undefined) {
                lines.push(`• Layers on GPU: ${breakdown.layers_on_gpu}`);
            }
            if (breakdown.fully_loaded !== undefined) {
                lines.push(
                    `• Fully Loaded: ${breakdown.fully_loaded ? "Yes" : "No"}`,
                );
            }
            if (breakdown.architecture) {
                lines.push(`• Architecture: ${breakdown.architecture}`);
            }
        }
        return lines.join("\n");
    }, [slot.runtime, slot.memory_estimation_meta]);

    // Enhanced gradient border based on status
    const borderGradient = useMemo(() => {
        if (slot.active) {
            return "linear-gradient(90deg, #F4D35E 0%, rgba(244, 211, 94, 0.7) 100%)";
        }
        if (!slot.ready) {
            return "linear-gradient(90deg, #E28000 0%, rgba(226, 128, 0, 0.7) 100%)";
        }
        return "linear-gradient(90deg, #e5e5e5 0%, rgba(229, 229, 229, 0.7) 100%)";
    }, [slot.ready, slot.active]);

    return (
        <Paper
            elevation={0}
            sx={{
                width: "calc(100% - 24px)",
                mx: 1.5,
                my: 1.5,
                backgroundColor: "rgba(30, 30, 32, 0.4)",
                position: "relative",
                overflow: "hidden",
                borderRadius: "3px",
                border: "1px solid",
                borderColor: (theme) =>
                    slot.active ? statusColor : "rgba(255, 255, 255, 0.05)",
                boxShadow: (theme) =>
                    slot.active ? `0 0 10px rgba(244, 211, 94, 0.15)` : "none",
            }}
        >
            <Box sx={{ p: 2.5, pl: 3 }}>
                <Grid container spacing={1}>
                    <Grid item xs>
                        <Typography
                            variant="subtitle1"
                            sx={{
                                fontWeight: 600,
                                color: "rgba(255, 255, 255, 0.9)",
                                display: "flex",
                                alignItems: "center",
                            }}
                        >
                            <Box
                                component="span"
                                sx={{
                                    display: "inline-block",
                                    width: 8,
                                    height: 8,
                                    borderRadius: "50%",
                                    backgroundColor: runtimeColor,
                                    mr: 1.5,
                                    boxShadow: (theme) =>
                                        slot.active
                                            ? `0 0 6px ${runtimeColor}`
                                            : "none",
                                }}
                            />
                            {slot.runtime}: {slot.model}
                            {modelMemory && (
                                <Tooltip
                                    title={
                                        memoryEstimationTooltip ||
                                        "Memory estimate"
                                    }
                                    placement="top"
                                    arrow
                                    enterDelay={500}
                                >
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            ml: 2,
                                            color: "rgba(255, 255, 255, 0.7)",
                                            backgroundColor:
                                                "rgba(114, 201, 154, 0.1)",
                                            border: "1px solid rgba(114, 201, 154, 0.3)",
                                            px: 1,
                                            py: 0.3,
                                            borderRadius: "3px",
                                            fontFamily: "monospace",
                                            fontSize: "0.7rem",
                                            cursor:
                                                slot.runtime === "ollama" &&
                                                memoryEstimationTooltip
                                                    ? "help"
                                                    : "default",
                                        }}
                                    >
                                        {prettyBytes(modelMemory)}
                                    </Typography>
                                </Tooltip>
                            )}
                            {/* Multi-GPU allocation display */}
                            {slot.gpu_indices &&
                                slot.gpu_indices.length > 1 && (
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            ml: 1,
                                            color: "rgba(255, 255, 255, 0.8)",
                                            backgroundColor:
                                                "rgba(0, 200, 255, 0.1)",
                                            border: "1px solid rgba(0, 200, 255, 0.3)",
                                            px: 1,
                                            py: 0.3,
                                            borderRadius: "3px",
                                            fontFamily: "monospace",
                                            fontSize: "0.7rem",
                                        }}
                                    >
                                        GPUs: {slot.gpu_indices.join(",")}
                                        {slot.tensor_parallel_size &&
                                            slot.tensor_parallel_size > 1 && (
                                                <>
                                                    {" "}
                                                    (TP:
                                                    {slot.tensor_parallel_size})
                                                </>
                                            )}
                                    </Typography>
                                )}
                            {/* Single GPU allocation display */}
                            {slot.gpu_index !== undefined &&
                                (!slot.gpu_indices ||
                                    slot.gpu_indices.length <= 1) && (
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            ml: 1,
                                            color: "rgba(255, 255, 255, 0.7)",
                                            backgroundColor:
                                                "rgba(156, 39, 176, 0.1)",
                                            border: "1px solid rgba(156, 39, 176, 0.3)",
                                            px: 1,
                                            py: 0.3,
                                            borderRadius: "3px",
                                            fontFamily: "monospace",
                                            fontSize: "0.7rem",
                                        }}
                                    >
                                        GPU: {slot.gpu_index}
                                    </Typography>
                                )}
                        </Typography>
                    </Grid>
                    <Grid item>
                        <Box
                            sx={{
                                display: "flex",
                                flexDirection: "column",
                                alignItems: "flex-end",
                                gap: 0.5,
                            }}
                        >
                            <Typography
                                variant="caption"
                                sx={{
                                    color: "rgba(255, 255, 255, 0.6)",
                                    fontFamily: "monospace",
                                    backgroundColor:
                                        "rgba(255, 255, 255, 0.05)",
                                    px: 1,
                                    py: 0.5,
                                    borderRadius: "2px",
                                }}
                            >
                                {slot.id}
                            </Typography>
                            {slot.created &&
                                (() => {
                                    const now = new Date();
                                    const created = new Date(slot.created);
                                    const diffHours =
                                        (now.getTime() - created.getTime()) /
                                        (1000 * 60 * 60);

                                    const isOld = diffHours > 24;
                                    const isStale = diffHours > 4;

                                    return (
                                        <Tooltip
                                            title={`Created: ${formatDate(slot.created)}`}
                                            placement="top"
                                            arrow
                                        >
                                            <Typography
                                                variant="caption"
                                                sx={{
                                                    color:
                                                        isOld || isStale
                                                            ? "rgba(255, 255, 255, 0.7)"
                                                            : "rgba(255, 255, 255, 0.5)",
                                                    fontSize: "0.65rem",
                                                    fontStyle: "italic",
                                                    fontWeight: 400,
                                                    cursor: "help",
                                                    backgroundColor:
                                                        "transparent",
                                                    px: 0,
                                                    py: 0,
                                                    borderRadius: "3px",
                                                }}
                                            >
                                                {formatRelativeTime(
                                                    slot.created,
                                                )}
                                            </Typography>
                                        </Tooltip>
                                    );
                                })()}
                        </Box>
                    </Grid>
                </Grid>

                {/* Runtime Arguments Display for VLLM */}
                {slot.runtime?.toLowerCase().includes("vllm") &&
                    slot.runtime_args &&
                    slot.runtime_args.args && (
                        <Box sx={{ mt: 1.5 }}>
                            <Typography
                                variant="caption"
                                sx={{
                                    color: "rgba(255, 255, 255, 0.8)",
                                    fontWeight: 500,
                                    mb: 0.5,
                                    display: "block",
                                }}
                            >
                                VLLM Arguments:
                            </Typography>
                            <Box
                                sx={{
                                    backgroundColor: "rgba(0, 0, 0, 0.3)",
                                    border: "1px solid rgba(114, 201, 154, 0.2)",
                                    borderRadius: "3px",
                                    p: 1,
                                    maxHeight: "100px",
                                    overflowY: "auto",
                                }}
                            >
                                <Typography
                                    variant="caption"
                                    sx={{
                                        fontFamily: "monospace",
                                        fontSize: "0.65rem",
                                        color: "rgba(255, 255, 255, 0.9)",
                                        wordBreak: "break-all",
                                        lineHeight: 1.2,
                                    }}
                                >
                                    {Array.isArray(slot.runtime_args.args)
                                        ? slot.runtime_args.args.join(" ")
                                        : JSON.stringify(
                                              slot.runtime_args.args,
                                          )}
                                </Typography>
                            </Box>
                        </Box>
                    )}

                {/* Action Buttons */}
                <Box sx={{ mt: 1, display: "flex", gap: 1 }}>
                    <Button
                        variant="outlined"
                        size="small"
                        onClick={(e) => {
                            const rect =
                                e.currentTarget.getBoundingClientRect();
                            const clickPosition = {
                                x: rect.left - 500, // Position floating window to the left of button
                                y: rect.top - 300, // Position above the button
                            };
                            floatingModal.showFloatingModal(
                                {
                                    type: "logs",
                                    runner: {
                                        id: `runner-for-${slot.id}`,
                                        slots: [slot],
                                    },
                                },
                                clickPosition,
                            );
                        }}
                        startIcon={<ViewListIcon />}
                        sx={{
                            fontSize: "0.7rem",
                            borderColor: "rgba(255, 255, 255, 0.2)",
                            color: "rgba(255, 255, 255, 0.7)",
                            "&:hover": {
                                borderColor: "rgba(255, 255, 255, 0.4)",
                                backgroundColor: "rgba(255, 255, 255, 0.05)",
                            },
                        }}
                    >
                        View Logs
                    </Button>

                    <Button
                        variant="outlined"
                        size="small"
                        onClick={() => setDeleteDialogOpen(true)}
                        startIcon={<DeleteIcon />}
                        sx={{
                            fontSize: "0.7rem",
                            borderColor: "rgba(255, 100, 100, 0.3)",
                            color: "rgba(255, 100, 100, 0.8)",
                            "&:hover": {
                                borderColor: "rgba(255, 100, 100, 0.5)",
                                backgroundColor: "rgba(255, 100, 100, 0.05)",
                            },
                        }}
                    >
                        Delete
                    </Button>
                </Box>

                {/* Delete Confirmation Dialog */}
                <Dialog
                    open={deleteDialogOpen}
                    onClose={() => setDeleteDialogOpen(false)}
                    PaperProps={{
                        sx: {
                            backgroundColor: "rgba(30, 30, 32, 0.95)",
                            border: "1px solid rgba(255, 255, 255, 0.1)",
                        },
                    }}
                >
                    <DialogTitle sx={{ color: "rgba(255, 255, 255, 0.9)" }}>
                        Delete Slot
                    </DialogTitle>
                    <DialogContent>
                        <DialogContentText
                            sx={{ color: "rgba(255, 255, 255, 0.7)" }}
                        >
                            Are you sure you want to delete this slot? This will
                            remove it from the scheduler's desired state and the
                            reconciler will clean it up from the runner.
                            <br />
                            <br />
                            <strong>Slot:</strong> {slot.id}
                            <br />
                            <strong>Model:</strong> {slot.runtime}: {slot.model}
                        </DialogContentText>
                    </DialogContent>
                    <DialogActions>
                        <Button
                            onClick={() => setDeleteDialogOpen(false)}
                            sx={{ color: "rgba(255, 255, 255, 0.7)" }}
                        >
                            Cancel
                        </Button>
                        <Button
                            onClick={async () => {
                                try {
                                    await deleteSlot.mutateAsync(slot.id || "");
                                    setDeleteDialogOpen(false);
                                } catch (error) {
                                    console.error(
                                        "Failed to delete slot:",
                                        error,
                                    );
                                    // You might want to show an error snackbar here
                                }
                            }}
                            disabled={deleteSlot.isPending}
                            sx={{
                                color: "rgba(255, 100, 100, 0.8)",
                                "&:hover": {
                                    backgroundColor: "rgba(255, 100, 100, 0.1)",
                                },
                            }}
                        >
                            {deleteSlot.isPending ? "Deleting..." : "Delete"}
                        </Button>
                    </DialogActions>
                </Dialog>

                <Box
                    sx={{
                        display: "flex",
                        alignItems: "center",
                        mt: 1.5,
                        gap: 1,
                    }}
                >
                    <Typography
                        variant="caption"
                        sx={{
                            color: slot.active
                                ? statusColor
                                : "rgba(255, 255, 255, 0.6)",
                            fontWeight: slot.active ? 500 : 400,
                            px: 1.5,
                            py: 0.7,
                            borderRadius: "3px",
                            border: "1px solid",
                            borderColor: slot.active
                                ? `${statusColor}40`
                                : "transparent",
                            backgroundColor: slot.active
                                ? `${statusColor}10`
                                : "transparent",
                        }}
                    >
                        {slot.status}
                    </Typography>

                    {/* Concurrency Information for VLLM and Ollama */}
                    {(slot.runtime === "vllm" || slot.runtime === "ollama") &&
                        slot.active_requests !== undefined &&
                        slot.max_concurrency !== undefined && (
                            <Tooltip
                                title={`Currently processing ${slot.active_requests} out of ${slot.max_concurrency} maximum concurrent requests. This model instance can handle multiple requests simultaneously for better throughput.`}
                                arrow
                            >
                                <Typography
                                    variant="caption"
                                    sx={{
                                        color: "rgba(255, 255, 255, 0.7)",
                                        fontSize: "0.7rem",
                                        px: 1,
                                        py: 0.3,
                                        borderRadius: "2px",
                                        backgroundColor:
                                            "rgba(0, 200, 255, 0.1)",
                                        border: "1px solid rgba(0, 200, 255, 0.2)",
                                        fontFamily: "monospace",
                                        cursor: "help",
                                    }}
                                >
                                    {slot.active_requests}/
                                    {slot.max_concurrency} requests
                                </Typography>
                            </Tooltip>
                        )}

                    {!slot.ready && !slot.active && (
                        <CircularProgress
                            size={14}
                            thickness={4}
                            sx={{
                                color: "#E28000",
                            }}
                        />
                    )}
                </Box>
            </Box>
        </Paper>
    );
};

export default ModelInstanceSummary;

// TODO(phil): Old functionality
// Model Instance Display
// - Shows model name and mode via SessionBadge
// - Displays memory usage in a readable format
// - Shows current status code
// - Different border styling for active vs inactive instances
// Session Information
// - When a session is active:
//   - Displays session headline in bold
// Shows additional session summary caption
// When idle:
// Shows model headline with name, mode, and LoRA directory
// Displays idle time duration
// Job History Functionality
// Toggleable job history view with "view jobs"/"hide jobs" button
// Shows count of jobs (properly pluralized)
// History displayed in scrollable container (max height 100px)
// - Jobs are displayed in reverse chronological order
// - Per Job Entry Display
//   - Time stamp (HH:MM:SS format)
//   - Session and interaction IDs (shortened format)
//   - Interactive JSON viewer for detailed job data
//   - Quick "view session" button (eye icon) to navigate to full session view
// Layout Features
// - Responsive grid layout using Row and Cell components
// - Consistent typography styling with MUI components
// - Compact line heights (lineHeight: 1)
// - Right-aligned job history toggle
// State Management
// Uses local state for history view toggle
// Memoized job history array to prevent unnecessary recalculations
