import React, { FC, useState, useEffect } from "react";
import Box from "@mui/material/Box";
import Paper from "@mui/material/Paper";
import Typography from "@mui/material/Typography";
import IconButton from "@mui/material/IconButton";

import Collapse from "@mui/material/Collapse";
import LinearProgress from "@mui/material/LinearProgress";
import Chip from "@mui/material/Chip";
import Tooltip from "@mui/material/Tooltip";
import CircularProgress from "@mui/material/CircularProgress";
import MinimizeIcon from "@mui/icons-material/Minimize";
import MaximizeIcon from "@mui/icons-material/CropFree";
import CloseIcon from "@mui/icons-material/Close";
import DnsIcon from "@mui/icons-material/Dns";
import LaunchIcon from "@mui/icons-material/Launch";

import { prettyBytes } from "../../utils/format";
import { useGetDashboardData } from "../../services/dashboardService";
import { TypesDashboardRunner, TypesGPUStatus } from "../../api/api";
import useRouter from "../../hooks/useRouter";
import { useFloatingRunnerState } from "../../contexts/floatingRunnerState";
import { useResize } from "../../hooks/useResize";

interface FloatingRunnerStateProps {
    onClose?: () => void;
}

const FloatingRunnerState: FC<FloatingRunnerStateProps> = ({ onClose }) => {
    const floatingRunnerState = useFloatingRunnerState();
    const [isMinimized, setIsMinimized] = useState(false);
    const [isDragging, setIsDragging] = useState(false);

    // Use click position from context if available, otherwise use default position
    const getInitialPosition = () => {
        if (floatingRunnerState.clickPosition) {
            return {
                x: Math.max(
                    0,
                    Math.min(
                        floatingRunnerState.clickPosition.x,
                        window.innerWidth - 340,
                    ),
                ),
                y: Math.max(
                    0,
                    Math.min(
                        floatingRunnerState.clickPosition.y,
                        window.innerHeight - 420,
                    ),
                ),
            };
        }
        return {
            x: window.innerWidth - 340,
            y: window.innerHeight - 420,
        };
    };

    const [position, setPosition] = useState(getInitialPosition);
    const [dragOffset, setDragOffset] = useState({ x: 0, y: 0 });

    const { size, setSize, isResizing, getResizeHandles } = useResize({
        initialSize: { width: 340, height: 420 },
        minSize: { width: 280, height: 200 },
        maxSize: { width: 600, height: window.innerHeight - 40 },
        onResize: (newSize, direction, delta) => {
            // Adjust position when resizing from top or left edges
            if (direction.includes("w") || direction.includes("n")) {
                setPosition((prev) => ({
                    x: direction.includes("w")
                        ? prev.x + (size.width - newSize.width)
                        : prev.x,
                    y: direction.includes("n")
                        ? prev.y + (size.height - newSize.height)
                        : prev.y,
                }));
            }
        },
    });

    // Update position when click position changes
    useEffect(() => {
        if (floatingRunnerState.clickPosition) {
            setPosition({
                x: Math.max(
                    0,
                    Math.min(
                        floatingRunnerState.clickPosition.x,
                        window.innerWidth - size.width,
                    ),
                ),
                y: Math.max(
                    0,
                    Math.min(
                        floatingRunnerState.clickPosition.y,
                        window.innerHeight - size.height,
                    ),
                ),
            });
        }
    }, [floatingRunnerState.clickPosition, size.width, size.height]);

    const { data: dashboardData, isLoading } = useGetDashboardData();
    const router = useRouter();

    const handleMouseDown = (e: React.MouseEvent) => {
        if (isResizing) return; // Don't allow dragging when resizing
        setIsDragging(true);
        setDragOffset({
            x: e.clientX - position.x,
            y: e.clientY - position.y,
        });
    };

    const handleViewFullDashboard = () => {
        router.navigate("dashboard", { tab: "runners" });
    };

    useEffect(() => {
        const handleMouseMove = (e: MouseEvent) => {
            if (isDragging && !isResizing) {
                const newX = e.clientX - dragOffset.x;
                const newY = e.clientY - dragOffset.y;

                // Keep within bounds
                const boundedX = Math.max(
                    0,
                    Math.min(newX, window.innerWidth - size.width),
                );
                const boundedY = Math.max(
                    0,
                    Math.min(newY, window.innerHeight - size.height),
                );

                setPosition({
                    x: boundedX,
                    y: boundedY,
                });
            }
        };

        const handleMouseUp = () => {
            setIsDragging(false);
        };

        if (isDragging) {
            document.addEventListener("mousemove", handleMouseMove);
            document.addEventListener("mouseup", handleMouseUp);
        }

        return () => {
            document.removeEventListener("mousemove", handleMouseMove);
            document.removeEventListener("mouseup", handleMouseUp);
        };
    }, [isDragging, dragOffset, isResizing, size]);

    const GPUDisplay: FC<{
        gpu: TypesGPUStatus;
        allocatedMemory: number;
        allocatedModels: Array<{
            model: string;
            memory: number;
            isMultiGPU: boolean;
        }>;
    }> = ({ gpu, allocatedMemory, allocatedModels }) => {
        const total_memory = gpu.total_memory || 1;
        const used_memory = gpu.used_memory || 0;
        const free_memory = gpu.free_memory || 0;

        const usedPercent = Math.round((used_memory / total_memory) * 100);
        const allocatedPercent = Math.round(
            (allocatedMemory / total_memory) * 100,
        );

        // Create tooltip content for allocated models
        const allocatedTooltip =
            allocatedModels.length > 0 ? (
                <Box>
                    <Typography
                        variant="caption"
                        sx={{ fontWeight: 600, display: "block", mb: 0.5 }}
                    >
                        Allocated Models:
                    </Typography>
                    {allocatedModels.map((modelInfo, idx) => (
                        <Typography
                            key={idx}
                            variant="caption"
                            sx={{ display: "block", fontSize: "0.6rem" }}
                        >
                            • {modelInfo.model} ({prettyBytes(modelInfo.memory)}
                            )
                            {modelInfo.isMultiGPU && (
                                <span style={{ color: "#7986cb" }}>
                                    {" "}
                                    [Multi-GPU]
                                </span>
                            )}
                        </Typography>
                    ))}
                </Box>
            ) : null;

        // Simplify GPU model name for display
        const getSimpleModelName = (fullName: string) => {
            if (!fullName || fullName === "unknown") return "Unknown";

            // Extract key parts: H100, RTX 4090, RX 7900 XTX, etc. (vendor-neutral)
            const name = fullName
                .replace(/^(NVIDIA|AMD|Intel)\s+/, "")  // Remove vendor prefix
                .replace(/^Radeon\s+/, "")               // Remove "Radeon" prefix for AMD
                .replace(/\s+PCIe.*$/, "")               // Remove PCIe details
                .replace(/\s+SXM.*$/, "");               // Remove SXM details
            return name.length > 12 ? name.substring(0, 12) + "..." : name;
        };

        return (
            <Box
                sx={{
                    mb: 0.5,
                    p: 0.75,
                    backgroundColor: "rgba(0, 0, 0, 0.15)",
                    borderRadius: 0.5,
                    border: "1px solid rgba(255, 255, 255, 0.05)",
                }}
            >
                <Box
                    sx={{
                        display: "flex",
                        alignItems: "center",
                        justifyContent: "space-between",
                        mb: 0.5,
                    }}
                >
                    <Box>
                        <Typography
                            variant="caption"
                            sx={{
                                fontWeight: 600,
                                fontSize: "0.65rem",
                                color: "#00c8ff",
                                display: "block",
                                lineHeight: 1,
                            }}
                        >
                            GPU {gpu.index}
                        </Typography>
                        <Tooltip
                            title={`${gpu.model_name || "Unknown GPU"} • Driver: ${gpu.driver_version || "unknown"} • SDK: ${gpu.sdk_version || "unknown"}`}
                        >
                            <Typography
                                variant="caption"
                                sx={{
                                    fontSize: "0.55rem",
                                    color: "rgba(255, 255, 255, 0.5)",
                                    cursor: "help",
                                }}
                            >
                                {getSimpleModelName(gpu.model_name || "")}
                            </Typography>
                        </Tooltip>
                    </Box>
                    <Chip
                        size="small"
                        label={`${usedPercent}%`}
                        sx={{
                            height: 14,
                            fontSize: "0.55rem",
                            backgroundColor:
                                usedPercent > 80
                                    ? "rgba(255, 0, 0, 0.2)"
                                    : "rgba(0, 200, 255, 0.2)",
                            color: usedPercent > 80 ? "#ff6b6b" : "#00c8ff",
                            border: `1px solid ${usedPercent > 80 ? "rgba(255, 0, 0, 0.3)" : "rgba(0, 200, 255, 0.3)"}`,
                        }}
                    />
                </Box>

                <Box sx={{ position: "relative", height: 8, mb: 0.5 }}>
                    <Box
                        sx={{
                            position: "absolute",
                            width: "100%",
                            height: "100%",
                            backgroundColor: "rgba(255, 255, 255, 0.03)",
                            borderRadius: "2px",
                            boxShadow: "inset 0 1px 2px rgba(0,0,0,0.3)",
                        }}
                    />

                    {/* Allocated memory bar */}
                    <Tooltip title={allocatedTooltip || "No models allocated"}>
                        <LinearProgress
                            variant="determinate"
                            value={allocatedPercent}
                            sx={{
                                width: "100%",
                                height: "100%",
                                borderRadius: "2px",
                                backgroundColor: "transparent",
                                cursor: allocatedTooltip ? "help" : "default",
                                "& .MuiLinearProgress-bar": {
                                    background:
                                        "linear-gradient(90deg, rgba(121,134,203,0.9) 0%, rgba(121,134,203,0.7) 100%)",
                                    borderRadius: "2px",
                                    transition:
                                        "transform 0.5s cubic-bezier(0.4, 0, 0.2, 1)",
                                    boxShadow: "0 0 4px rgba(121,134,203,0.5)",
                                },
                            }}
                        />
                    </Tooltip>

                    {/* Used memory bar */}
                    <LinearProgress
                        variant="determinate"
                        value={usedPercent}
                        sx={{
                            position: "absolute",
                            width: "100%",
                            height: 5,
                            top: 1.5,
                            borderRadius: "2px",
                            backgroundColor: "transparent",
                            "& .MuiLinearProgress-bar": {
                                background:
                                    usedPercent > 80
                                        ? "linear-gradient(90deg, rgba(255,107,107,1) 0%, rgba(255,107,107,0.8) 100%)"
                                        : "linear-gradient(90deg, rgba(0,200,255,1) 0%, rgba(0,200,255,0.8) 100%)",
                                borderRadius: "2px",
                                boxShadow:
                                    usedPercent > 80
                                        ? "0 0 6px rgba(255,107,107,0.7)"
                                        : "0 0 6px rgba(0,200,255,0.7)",
                                transition:
                                    "transform 0.5s cubic-bezier(0.4, 0, 0.2, 1)",
                            },
                        }}
                    />
                </Box>

                <Box
                    sx={{
                        display: "flex",
                        flexDirection: "column",
                        gap: 0.125,
                    }}
                >
                    <Box
                        sx={{
                            display: "flex",
                            justifyContent: "space-between",
                        }}
                    >
                        <Typography
                            variant="caption"
                            sx={{
                                fontSize: "0.55rem",
                                color: "#00c8ff",
                                fontWeight: 600,
                            }}
                        >
                            Used: {prettyBytes(used_memory)}
                        </Typography>
                    </Box>
                    {allocatedMemory > 0 && (
                        <Box
                            sx={{
                                display: "flex",
                                justifyContent: "space-between",
                            }}
                        >
                            <Typography
                                variant="caption"
                                sx={{
                                    fontSize: "0.55rem",
                                    color: "#7986cb",
                                    fontWeight: 600,
                                }}
                            >
                                Allocated: {prettyBytes(allocatedMemory)}
                            </Typography>
                        </Box>
                    )}
                    <Box
                        sx={{
                            display: "flex",
                            justifyContent: "space-between",
                        }}
                    >
                        <Typography
                            variant="caption"
                            sx={{
                                fontSize: "0.55rem",
                                color: "rgba(255, 255, 255, 0.4)",
                            }}
                        >
                            Total: {prettyBytes(total_memory)}
                        </Typography>
                    </Box>
                </Box>
            </Box>
        );
    };

    // Calculate allocated memory per GPU based on running slots
    const calculateGPUAllocatedMemory = (
        runner: TypesDashboardRunner,
    ): Map<number, number> => {
        const gpuAllocatedMemory = new Map<number, number>();

        if (!runner.slots || !runner.models) {
            return gpuAllocatedMemory;
        }

        // Create a map of model ID to memory requirement
        const modelMemoryMap = new Map<string, number>();
        runner.models.forEach((model) => {
            if (model.model_id && model.memory) {
                modelMemoryMap.set(model.model_id, model.memory);
            }
        });

        // Calculate allocated memory per GPU
        runner.slots.forEach((slot) => {
            if (!slot.model) return;

            const modelMemory = modelMemoryMap.get(slot.model);
            if (!modelMemory) return;

            if (slot.gpu_indices && slot.gpu_indices.length > 1) {
                // Multi-GPU model: distribute memory across GPUs
                const memoryPerGPU = modelMemory / slot.gpu_indices.length;
                slot.gpu_indices.forEach((gpuIndex) => {
                    const current = gpuAllocatedMemory.get(gpuIndex) || 0;
                    gpuAllocatedMemory.set(gpuIndex, current + memoryPerGPU);
                });
            } else if (slot.gpu_index !== undefined) {
                // Single GPU model: allocate full memory to this GPU
                const current = gpuAllocatedMemory.get(slot.gpu_index) || 0;
                gpuAllocatedMemory.set(slot.gpu_index, current + modelMemory);
            }
        });

        return gpuAllocatedMemory;
    };

    // Calculate which models are allocated to each GPU
    const calculateGPUAllocatedModels = (
        runner: TypesDashboardRunner,
    ): Map<
        number,
        Array<{ model: string; memory: number; isMultiGPU: boolean }>
    > => {
        const gpuAllocatedModels = new Map<
            number,
            Array<{ model: string; memory: number; isMultiGPU: boolean }>
        >();

        if (!runner.slots || !runner.models) {
            return gpuAllocatedModels;
        }

        // Create a map of model ID to memory requirement
        const modelMemoryMap = new Map<string, number>();
        runner.models.forEach((model) => {
            if (model.model_id && model.memory) {
                modelMemoryMap.set(model.model_id, model.memory);
            }
        });

        // Track models per GPU
        runner.slots.forEach((slot) => {
            if (!slot.model) return;

            const modelMemory = modelMemoryMap.get(slot.model);
            if (!modelMemory) return;

            if (slot.gpu_indices && slot.gpu_indices.length > 1) {
                // Multi-GPU model: add to all GPUs
                const memoryPerGPU = modelMemory / slot.gpu_indices.length;
                slot.gpu_indices.forEach((gpuIndex) => {
                    if (!gpuAllocatedModels.has(gpuIndex)) {
                        gpuAllocatedModels.set(gpuIndex, []);
                    }
                    gpuAllocatedModels.get(gpuIndex)!.push({
                        model: slot.model!,
                        memory: memoryPerGPU,
                        isMultiGPU: true,
                    });
                });
            } else if (slot.gpu_index !== undefined) {
                // Single GPU model: add to specific GPU
                if (!gpuAllocatedModels.has(slot.gpu_index)) {
                    gpuAllocatedModels.set(slot.gpu_index, []);
                }
                gpuAllocatedModels.get(slot.gpu_index)!.push({
                    model: slot.model,
                    memory: modelMemory,
                    isMultiGPU: false,
                });
            }
        });

        return gpuAllocatedModels;
    };

    const CompactRunnerDisplay: FC<{ runner: TypesDashboardRunner }> = ({
        runner,
    }) => {
        const total_memory = runner.total_memory || 1;
        const used_memory =
            typeof runner.used_memory === "number" ? runner.used_memory : 0;
        const allocated_memory =
            !runner.slots ||
            runner.slots.length === 0 ||
            !runner.allocated_memory
                ? 0
                : runner.allocated_memory;

        const actualPercent = Math.round((used_memory / total_memory) * 100);
        const allocatedPercent = Math.round(
            (allocated_memory / total_memory) * 100,
        );

        const getModelMemory = (slotModel: string): number | undefined => {
            if (!runner.models) return undefined;

            const matchingModel = runner.models.find(
                (model) =>
                    model.model_id === slotModel ||
                    model.model_id?.split(":")[0] === slotModel?.split(":")[0],
            );

            return matchingModel?.memory;
        };

        // Check if we have per-GPU data
        const hasPerGPUData = runner.gpus && runner.gpus.length > 0;

        return (
            <Box
                sx={{
                    mb: 1,
                    p: 1,
                    backgroundColor: "rgba(0, 0, 0, 0.1)",
                    borderRadius: 1,
                }}
            >
                <Box sx={{ display: "flex", alignItems: "center", mb: 0.5 }}>
                    <DnsIcon sx={{ fontSize: 14, mr: 0.5, color: "#00c8ff" }} />
                    <Typography
                        variant="caption"
                        sx={{ fontWeight: 600, mr: 1, fontSize: "0.7rem" }}
                    >
                        {runner.id?.slice(0, 8)}...
                    </Typography>
                    {hasPerGPUData ? (
                        <Chip
                            size="small"
                            label={`${runner.gpus?.length || 0} GPUs`}
                            sx={{
                                height: 16,
                                fontSize: "0.6rem",
                                backgroundColor: "rgba(0, 200, 255, 0.2)",
                                color: "#00c8ff",
                                border: "1px solid rgba(0, 200, 255, 0.3)",
                            }}
                        />
                    ) : (
                        <Chip
                            size="small"
                            label={`${actualPercent}%`}
                            sx={{
                                height: 16,
                                fontSize: "0.6rem",
                                backgroundColor:
                                    actualPercent > 80
                                        ? "rgba(255, 0, 0, 0.2)"
                                        : "rgba(0, 200, 255, 0.2)",
                                color:
                                    actualPercent > 80 ? "#ff6b6b" : "#00c8ff",
                                border: `1px solid ${actualPercent > 80 ? "rgba(255, 0, 0, 0.3)" : "rgba(0, 200, 255, 0.3)"}`,
                            }}
                        />
                    )}
                </Box>

                {hasPerGPUData ? (
                    // Show per-GPU visualization
                    <Box>
                        {(() => {
                            const gpuAllocatedMemory =
                                calculateGPUAllocatedMemory(runner);
                            const gpuAllocatedModels =
                                calculateGPUAllocatedModels(runner);
                            // Sort GPUs by index to ensure consistent ordering (GPU 0, GPU 1, etc.)
                            const sortedGPUs = [...(runner.gpus || [])].sort(
                                (a, b) => (a.index || 0) - (b.index || 0),
                            );
                            return sortedGPUs.map((gpu) => (
                                <GPUDisplay
                                    key={gpu.index}
                                    gpu={gpu}
                                    allocatedMemory={
                                        gpuAllocatedMemory.get(
                                            gpu.index || 0,
                                        ) || 0
                                    }
                                    allocatedModels={
                                        gpuAllocatedModels.get(
                                            gpu.index || 0,
                                        ) || []
                                    }
                                />
                            ));
                        })()}
                    </Box>
                ) : (
                    // Fallback to aggregated memory display
                    <>
                        <Box sx={{ position: "relative", height: 12, mb: 0.5 }}>
                            <Box
                                sx={{
                                    position: "absolute",
                                    width: "100%",
                                    height: "100%",
                                    backgroundColor:
                                        "rgba(255, 255, 255, 0.03)",
                                    borderRadius: "4px",
                                    boxShadow:
                                        "inset 0 1px 3px rgba(0,0,0,0.3)",
                                }}
                            />

                            <LinearProgress
                                variant="determinate"
                                value={allocatedPercent}
                                sx={{
                                    width: "100%",
                                    height: "100%",
                                    borderRadius: "4px",
                                    backgroundColor: "transparent",
                                    "& .MuiLinearProgress-bar": {
                                        background:
                                            "linear-gradient(90deg, rgba(121,134,203,0.9) 0%, rgba(121,134,203,0.7) 100%)",
                                        borderRadius: "4px",
                                        transition:
                                            "transform 0.5s cubic-bezier(0.4, 0, 0.2, 1)",
                                        boxShadow:
                                            "0 0 10px rgba(121,134,203,0.5)",
                                    },
                                }}
                            />

                            <LinearProgress
                                variant="determinate"
                                value={actualPercent}
                                sx={{
                                    position: "absolute",
                                    width: "100%",
                                    height: 6,
                                    top: 3,
                                    borderRadius: "4px",
                                    backgroundColor: "transparent",
                                    "& .MuiLinearProgress-bar": {
                                        background:
                                            "linear-gradient(90deg, rgba(0,200,255,1) 0%, rgba(0,200,255,0.8) 100%)",
                                        borderRadius: "4px",
                                        boxShadow:
                                            "0 0 10px rgba(0,200,255,0.7)",
                                        transition:
                                            "transform 0.5s cubic-bezier(0.4, 0, 0.2, 1)",
                                    },
                                }}
                            />
                        </Box>

                        <Box
                            sx={{
                                display: "flex",
                                justifyContent: "space-between",
                                mb: 0.5,
                            }}
                        >
                            <Typography
                                variant="caption"
                                sx={{
                                    fontSize: "0.6rem",
                                    color: "rgba(255, 255, 255, 0.6)",
                                }}
                            >
                                Used: {prettyBytes(used_memory)} (
                                {actualPercent}%)
                            </Typography>
                            <Typography
                                variant="caption"
                                sx={{
                                    fontSize: "0.6rem",
                                    color: "rgba(255, 255, 255, 0.4)",
                                }}
                            >
                                Total: {prettyBytes(total_memory)}
                            </Typography>
                        </Box>

                        {allocated_memory > 0 && (
                            <Typography
                                variant="caption"
                                sx={{
                                    fontSize: "0.6rem",
                                    color: "#7986cb",
                                    display: "block",
                                    mb: 0.5,
                                }}
                            >
                                Allocated: {prettyBytes(allocated_memory)} (
                                {allocatedPercent}%)
                            </Typography>
                        )}
                    </>
                )}

                {/* Process Cleanup Stats */}
                {runner.process_stats && (
                    <Box sx={{ mt: 0.5, mb: 0.5 }}>
                        <Typography
                            variant="caption"
                            sx={{
                                fontSize: "0.6rem",
                                color: "rgba(255, 255, 255, 0.4)",
                                mb: 0.25,
                                display: "block",
                            }}
                        >
                            Process Cleanup:
                        </Typography>
                        <Box
                            sx={{
                                display: "flex",
                                flexWrap: "wrap",
                                gap: 0.25,
                                alignItems: "center",
                            }}
                        >
                            <Chip
                                size="small"
                                label={`${runner.process_stats.cleanup_stats?.total_cleaned || 0} cleaned`}
                                sx={{
                                    height: 14,
                                    fontSize: "0.55rem",
                                    backgroundColor:
                                        (runner.process_stats.cleanup_stats
                                            ?.total_cleaned || 0) > 0
                                            ? "rgba(255, 152, 0, 0.15)"
                                            : "rgba(76, 175, 80, 0.15)",
                                    color:
                                        (runner.process_stats.cleanup_stats
                                            ?.total_cleaned || 0) > 0
                                            ? "#FF9800"
                                            : "#4CAF50",
                                    border: `1px solid ${(runner.process_stats.cleanup_stats?.total_cleaned || 0) > 0 ? "rgba(255, 152, 0, 0.3)" : "rgba(76, 175, 80, 0.3)"}`,
                                    "& .MuiChip-label": { px: 0.5 },
                                }}
                            />
                            <Chip
                                size="small"
                                label={`${runner.process_stats.total_tracked_processes || 0} tracked`}
                                sx={{
                                    height: 14,
                                    fontSize: "0.55rem",
                                    backgroundColor: "rgba(0, 200, 255, 0.15)",
                                    color: "#00c8ff",
                                    border: "1px solid rgba(0, 200, 255, 0.3)",
                                    "& .MuiChip-label": { px: 0.5 },
                                }}
                            />
                            {runner.process_stats.cleanup_stats
                                ?.last_cleanup_time && (
                                <Typography
                                    variant="caption"
                                    sx={{
                                        fontSize: "0.5rem",
                                        color: "rgba(255, 255, 255, 0.3)",
                                    }}
                                >
                                    Last:{" "}
                                    {new Date(
                                        runner.process_stats.cleanup_stats.last_cleanup_time,
                                    ).toLocaleTimeString()}
                                </Typography>
                            )}
                        </Box>
                        {runner.process_stats.cleanup_stats?.recent_cleanups &&
                            runner.process_stats.cleanup_stats.recent_cleanups
                                .length > 0 && (
                                <Box sx={{ mt: 0.25 }}>
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            fontSize: "0.5rem",
                                            color: "rgba(255, 255, 255, 0.3)",
                                            display: "block",
                                        }}
                                    >
                                        Recent:{" "}
                                        {runner.process_stats.cleanup_stats.recent_cleanups
                                            .slice(-3)
                                            .map(
                                                (cleanup: any) =>
                                                    `PID ${cleanup.pid} (${cleanup.method})`,
                                            )
                                            .join(", ")}
                                    </Typography>
                                </Box>
                            )}
                    </Box>
                )}

                {runner.slots && runner.slots.length > 0 && (
                    <Box sx={{ mt: 0.5 }}>
                        <Typography
                            variant="caption"
                            sx={{
                                fontSize: "0.6rem",
                                color: "rgba(255, 255, 255, 0.4)",
                                mb: 0.25,
                                display: "block",
                            }}
                        >
                            Running Models:
                        </Typography>
                        <Box
                            sx={{
                                display: "flex",
                                flexWrap: "wrap",
                                gap: 0.25,
                            }}
                        >
                            {runner.slots.slice(0, 3).map((slot, index) => {
                                const modelMemory = getModelMemory(
                                    slot.model || "",
                                );
                                const modelName =
                                    slot.model?.split(":")[0] ||
                                    slot.runtime ||
                                    "unknown";
                                const memoryDisplay = modelMemory
                                    ? ` (${prettyBytes(modelMemory)})`
                                    : "";

                                // GPU allocation display
                                let gpuDisplay = "";
                                if (
                                    slot.gpu_indices &&
                                    slot.gpu_indices.length > 1
                                ) {
                                    gpuDisplay = ` [GPUs:${slot.gpu_indices.join(",")}]`;
                                    if (
                                        slot.tensor_parallel_size &&
                                        slot.tensor_parallel_size > 1
                                    ) {
                                        gpuDisplay += ` TP:${slot.tensor_parallel_size}`;
                                    }
                                } else if (slot.gpu_index !== undefined) {
                                    gpuDisplay = ` [GPU:${slot.gpu_index}]`;
                                }

                                const isLoading = !slot.ready && !slot.active;

                                return (
                                    <Box
                                        key={slot.id || index}
                                        sx={{
                                            display: "flex",
                                            alignItems: "center",
                                            gap: 0.25,
                                        }}
                                    >
                                        <Chip
                                            size="small"
                                            label={`${modelName}${memoryDisplay}${gpuDisplay}`}
                                            sx={{
                                                height: 14,
                                                fontSize: "0.55rem",
                                                backgroundColor: slot.active
                                                    ? "rgba(244, 211, 94, 0.15)"
                                                    : slot.ready
                                                      ? "rgba(0, 200, 255, 0.15)"
                                                      : "rgba(226, 128, 0, 0.15)",
                                                color: slot.active
                                                    ? "#F4D35E"
                                                    : slot.ready
                                                      ? "#00c8ff"
                                                      : "#E28000",
                                                border: `1px solid ${
                                                    slot.active
                                                        ? "rgba(244, 211, 94, 0.3)"
                                                        : slot.ready
                                                          ? "rgba(0, 200, 255, 0.3)"
                                                          : "rgba(226, 128, 0, 0.3)"
                                                }`,
                                                "& .MuiChip-label": {
                                                    px: 0.5,
                                                },
                                            }}
                                        />
                                        {isLoading && (
                                            <CircularProgress
                                                size={8}
                                                thickness={6}
                                                sx={{
                                                    color: "#E28000",
                                                    ml: 0.25,
                                                }}
                                            />
                                        )}
                                    </Box>
                                );
                            })}
                            {runner.slots.length > 3 && (
                                <Typography
                                    variant="caption"
                                    sx={{
                                        fontSize: "0.55rem",
                                        color: "rgba(255, 255, 255, 0.4)",
                                    }}
                                >
                                    +{runner.slots.length - 3}
                                </Typography>
                            )}
                        </Box>
                    </Box>
                )}

                {(!runner.slots || runner.slots.length === 0) && (
                    <Typography
                        variant="caption"
                        sx={{
                            fontSize: "0.6rem",
                            color: "rgba(255, 255, 255, 0.3)",
                            fontStyle: "italic",
                        }}
                    >
                        No active model instances
                    </Typography>
                )}
            </Box>
        );
    };

    return (
        <Paper
            elevation={8}
            sx={{
                position: "fixed",
                left: position.x,
                top: position.y,
                width: isMinimized ? 200 : size.width,
                height: isMinimized ? "auto" : size.height,
                backgroundColor: "rgba(20, 20, 23, 0.95)",
                backdropFilter: "blur(12px)",
                borderRadius: 2,
                border: "1px solid rgba(0, 200, 255, 0.3)",
                boxShadow:
                    "0 8px 32px rgba(0, 0, 0, 0.3), 0 0 0 1px rgba(255, 255, 255, 0.05)",
                zIndex: 9999,
                cursor: isDragging ? "grabbing" : "default",
                transition: isMinimized
                    ? "width 0.3s ease, height 0.3s ease"
                    : "none",
                overflow: "hidden",
            }}
        >
            {/* Resize Handles */}
            {!isMinimized &&
                getResizeHandles().map((handle) => (
                    <Box
                        key={handle.direction}
                        onMouseDown={handle.onMouseDown}
                        sx={{
                            ...handle.style,
                            "&:hover": {
                                backgroundColor: "rgba(0, 200, 255, 0.1)",
                            },
                        }}
                    />
                ))}
            <Box
                sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    p: 1,
                    backgroundColor: "rgba(0, 200, 255, 0.1)",
                    borderBottom: "1px solid rgba(0, 200, 255, 0.2)",
                    cursor: "grab",
                    userSelect: "none",
                }}
                onMouseDown={handleMouseDown}
            >
                <Box sx={{ display: "flex", alignItems: "center" }}>
                    <DnsIcon sx={{ fontSize: 16, mr: 0.5, color: "#00c8ff" }} />
                    <Typography
                        variant="caption"
                        sx={{
                            fontWeight: 600,
                            color: "#fff",
                            fontSize: "0.75rem",
                        }}
                    >
                        Runner State
                    </Typography>
                    {dashboardData?.runners && (
                        <Chip
                            size="small"
                            label={dashboardData.runners.length}
                            sx={{
                                ml: 1,
                                height: 18,
                                fontSize: "0.6rem",
                                backgroundColor: "rgba(0, 200, 255, 0.2)",
                                color: "#00c8ff",
                                border: "1px solid rgba(0, 200, 255, 0.3)",
                            }}
                        />
                    )}
                    {dashboardData?.queue && dashboardData.queue.length > 0 && (
                        <Chip
                            size="small"
                            label={`Q: ${dashboardData.queue.length}`}
                            sx={{
                                ml: 0.5,
                                height: 18,
                                fontSize: "0.6rem",
                                backgroundColor: "rgba(255, 165, 0, 0.2)",
                                color: "#FFA500",
                                border: "1px solid rgba(255, 165, 0, 0.3)",
                            }}
                        />
                    )}
                </Box>

                <Box>
                    <Tooltip title="Open full dashboard">
                        <IconButton
                            size="small"
                            onClick={handleViewFullDashboard}
                            sx={{ color: "rgba(255, 255, 255, 0.7)", p: 0.25 }}
                        >
                            <LaunchIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                    <Tooltip title={isMinimized ? "Maximize" : "Minimize"}>
                        <IconButton
                            size="small"
                            onClick={() => setIsMinimized(!isMinimized)}
                            sx={{ color: "rgba(255, 255, 255, 0.7)", p: 0.25 }}
                        >
                            {isMinimized ? (
                                <MaximizeIcon fontSize="small" />
                            ) : (
                                <MinimizeIcon fontSize="small" />
                            )}
                        </IconButton>
                    </Tooltip>
                    {onClose && (
                        <Tooltip title="Close">
                            <IconButton
                                size="small"
                                onClick={onClose}
                                sx={{
                                    color: "rgba(255, 255, 255, 0.7)",
                                    p: 0.25,
                                    ml: 0.5,
                                }}
                            >
                                <CloseIcon fontSize="small" />
                            </IconButton>
                        </Tooltip>
                    )}
                </Box>
            </Box>

            <Collapse in={!isMinimized}>
                <Box
                    sx={{
                        p: 1.5,
                        height: `calc(${size.height}px - 60px)`, // Subtract header height
                        overflowY: "auto",
                    }}
                >
                    {isLoading ? (
                        <Box sx={{ textAlign: "center", py: 2 }}>
                            <Typography
                                variant="caption"
                                sx={{ color: "rgba(255, 255, 255, 0.5)" }}
                            >
                                Loading runners...
                            </Typography>
                        </Box>
                    ) : !dashboardData?.runners ||
                      dashboardData.runners.length === 0 ? (
                        <Box sx={{ textAlign: "center", py: 2 }}>
                            <Typography
                                variant="caption"
                                sx={{ color: "rgba(255, 255, 255, 0.5)" }}
                            >
                                No active runners
                            </Typography>
                        </Box>
                    ) : (
                        <>
                            <Typography
                                variant="caption"
                                sx={{
                                    color: "rgba(255, 255, 255, 0.7)",
                                    mb: 1,
                                    display: "block",
                                    fontSize: "0.7rem",
                                }}
                            >
                                {dashboardData.runners.length} Active Runner
                                {dashboardData.runners.length !== 1 ? "s" : ""}
                            </Typography>

                            {dashboardData.runners.map(
                                (runner: TypesDashboardRunner) => (
                                    <CompactRunnerDisplay
                                        key={runner.id}
                                        runner={runner}
                                    />
                                ),
                            )}

                            {dashboardData.queue &&
                                dashboardData.queue.length > 0 && (
                                    <>
                                        <Box sx={{ mt: 2, mb: 1 }}>
                                            <Typography
                                                variant="caption"
                                                sx={{
                                                    color: "rgba(255, 255, 255, 0.7)",
                                                    fontSize: "0.7rem",
                                                    display: "block",
                                                }}
                                            >
                                                Queue (
                                                {dashboardData.queue.length}{" "}
                                                pending)
                                            </Typography>
                                            <Box
                                                sx={{
                                                    mt: 0.5,
                                                    display: "flex",
                                                    flexWrap: "wrap",
                                                    gap: 0.25,
                                                }}
                                            >
                                                {dashboardData.queue
                                                    .slice(0, 3)
                                                    .map((workload, index) => (
                                                        <Chip
                                                            key={
                                                                workload.id ||
                                                                index
                                                            }
                                                            size="small"
                                                            label={
                                                                workload.model_name ||
                                                                "Unknown"
                                                            }
                                                            sx={{
                                                                height: 14,
                                                                fontSize:
                                                                    "0.55rem",
                                                                backgroundColor:
                                                                    "rgba(255, 165, 0, 0.15)",
                                                                color: "#FFA500",
                                                                border: "1px solid rgba(255, 165, 0, 0.3)",
                                                                "& .MuiChip-label":
                                                                    {
                                                                        px: 0.5,
                                                                    },
                                                            }}
                                                        />
                                                    ))}
                                                {dashboardData.queue.length >
                                                    3 && (
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            fontSize: "0.55rem",
                                                            color: "rgba(255, 255, 255, 0.4)",
                                                        }}
                                                    >
                                                        +
                                                        {dashboardData.queue
                                                            .length - 3}
                                                    </Typography>
                                                )}
                                            </Box>
                                        </Box>
                                    </>
                                )}
                        </>
                    )}
                </Box>
            </Collapse>
        </Paper>
    );
};

export default FloatingRunnerState;
