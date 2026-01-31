import Box from "@mui/material/Box";
import LinearProgress from "@mui/material/LinearProgress";
import Typography from "@mui/material/Typography";
import { FC } from "react";
import { prettyBytes } from "../../utils/format";
import Cell from "../widgets/Cell";
import Row from "../widgets/Row";
import ModelInstanceSummary from "./ModelInstanceSummary";
import Paper from "@mui/material/Paper";
import Divider from "@mui/material/Divider";
import Chip from "@mui/material/Chip";
import Grid from "@mui/material/Grid";
import Tooltip from "@mui/material/Tooltip";
import CircularProgress from "@mui/material/CircularProgress";
import Accordion from "@mui/material/Accordion";
import AccordionSummary from "@mui/material/AccordionSummary";
import AccordionDetails from "@mui/material/AccordionDetails";
import ExpandMoreIcon from "@mui/icons-material/ExpandMore";
import { LineChart } from "@mui/x-charts";

import {
    TypesDashboardRunner,
    TypesGPUStatus,
    TypesGPUMemoryStats,
    TypesGPUMemoryDataPoint,
    TypesSchedulingEvent,
} from "../../api/api";

import ModelInstanceLogs from "../admin/ModelInstanceLogs";

// GPU Memory Chart Component
const GPUMemoryChart: FC<{
    gpuMemoryStats: TypesGPUMemoryStats;
    gpuIndex?: number;
    height?: number;
}> = ({ gpuMemoryStats, gpuIndex, height = 200 }) => {
    if (!gpuMemoryStats?.memory_time_series?.length) {
        return (
            <Box
                sx={{
                    height,
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "center",
                    color: "rgba(255, 255, 255, 0.5)",
                    backgroundColor: "rgba(0, 0, 0, 0.2)",
                    borderRadius: 2,
                    border: "1px solid rgba(255, 255, 255, 0.08)",
                }}
            >
                <Typography variant="body2">
                    No memory data available
                </Typography>
            </Box>
        );
    }

    // Filter data for specific GPU if provided and sort by timestamp
    const filteredData =
        gpuIndex !== undefined
            ? gpuMemoryStats.memory_time_series
                  .filter((point) => point.gpu_index === gpuIndex)
                  .sort(
                      (a, b) =>
                          new Date(a.timestamp || "").getTime() -
                          new Date(b.timestamp || "").getTime(),
                  )
            : gpuMemoryStats.memory_time_series.sort(
                  (a, b) =>
                      new Date(a.timestamp || "").getTime() -
                      new Date(b.timestamp || "").getTime(),
              );

    // Transform data for MUI charts (already sorted)
    const chartData = filteredData.map((point) => ({
        timestamp: new Date(point.timestamp || ""),
        allocated: point.allocated_mb || 0,
        used: point.actual_used_mb || 0,
        total: point.actual_total_mb || 0,
    }));

    const xAxisData = chartData.map((point) => point.timestamp);
    const allocatedData = chartData.map((point) => point.allocated);
    const usedData = chartData.map((point) => point.used);

    const formatBytes = (bytes: number) => {
        if (bytes === 0) return "0 MB";
        const k = 1024;
        const sizes = ["MB", "GB"];
        const i = Math.floor(Math.log(bytes) / Math.log(k));
        return parseFloat((bytes / Math.pow(k, i)).toFixed(1)) + " " + sizes[i];
    };

    return (
        <Box
            sx={{
                height,
                backgroundColor: "rgba(0, 0, 0, 0.2)",
                borderRadius: 2,
                border: "1px solid rgba(255, 255, 255, 0.08)",
                p: 1.5,
                boxShadow: "inset 0 1px 3px rgba(0, 0, 0, 0.3)",
            }}
        >
            <LineChart
                xAxis={[
                    {
                        data: xAxisData,
                        scaleType: "time",
                        valueFormatter: (value: Date) =>
                            value.toLocaleTimeString(),
                        labelStyle: {
                            fontSize: "10px",
                            fill: "rgba(255, 255, 255, 0.6)",
                        },
                        tickMinStep: 30000, // 30 seconds
                    },
                ]}
                yAxis={[
                    {
                        valueFormatter: formatBytes,
                        labelStyle: {
                            fontSize: "10px",
                            fill: "rgba(255, 255, 255, 0.6)",
                        },
                        min: 0,
                        max: chartData.length > 0 ? chartData[0].total : 100000,
                    },
                ]}
                series={[
                    {
                        data: allocatedData,
                        label: "Allocated",
                        color: "#7986cb",
                        showMark: false,
                        curve: "linear",
                    },
                    {
                        data: usedData,
                        label: "Used",
                        color: "#00c8ff",
                        showMark: false,
                        curve: "linear",
                    },
                ]}
                height={height - 30}
                margin={{ left: 60, right: 20, top: 40, bottom: 30 }}
                grid={{ horizontal: true, vertical: false }}
                sx={{
                    "& .MuiChartsGrid-line": {
                        stroke: "rgba(255, 255, 255, 0.1)",
                        strokeDasharray: "3 3",
                    },
                    "& .MuiChartsAxis-tick": {
                        stroke: "rgba(255, 255, 255, 0.6)",
                    },
                    "& .MuiChartsAxis-line": {
                        stroke: "rgba(255, 255, 255, 0.6)",
                    },
                }}
                slotProps={{
                    legend: {
                        direction: "row",
                        position: { vertical: "top", horizontal: "left" },
                        padding: 10,
                        labelStyle: {
                            fontSize: "10px",
                            fill: "rgba(255, 255, 255, 0.8)",
                        },
                    },
                }}
            />
        </Box>
    );
};

const GPUCard: FC<{
    gpu: TypesGPUStatus;
    allocatedMemory: number;
    allocatedModels: Array<{
        model: string;
        memory: number;
        isMultiGPU: boolean;
    }>;
    runner: TypesDashboardRunner;
}> = ({ gpu, allocatedMemory, allocatedModels, runner }) => {
    const total_memory = gpu.total_memory || 1;
    const used_memory = gpu.used_memory || 0;

    const usedPercent = Math.round((used_memory / total_memory) * 100);
    const allocatedPercent = Math.round((allocatedMemory / total_memory) * 100);

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
                        sx={{ display: "block", fontSize: "0.65rem" }}
                    >
                        • {modelInfo.model} ({prettyBytes(modelInfo.memory)})
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

    // Simplify GPU model name for display (vendor-neutral)
    const getSimpleModelName = (fullName: string) => {
        if (!fullName || fullName === "unknown") return "Unknown GPU";

        // Extract key parts: H100, RTX 4090, RX 7900 XTX, etc.
        // Remove vendor prefix and connection type suffixes
        const name = fullName
            .replace(/^(NVIDIA|AMD|Intel)\s+/, "")  // Remove vendor prefix
            .replace(/\s+PCIe.*$/, "")               // Remove PCIe details
            .replace(/\s+SXM.*$/, "")                // Remove SXM details
            .replace(/^Radeon\s+/, "");              // Remove "Radeon" prefix for AMD
        return name;
    };

    return (
        <Box
            sx={{
                mb: 1.5,
                p: 2,
                backgroundColor: "rgba(0, 0, 0, 0.2)",
                borderRadius: 1,
                border: "1px solid rgba(255, 255, 255, 0.05)",
                backdropFilter: "blur(5px)",
            }}
        >
            <Box
                sx={{
                    display: "flex",
                    alignItems: "center",
                    justifyContent: "space-between",
                    mb: 1,
                }}
            >
                <Box>
                    <Typography
                        variant="subtitle2"
                        sx={{
                            fontWeight: 600,
                            color: "#00c8ff",
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
                                fontSize: "0.65rem",
                                color: "rgba(255, 255, 255, 0.6)",
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
                        height: 18,
                        fontSize: "0.65rem",
                        backgroundColor:
                            usedPercent > 80
                                ? "rgba(255, 0, 0, 0.2)"
                                : "rgba(0, 200, 255, 0.2)",
                        color: usedPercent > 80 ? "#ff6b6b" : "#00c8ff",
                        border: `1px solid ${usedPercent > 80 ? "rgba(255, 0, 0, 0.3)" : "rgba(0, 200, 255, 0.3)"}`,
                    }}
                />
            </Box>

            {/* Dual progress bars - allocated (background) and used (foreground) */}
            <Box sx={{ position: "relative", height: 12, mb: 1 }}>
                <Box
                    sx={{
                        position: "absolute",
                        width: "100%",
                        height: "100%",
                        backgroundColor: "rgba(255, 255, 255, 0.03)",
                        borderRadius: "3px",
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
                            borderRadius: "3px",
                            backgroundColor: "transparent",
                            cursor: allocatedTooltip ? "help" : "default",
                            "& .MuiLinearProgress-bar": {
                                background:
                                    "linear-gradient(90deg, rgba(121,134,203,0.9) 0%, rgba(121,134,203,0.7) 100%)",
                                borderRadius: "3px",
                                transition:
                                    "transform 0.5s cubic-bezier(0.4, 0, 0.2, 1)",
                                boxShadow: "0 0 6px rgba(121,134,203,0.5)",
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
                        height: 8,
                        top: 2,
                        borderRadius: "3px",
                        backgroundColor: "transparent",
                        "& .MuiLinearProgress-bar": {
                            background:
                                usedPercent > 80
                                    ? "linear-gradient(90deg, rgba(255,107,107,1) 0%, rgba(255,107,107,0.8) 100%)"
                                    : "linear-gradient(90deg, rgba(0,200,255,1) 0%, rgba(0,200,255,0.8) 100%)",
                            borderRadius: "3px",
                            boxShadow:
                                usedPercent > 80
                                    ? "0 0 8px rgba(255,107,107,0.7)"
                                    : "0 0 8px rgba(0,200,255,0.7)",
                            transition:
                                "transform 0.5s cubic-bezier(0.4, 0, 0.2, 1)",
                        },
                    }}
                />
            </Box>

            {/* Memory details */}
            <Box sx={{ display: "flex", flexDirection: "column", gap: 0.25 }}>
                <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                    <Typography
                        variant="caption"
                        sx={{
                            fontSize: "0.7rem",
                            color: "#00c8ff",
                            fontWeight: 600,
                        }}
                    >
                        Used: {prettyBytes(used_memory)} ({usedPercent}%)
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
                                fontSize: "0.7rem",
                                color: "#7986cb",
                                fontWeight: 600,
                            }}
                        >
                            Allocated: {prettyBytes(allocatedMemory)} (
                            {allocatedPercent}%)
                        </Typography>
                    </Box>
                )}
                <Box sx={{ display: "flex", justifyContent: "space-between" }}>
                    <Typography
                        variant="caption"
                        sx={{
                            fontSize: "0.7rem",
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

export const RunnerSummary: FC<{
    runner: TypesDashboardRunner;
    onViewSession: {
        (id: string): void;
    };
}> = ({ runner, onViewSession }) => {
    // Get memory values with proper fallbacks to ensure we have valid numbers
    const total_memory = runner.total_memory || 1; // Avoid division by zero
    const used_memory =
        typeof runner.used_memory === "number" ? runner.used_memory : 0;
    const allocated_memory =
        !runner.slots || runner.slots.length === 0 || !runner.allocated_memory
            ? 0
            : runner.allocated_memory;

    // Calculate percentages with safeguards against NaN or Infinity
    const actualPercent = isFinite(
        Math.round((used_memory / total_memory) * 100),
    )
        ? Math.round((used_memory / total_memory) * 100)
        : 0;

    const allocatedPercent = isFinite(
        Math.round((allocated_memory / total_memory) * 100),
    )
        ? Math.round((allocated_memory / total_memory) * 100)
        : 0;

    return (
        <Paper
            elevation={3}
            sx={{
                width: "100%",
                minWidth: 600,
                minHeight: 180,
                mb: 3,
                backgroundColor: "rgba(30, 30, 32, 0.95)",
                borderLeft: "4px solid",
                borderColor: "#00c8ff",
                borderRadius: "3px",
                overflow: "hidden",
                position: "relative",
                backdropFilter: "blur(10px)",
                boxShadow:
                    "0 6px 14px -2px rgba(0, 0, 0, 0.2), 0 0 0 1px rgba(255, 255, 255, 0.05)",
                "&::before": {
                    content: '""',
                    position: "absolute",
                    top: 0,
                    left: 0,
                    right: 0,
                    height: "100%",
                    backgroundImage:
                        "linear-gradient(180deg, rgba(0, 200, 255, 0.08) 0%, rgba(0, 0, 0, 0) 30%)",
                    pointerEvents: "none",
                },
            }}
        >
            {/* Side glow effect */}
            <Box
                sx={{
                    position: "absolute",
                    left: 0,
                    top: 0,
                    bottom: 0,
                    width: "4px",
                    background:
                        "linear-gradient(180deg, #00c8ff 0%, rgba(0, 200, 255, 0.3) 100%)",
                    boxShadow: "0 0 15px 1px rgba(0, 200, 255, 0.5)",
                    opacity: 0.8,
                    zIndex: 2,
                }}
            />

            {/* Light reflection effect */}
            <Box
                sx={{
                    position: "absolute",
                    right: 0,
                    top: 0,
                    width: "40%",
                    height: "100%",
                    background:
                        "linear-gradient(90deg, rgba(255,255,255,0) 0%, rgba(255,255,255,0.03) 100%)",
                    pointerEvents: "none",
                    opacity: 0.5,
                }}
            />

            <Box sx={{ p: 2.5 }}>
                <Grid container alignItems="center" spacing={2}>
                    <Grid item xs>
                        <Box sx={{ display: "flex", alignItems: "center" }}>
                            <Typography
                                variant="h6"
                                fontWeight="600"
                                sx={{
                                    mr: 2,
                                    color: "#fff",
                                    letterSpacing: "0.5px",
                                }}
                            >
                                {runner.id}
                            </Typography>

                            <Box
                                sx={{
                                    display: "inline-block",
                                    px: 1.5,
                                    py: 0.5,
                                    backgroundColor:
                                        "rgba(255, 255, 255, 0.07)",
                                    border: "1px solid rgba(255, 255, 255, 0.1)",
                                    borderRadius: "3px",
                                    boxShadow:
                                        "inset 0 1px 1px rgba(0, 0, 0, 0.1)",
                                    backdropFilter: "blur(5px)",
                                }}
                            >
                                <Typography
                                    variant="caption"
                                    sx={{
                                        fontFamily: "monospace",
                                        color: "rgba(255, 255, 255, 0.7)",
                                        fontWeight: 500,
                                        letterSpacing: "0.5px",
                                    }}
                                >
                                    {runner.version || "unknown"}
                                </Typography>
                            </Box>
                        </Box>
                    </Grid>

                    <Grid item>
                        <Box
                            sx={{
                                display: "flex",
                                flexWrap: "wrap",
                                justifyContent: "flex-end",
                            }}
                        >
                            {Object.keys(runner.labels || {}).map((k) => (
                                <Chip
                                    key={k}
                                    size="small"
                                    label={`${k}=${runner.labels?.[k]}`}
                                    sx={{
                                        mr: 0.5,
                                        mb: 0.5,
                                        borderRadius: "3px",
                                        backgroundColor:
                                            "rgba(0, 200, 255, 0.08)",
                                        border: "1px solid rgba(0, 200, 255, 0.2)",
                                        color: "rgba(255, 255, 255, 0.85)",
                                        "& .MuiChip-label": {
                                            fontSize: "0.7rem",
                                            px: 1.2,
                                        },
                                    }}
                                />
                            ))}
                        </Box>
                    </Grid>
                </Grid>

                <Divider
                    sx={{
                        my: 2,
                        borderColor: "rgba(255, 255, 255, 0.06)",
                        boxShadow: "0 1px 2px rgba(0, 0, 0, 0.1)",
                    }}
                />

                {/* Process Cleanup Stats Section */}
                {runner.process_stats && (
                    <Accordion
                        sx={{
                            mb: 3,
                            backgroundColor: "rgba(255, 255, 255, 0.02)",
                            border: "1px solid rgba(255, 255, 255, 0.06)",
                            borderRadius: 2,
                            "&:before": {
                                display: "none",
                            },
                            "& .MuiAccordionSummary-root": {
                                minHeight: 48,
                                "&.Mui-expanded": {
                                    minHeight: 48,
                                },
                            },
                            "& .MuiAccordionDetails-root": {
                                paddingTop: 0,
                            },
                        }}
                    >
                        <AccordionSummary
                            expandIcon={
                                <ExpandMoreIcon
                                    sx={{ color: "rgba(255, 255, 255, 0.7)" }}
                                />
                            }
                            sx={{
                                "& .MuiAccordionSummary-content": {
                                    margin: "12px 0",
                                    "&.Mui-expanded": {
                                        margin: "12px 0",
                                    },
                                },
                            }}
                        >
                            <Typography
                                variant="subtitle2"
                                sx={{
                                    color: "rgba(255, 255, 255, 0.8)",
                                    fontWeight: 600,
                                    display: "flex",
                                    alignItems: "center",
                                    gap: 1,
                                }}
                            >
                                Process Management
                                <Chip
                                    size="small"
                                    label={`${runner.process_stats.total_tracked_processes || 0} tracked`}
                                    sx={{
                                        height: 20,
                                        fontSize: "0.65rem",
                                        backgroundColor:
                                            "rgba(0, 200, 255, 0.2)",
                                        color: "#00c8ff",
                                        border: "1px solid rgba(0, 200, 255, 0.3)",
                                    }}
                                />
                            </Typography>
                        </AccordionSummary>
                        <AccordionDetails>
                            <Grid container spacing={2}>
                                <Grid item xs={12} md={6}>
                                    <Box
                                        sx={{
                                            p: 2,
                                            backgroundColor:
                                                "rgba(0, 0, 0, 0.2)",
                                            borderRadius: "6px",
                                            border: "1px solid rgba(255, 255, 255, 0.05)",
                                        }}
                                    >
                                        <Typography
                                            variant="body2"
                                            sx={{
                                                color: "rgba(255, 255, 255, 0.7)",
                                                mb: 1,
                                            }}
                                        >
                                            Cleanup Statistics
                                        </Typography>
                                        <Box
                                            sx={{
                                                display: "flex",
                                                flexWrap: "wrap",
                                                gap: 1,
                                                mb: 2,
                                            }}
                                        >
                                            <Chip
                                                size="small"
                                                label={`${runner.process_stats.cleanup_stats?.total_cleaned || 0} processes cleaned`}
                                                sx={{
                                                    height: 24,
                                                    fontSize: "0.7rem",
                                                    backgroundColor:
                                                        (runner.process_stats
                                                            .cleanup_stats
                                                            ?.total_cleaned ||
                                                            0) > 0
                                                            ? "rgba(255, 152, 0, 0.15)"
                                                            : "rgba(76, 175, 80, 0.15)",
                                                    color:
                                                        (runner.process_stats
                                                            .cleanup_stats
                                                            ?.total_cleaned ||
                                                            0) > 0
                                                            ? "#FF9800"
                                                            : "#4CAF50",
                                                    border: `1px solid ${
                                                        (runner.process_stats
                                                            .cleanup_stats
                                                            ?.total_cleaned ||
                                                            0) > 0
                                                            ? "rgba(255, 152, 0, 0.3)"
                                                            : "rgba(76, 175, 80, 0.3)"
                                                    }`,
                                                }}
                                            />
                                            <Chip
                                                size="small"
                                                label={`${runner.process_stats.cleanup_stats?.synchronous_runs || 0} sync runs`}
                                                sx={{
                                                    height: 24,
                                                    fontSize: "0.7rem",
                                                    backgroundColor:
                                                        "rgba(156, 39, 176, 0.15)",
                                                    color: "#9C27B0",
                                                    border: "1px solid rgba(156, 39, 176, 0.3)",
                                                }}
                                            />
                                            <Chip
                                                size="small"
                                                label={`${runner.process_stats.cleanup_stats?.asynchronous_runs || 0} async runs`}
                                                sx={{
                                                    height: 24,
                                                    fontSize: "0.7rem",
                                                    backgroundColor:
                                                        "rgba(63, 81, 181, 0.15)",
                                                    color: "#3F51B5",
                                                    border: "1px solid rgba(63, 81, 181, 0.3)",
                                                }}
                                            />
                                        </Box>
                                        {runner.process_stats.cleanup_stats
                                            ?.last_cleanup_time && (
                                            <Typography
                                                variant="caption"
                                                sx={{
                                                    color: "rgba(255, 255, 255, 0.5)",
                                                    fontSize: "0.7rem",
                                                    display: "block",
                                                }}
                                            >
                                                Last cleanup:{" "}
                                                {new Date(
                                                    runner.process_stats.cleanup_stats.last_cleanup_time,
                                                ).toLocaleString()}
                                            </Typography>
                                        )}
                                    </Box>
                                </Grid>

                                {runner.process_stats.cleanup_stats
                                    ?.recent_cleanups &&
                                    runner.process_stats.cleanup_stats
                                        .recent_cleanups.length > 0 && (
                                        <Grid item xs={12} md={6}>
                                            <Box
                                                sx={{
                                                    p: 2,
                                                    backgroundColor:
                                                        "rgba(0, 0, 0, 0.2)",
                                                    borderRadius: "6px",
                                                    border: "1px solid rgba(255, 255, 255, 0.05)",
                                                }}
                                            >
                                                <Typography
                                                    variant="body2"
                                                    sx={{
                                                        color: "rgba(255, 255, 255, 0.7)",
                                                        mb: 1,
                                                    }}
                                                >
                                                    Recent Cleanups (
                                                    {
                                                        runner.process_stats
                                                            .cleanup_stats
                                                            .recent_cleanups
                                                            .length
                                                    }
                                                    )
                                                </Typography>
                                                <Box
                                                    sx={{
                                                        maxHeight: "120px",
                                                        overflowY: "auto",
                                                    }}
                                                >
                                                    {runner.process_stats.cleanup_stats.recent_cleanups
                                                        .slice(-5)
                                                        .reverse()
                                                        .map(
                                                            (
                                                                cleanup: any,
                                                                index: number,
                                                            ) => (
                                                                <Box
                                                                    key={index}
                                                                    sx={{
                                                                        mb: 1,
                                                                        p: 1,
                                                                        backgroundColor:
                                                                            "rgba(255, 255, 255, 0.03)",
                                                                        borderRadius:
                                                                            "4px",
                                                                        border: "1px solid rgba(255, 255, 255, 0.05)",
                                                                    }}
                                                                >
                                                                    <Box
                                                                        sx={{
                                                                            display:
                                                                                "flex",
                                                                            justifyContent:
                                                                                "space-between",
                                                                            alignItems:
                                                                                "center",
                                                                            mb: 0.5,
                                                                        }}
                                                                    >
                                                                        <Typography
                                                                            variant="caption"
                                                                            sx={{
                                                                                color: "#00c8ff",
                                                                                fontWeight: 600,
                                                                                fontSize:
                                                                                    "0.7rem",
                                                                            }}
                                                                        >
                                                                            PID{" "}
                                                                            {
                                                                                cleanup.pid
                                                                            }
                                                                        </Typography>
                                                                        <Chip
                                                                            size="small"
                                                                            label={
                                                                                cleanup.method
                                                                            }
                                                                            sx={{
                                                                                height: 16,
                                                                                fontSize:
                                                                                    "0.6rem",
                                                                                backgroundColor:
                                                                                    cleanup.method ===
                                                                                    "graceful"
                                                                                        ? "rgba(76, 175, 80, 0.2)"
                                                                                        : "rgba(244, 67, 54, 0.2)",
                                                                                color:
                                                                                    cleanup.method ===
                                                                                    "graceful"
                                                                                        ? "#4CAF50"
                                                                                        : "#F44336",
                                                                                border: `1px solid ${
                                                                                    cleanup.method ===
                                                                                    "graceful"
                                                                                        ? "rgba(76, 175, 80, 0.3)"
                                                                                        : "rgba(244, 67, 54, 0.3)"
                                                                                }`,
                                                                            }}
                                                                        />
                                                                    </Box>
                                                                    <Typography
                                                                        variant="caption"
                                                                        sx={{
                                                                            color: "rgba(255, 255, 255, 0.4)",
                                                                            fontSize:
                                                                                "0.65rem",
                                                                            display:
                                                                                "block",
                                                                            mb: 0.5,
                                                                            wordBreak:
                                                                                "break-all",
                                                                        }}
                                                                    >
                                                                        {cleanup
                                                                            .command
                                                                            .length >
                                                                        60
                                                                            ? `${cleanup.command.substring(0, 60)}...`
                                                                            : cleanup.command}
                                                                    </Typography>
                                                                    <Typography
                                                                        variant="caption"
                                                                        sx={{
                                                                            color: "rgba(255, 255, 255, 0.3)",
                                                                            fontSize:
                                                                                "0.6rem",
                                                                        }}
                                                                    >
                                                                        {new Date(
                                                                            cleanup.cleaned_at,
                                                                        ).toLocaleString()}
                                                                    </Typography>
                                                                </Box>
                                                            ),
                                                        )}
                                                </Box>
                                            </Box>
                                        </Grid>
                                    )}
                            </Grid>
                        </AccordionDetails>
                    </Accordion>
                )}

                {/* GPU Memory Section */}
                {runner.gpus &&
                    runner.gpus.length > 0 &&
                    runner.gpu_memory_stats && (
                        <Accordion
                            sx={{
                                mb: 3,
                                backgroundColor: "rgba(255, 255, 255, 0.02)",
                                border: "1px solid rgba(255, 255, 255, 0.06)",
                                borderRadius: 2,
                                "&:before": {
                                    display: "none",
                                },
                            }}
                        >
                            <AccordionSummary
                                expandIcon={
                                    <ExpandMoreIcon
                                        sx={{
                                            color: "rgba(255, 255, 255, 0.7)",
                                        }}
                                    />
                                }
                            >
                                <Typography
                                    variant="subtitle2"
                                    sx={{
                                        color: "rgba(255, 255, 255, 0.8)",
                                        fontWeight: 600,
                                        display: "flex",
                                        alignItems: "center",
                                        gap: 1,
                                    }}
                                >
                                    GPU Memory Usage
                                    <Chip
                                        size="small"
                                        label={`${runner.gpus.length} GPUs`}
                                        sx={{
                                            height: 20,
                                            fontSize: "0.65rem",
                                            backgroundColor:
                                                "rgba(0, 200, 255, 0.2)",
                                            color: "#00c8ff",
                                            border: "1px solid rgba(0, 200, 255, 0.3)",
                                        }}
                                    />
                                </Typography>
                            </AccordionSummary>
                            <AccordionDetails>
                                <Box sx={{ mb: 3 }}>
                                    <Box
                                        sx={{
                                            display: "flex",
                                            alignItems: "center",
                                            justifyContent: "space-between",
                                            mb: 1,
                                        }}
                                    >
                                        <Typography
                                            variant="body2"
                                            sx={{
                                                color: "rgba(255, 255, 255, 0.7)",
                                                fontSize: "0.8rem",
                                                fontWeight: 600,
                                            }}
                                        >
                                            Memory Usage Over Time (Last 10
                                            minutes)
                                        </Typography>
                                        <Box sx={{ display: "flex", gap: 2 }}>
                                            <Box
                                                sx={{
                                                    display: "flex",
                                                    alignItems: "center",
                                                    gap: 0.5,
                                                }}
                                            >
                                                <Box
                                                    sx={{
                                                        width: 12,
                                                        height: 2,
                                                        backgroundColor:
                                                            "#00c8ff",
                                                        borderRadius: 1,
                                                    }}
                                                />
                                                <Typography
                                                    variant="caption"
                                                    sx={{
                                                        color: "#00c8ff",
                                                        fontSize: "0.7rem",
                                                    }}
                                                >
                                                    Used
                                                </Typography>
                                            </Box>
                                            <Box
                                                sx={{
                                                    display: "flex",
                                                    alignItems: "center",
                                                    gap: 0.5,
                                                }}
                                            >
                                                <Box
                                                    sx={{
                                                        width: 12,
                                                        height: 2,
                                                        backgroundColor:
                                                            "#7986cb",
                                                        borderRadius: 1,
                                                        backgroundImage:
                                                            "repeating-linear-gradient(90deg, transparent, transparent 3px, rgba(255,255,255,0.3) 3px, rgba(255,255,255,0.3) 6px)",
                                                    }}
                                                />
                                                <Typography
                                                    variant="caption"
                                                    sx={{
                                                        color: "#7986cb",
                                                        fontSize: "0.7rem",
                                                    }}
                                                >
                                                    Allocated
                                                </Typography>
                                            </Box>
                                        </Box>
                                    </Box>

                                    <Grid container spacing={2}>
                                        {runner.gpus
                                            ? (() => {
                                                  const sortedGPUs = [
                                                      ...runner.gpus,
                                                  ].sort((a, b) => {
                                                      const indexA =
                                                          a.index ?? 999;
                                                      const indexB =
                                                          b.index ?? 999;
                                                      return indexA - indexB;
                                                  });
                                                  return sortedGPUs.map(
                                                      (gpu) => (
                                                          <Grid
                                                              item
                                                              xs={12}
                                                              md={
                                                                  runner.gpus
                                                                      ?.length ===
                                                                  1
                                                                      ? 12
                                                                      : 6
                                                              }
                                                              key={`gpu-chart-${gpu.index}`}
                                                          >
                                                              <Typography
                                                                  variant="caption"
                                                                  sx={{
                                                                      color: "#00c8ff",
                                                                      fontWeight: 600,
                                                                      display:
                                                                          "block",
                                                                      mb: 1,
                                                                  }}
                                                              >
                                                                  GPU{" "}
                                                                  {gpu.index}
                                                              </Typography>
                                                              <GPUMemoryChart
                                                                  gpuMemoryStats={
                                                                      runner.gpu_memory_stats!
                                                                  }
                                                                  gpuIndex={
                                                                      gpu.index
                                                                  }
                                                                  height={200}
                                                              />
                                                          </Grid>
                                                      ),
                                                  );
                                              })()
                                            : null}
                                    </Grid>

                                    {/* GPU Memory Statistics */}
                                    <Box
                                        sx={{
                                            mt: 3,
                                            p: 2,
                                            backgroundColor:
                                                "rgba(0, 0, 0, 0.2)",
                                            borderRadius: 2,
                                            border: "1px solid rgba(255, 255, 255, 0.05)",
                                        }}
                                    >
                                        <Typography
                                            variant="body2"
                                            sx={{
                                                color: "rgba(255, 255, 255, 0.7)",
                                                mb: 1,
                                                fontWeight: 600,
                                            }}
                                        >
                                            Memory Management Statistics
                                        </Typography>
                                        <Grid container spacing={2}>
                                            <Grid item xs={12} sm={6} md={3}>
                                                <Box
                                                    sx={{ textAlign: "center" }}
                                                >
                                                    <Tooltip
                                                        title={
                                                            (runner.gpu_memory_stats.recent_events?.filter(
                                                                (event) =>
                                                                    event.success,
                                                            )?.length ?? 0) >
                                                            0 ? (
                                                                <Box>
                                                                    <Typography
                                                                        variant="caption"
                                                                        sx={{
                                                                            fontWeight: 600,
                                                                            display:
                                                                                "block",
                                                                            mb: 1,
                                                                        }}
                                                                    >
                                                                        Recent
                                                                        Successes:
                                                                    </Typography>
                                                                    {runner.gpu_memory_stats.recent_events
                                                                        ?.filter(
                                                                            (
                                                                                event,
                                                                            ) =>
                                                                                event.success,
                                                                        )
                                                                        .slice(
                                                                            -3,
                                                                        ) // Show last 3 successes
                                                                        .map(
                                                                            (
                                                                                event,
                                                                                idx,
                                                                            ) => (
                                                                                <Box
                                                                                    key={
                                                                                        idx
                                                                                    }
                                                                                    sx={{
                                                                                        mb: 1,
                                                                                        p: 1,
                                                                                        backgroundColor:
                                                                                            "rgba(0, 255, 0, 0.1)",
                                                                                        borderRadius: 1,
                                                                                    }}
                                                                                >
                                                                                    <Typography
                                                                                        variant="caption"
                                                                                        sx={{
                                                                                            display:
                                                                                                "block",
                                                                                            fontSize:
                                                                                                "0.65rem",
                                                                                            color: "#4CAF50",
                                                                                        }}
                                                                                    >
                                                                                        {new Date(
                                                                                            event.timestamp ||
                                                                                                "",
                                                                                        ).toLocaleString()}
                                                                                    </Typography>
                                                                                    <Typography
                                                                                        variant="caption"
                                                                                        sx={{
                                                                                            display:
                                                                                                "block",
                                                                                            fontSize:
                                                                                                "0.65rem",
                                                                                        }}
                                                                                    >
                                                                                        {event.context ===
                                                                                        "startup"
                                                                                            ? "🚀 Pre-Startup"
                                                                                            : "🛑 Post-Deletion"}{" "}
                                                                                        •{" "}
                                                                                        Slot:{" "}
                                                                                        {event.slot_id?.substring(
                                                                                            0,
                                                                                            8,
                                                                                        )}
                                                                                        ...
                                                                                        (
                                                                                        {
                                                                                            event.runtime
                                                                                        }

                                                                                        )
                                                                                    </Typography>
                                                                                    <Typography
                                                                                        variant="caption"
                                                                                        sx={{
                                                                                            display:
                                                                                                "block",
                                                                                            fontSize:
                                                                                                "0.6rem",
                                                                                            color: "rgba(255, 255, 255, 0.5)",
                                                                                        }}
                                                                                    >
                                                                                        Polls:{" "}
                                                                                        {
                                                                                            event.polls_taken
                                                                                        }

                                                                                        ,
                                                                                        Wait:{" "}
                                                                                        {
                                                                                            event.total_wait_seconds
                                                                                        }

                                                                                        s
                                                                                    </Typography>
                                                                                    {event.memory_readings &&
                                                                                        event
                                                                                            .memory_readings
                                                                                            .length >
                                                                                            0 && (
                                                                                            <Box
                                                                                                sx={{
                                                                                                    mt: 0.5,
                                                                                                    p: 0.5,
                                                                                                    backgroundColor:
                                                                                                        "rgba(0, 0, 0, 0.2)",
                                                                                                    borderRadius: 0.5,
                                                                                                }}
                                                                                            >
                                                                                                <Typography
                                                                                                    variant="caption"
                                                                                                    sx={{
                                                                                                        display:
                                                                                                            "block",
                                                                                                        fontSize:
                                                                                                            "0.6rem",
                                                                                                        fontWeight: 600,
                                                                                                        mb: 0.5,
                                                                                                    }}
                                                                                                >
                                                                                                    Memory
                                                                                                    Readings:
                                                                                                </Typography>
                                                                                                {event.memory_readings.map(
                                                                                                    (
                                                                                                        reading,
                                                                                                        ridx,
                                                                                                    ) => (
                                                                                                        <Typography
                                                                                                            key={
                                                                                                                ridx
                                                                                                            }
                                                                                                            variant="caption"
                                                                                                            sx={{
                                                                                                                display:
                                                                                                                    "block",
                                                                                                                fontSize:
                                                                                                                    "0.55rem",
                                                                                                                color: reading.is_stable
                                                                                                                    ? "#4CAF50"
                                                                                                                    : "#ff6b6b",
                                                                                                                fontFamily:
                                                                                                                    "monospace",
                                                                                                            }}
                                                                                                        >
                                                                                                            Poll{" "}
                                                                                                            {
                                                                                                                reading.poll_number
                                                                                                            }

                                                                                                            :{" "}
                                                                                                            {(
                                                                                                                reading.memory_mb ||
                                                                                                                0
                                                                                                            ).toFixed(
                                                                                                                0,
                                                                                                            )}
                                                                                                            MB
                                                                                                            {reading.delta_mb !==
                                                                                                                undefined && (
                                                                                                                <>
                                                                                                                    {" "}
                                                                                                                    (
                                                                                                                    {reading.delta_mb >
                                                                                                                    0
                                                                                                                        ? "+"
                                                                                                                        : ""}
                                                                                                                    {
                                                                                                                        reading.delta_mb
                                                                                                                    }
                                                                                                                    MB
                                                                                                                    Δ)
                                                                                                                </>
                                                                                                            )}{" "}
                                                                                                            Stable:
                                                                                                            {
                                                                                                                reading.stable_count
                                                                                                            }

                                                                                                            /
                                                                                                            {
                                                                                                                event.required_stable_polls
                                                                                                            }
                                                                                                            {reading.is_stable
                                                                                                                ? " ✓"
                                                                                                                : " ✗"}
                                                                                                        </Typography>
                                                                                                    ),
                                                                                                )}
                                                                                                <Typography
                                                                                                    variant="caption"
                                                                                                    sx={{
                                                                                                        display:
                                                                                                            "block",
                                                                                                        fontSize:
                                                                                                            "0.55rem",
                                                                                                        color: "#4CAF50",
                                                                                                        fontWeight: 600,
                                                                                                        mt: 0.5,
                                                                                                    }}
                                                                                                >
                                                                                                    ✓
                                                                                                    Stabilized
                                                                                                    in{" "}
                                                                                                    {
                                                                                                        event.total_wait_seconds
                                                                                                    }

                                                                                                    s
                                                                                                </Typography>
                                                                                            </Box>
                                                                                        )}
                                                                                </Box>
                                                                            ),
                                                                        )}
                                                                </Box>
                                                            ) : (
                                                                "No recent successes"
                                                            )
                                                        }
                                                        placement="top"
                                                        arrow
                                                    >
                                                        <Typography
                                                            variant="h6"
                                                            sx={{
                                                                color: "#4CAF50",
                                                                fontWeight: 600,
                                                                cursor: "help",
                                                            }}
                                                        >
                                                            {runner
                                                                .gpu_memory_stats
                                                                .successful_stabilizations ||
                                                                0}
                                                        </Typography>
                                                    </Tooltip>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            color: "rgba(255, 255, 255, 0.6)",
                                                            fontSize: "0.7rem",
                                                        }}
                                                    >
                                                        Successful
                                                        Stabilizations
                                                    </Typography>
                                                </Box>
                                            </Grid>
                                            <Grid item xs={12} sm={6} md={3}>
                                                <Box
                                                    sx={{ textAlign: "center" }}
                                                >
                                                    <Tooltip
                                                        title={
                                                            (runner.gpu_memory_stats.recent_events?.filter(
                                                                (event) =>
                                                                    !event.success,
                                                            )?.length ?? 0) >
                                                            0 ? (
                                                                <Box>
                                                                    <Typography
                                                                        variant="caption"
                                                                        sx={{
                                                                            fontWeight: 600,
                                                                            display:
                                                                                "block",
                                                                            mb: 1,
                                                                        }}
                                                                    >
                                                                        Recent
                                                                        Failures:
                                                                    </Typography>
                                                                    {runner.gpu_memory_stats.recent_events
                                                                        ?.filter(
                                                                            (
                                                                                event,
                                                                            ) =>
                                                                                !event.success,
                                                                        )
                                                                        .slice(
                                                                            -3,
                                                                        ) // Show last 3 failures
                                                                        .map(
                                                                            (
                                                                                event,
                                                                                idx,
                                                                            ) => (
                                                                                <Box
                                                                                    key={
                                                                                        idx
                                                                                    }
                                                                                    sx={{
                                                                                        mb: 1,
                                                                                        p: 1,
                                                                                        backgroundColor:
                                                                                            "rgba(255, 0, 0, 0.1)",
                                                                                        borderRadius: 1,
                                                                                    }}
                                                                                >
                                                                                    <Typography
                                                                                        variant="caption"
                                                                                        sx={{
                                                                                            display:
                                                                                                "block",
                                                                                            fontSize:
                                                                                                "0.65rem",
                                                                                            color: "#ff6b6b",
                                                                                        }}
                                                                                    >
                                                                                        {new Date(
                                                                                            event.timestamp ||
                                                                                                "",
                                                                                        ).toLocaleString()}
                                                                                    </Typography>
                                                                                    <Typography
                                                                                        variant="caption"
                                                                                        sx={{
                                                                                            display:
                                                                                                "block",
                                                                                            fontSize:
                                                                                                "0.65rem",
                                                                                        }}
                                                                                    >
                                                                                        {event.context ===
                                                                                        "startup"
                                                                                            ? "🚀 Pre-Startup"
                                                                                            : "🛑 Post-Deletion"}{" "}
                                                                                        •{" "}
                                                                                        Slot:{" "}
                                                                                        {event.slot_id?.substring(
                                                                                            0,
                                                                                            8,
                                                                                        )}
                                                                                        ...
                                                                                        (
                                                                                        {
                                                                                            event.runtime
                                                                                        }

                                                                                        )
                                                                                    </Typography>
                                                                                    <Typography
                                                                                        variant="caption"
                                                                                        sx={{
                                                                                            display:
                                                                                                "block",
                                                                                            fontSize:
                                                                                                "0.65rem",
                                                                                        }}
                                                                                    >
                                                                                        {event.error_message ||
                                                                                            "Stabilization failed"}
                                                                                    </Typography>
                                                                                    <Typography
                                                                                        variant="caption"
                                                                                        sx={{
                                                                                            display:
                                                                                                "block",
                                                                                            fontSize:
                                                                                                "0.6rem",
                                                                                            color: "rgba(255, 255, 255, 0.5)",
                                                                                        }}
                                                                                    >
                                                                                        Polls:{" "}
                                                                                        {
                                                                                            event.polls_taken
                                                                                        }

                                                                                        ,
                                                                                        Wait:{" "}
                                                                                        {
                                                                                            event.total_wait_seconds
                                                                                        }

                                                                                        s
                                                                                    </Typography>
                                                                                    {event.memory_readings &&
                                                                                        event
                                                                                            .memory_readings
                                                                                            .length >
                                                                                            0 && (
                                                                                            <Box
                                                                                                sx={{
                                                                                                    mt: 0.5,
                                                                                                    p: 0.5,
                                                                                                    backgroundColor:
                                                                                                        "rgba(0, 0, 0, 0.2)",
                                                                                                    borderRadius: 0.5,
                                                                                                }}
                                                                                            >
                                                                                                <Typography
                                                                                                    variant="caption"
                                                                                                    sx={{
                                                                                                        display:
                                                                                                            "block",
                                                                                                        fontSize:
                                                                                                            "0.6rem",
                                                                                                        fontWeight: 600,
                                                                                                        mb: 0.5,
                                                                                                    }}
                                                                                                >
                                                                                                    Memory
                                                                                                    Readings:
                                                                                                </Typography>
                                                                                                {event.memory_readings.map(
                                                                                                    (
                                                                                                        reading,
                                                                                                        ridx,
                                                                                                    ) => (
                                                                                                        <Typography
                                                                                                            key={
                                                                                                                ridx
                                                                                                            }
                                                                                                            variant="caption"
                                                                                                            sx={{
                                                                                                                display:
                                                                                                                    "block",
                                                                                                                fontSize:
                                                                                                                    "0.55rem",
                                                                                                                color: reading.is_stable
                                                                                                                    ? "#4CAF50"
                                                                                                                    : "#ff6b6b",
                                                                                                                fontFamily:
                                                                                                                    "monospace",
                                                                                                            }}
                                                                                                        >
                                                                                                            Poll{" "}
                                                                                                            {
                                                                                                                reading.poll_number
                                                                                                            }

                                                                                                            :{" "}
                                                                                                            {(
                                                                                                                reading.memory_mb ||
                                                                                                                0
                                                                                                            ).toFixed(
                                                                                                                0,
                                                                                                            )}
                                                                                                            MB
                                                                                                            {reading.delta_mb !==
                                                                                                                undefined && (
                                                                                                                <>
                                                                                                                    {" "}
                                                                                                                    (
                                                                                                                    {reading.delta_mb >
                                                                                                                    0
                                                                                                                        ? "+"
                                                                                                                        : ""}
                                                                                                                    {
                                                                                                                        reading.delta_mb
                                                                                                                    }
                                                                                                                    MB
                                                                                                                    Δ)
                                                                                                                </>
                                                                                                            )}{" "}
                                                                                                            Stable:
                                                                                                            {
                                                                                                                reading.stable_count
                                                                                                            }

                                                                                                            /
                                                                                                            {
                                                                                                                event.required_stable_polls
                                                                                                            }
                                                                                                            {reading.is_stable
                                                                                                                ? " ✓"
                                                                                                                : " ✗"}
                                                                                                        </Typography>
                                                                                                    ),
                                                                                                )}
                                                                                                <Typography
                                                                                                    variant="caption"
                                                                                                    sx={{
                                                                                                        display:
                                                                                                            "block",
                                                                                                        fontSize:
                                                                                                            "0.55rem",
                                                                                                        color: "#ff6b6b",
                                                                                                        fontWeight: 600,
                                                                                                        mt: 0.5,
                                                                                                    }}
                                                                                                >
                                                                                                    Needed{" "}
                                                                                                    {
                                                                                                        event.required_stable_polls
                                                                                                    }{" "}
                                                                                                    consecutive
                                                                                                    stable
                                                                                                    polls
                                                                                                </Typography>
                                                                                            </Box>
                                                                                        )}
                                                                                </Box>
                                                                            ),
                                                                        )}
                                                                </Box>
                                                            ) : (
                                                                "No recent failures"
                                                            )
                                                        }
                                                        placement="top"
                                                        arrow
                                                    >
                                                        <Typography
                                                            variant="h6"
                                                            sx={{
                                                                color: "#F44336",
                                                                fontWeight: 600,
                                                                cursor: "help",
                                                            }}
                                                        >
                                                            {runner
                                                                .gpu_memory_stats
                                                                .failed_stabilizations ||
                                                                0}
                                                        </Typography>
                                                    </Tooltip>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            color: "rgba(255, 255, 255, 0.6)",
                                                            fontSize: "0.7rem",
                                                        }}
                                                    >
                                                        Failed Stabilizations
                                                    </Typography>
                                                </Box>
                                            </Grid>
                                            <Grid item xs={12} sm={6} md={3}>
                                                <Box
                                                    sx={{ textAlign: "center" }}
                                                >
                                                    <Typography
                                                        variant="h6"
                                                        sx={{
                                                            color: "#00c8ff",
                                                            fontWeight: 600,
                                                        }}
                                                    >
                                                        {runner.gpu_memory_stats
                                                            .average_wait_time_seconds
                                                            ? `${runner.gpu_memory_stats.average_wait_time_seconds.toFixed(1)}s`
                                                            : "0s"}
                                                    </Typography>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            color: "rgba(255, 255, 255, 0.6)",
                                                            fontSize: "0.7rem",
                                                        }}
                                                    >
                                                        Avg Wait Time
                                                    </Typography>
                                                </Box>
                                            </Grid>
                                            <Grid item xs={12} sm={6} md={3}>
                                                <Box
                                                    sx={{ textAlign: "center" }}
                                                >
                                                    <Typography
                                                        variant="h6"
                                                        sx={{
                                                            color: "#FF9800",
                                                            fontWeight: 600,
                                                        }}
                                                    >
                                                        {runner.gpu_memory_stats
                                                            .memory_time_series
                                                            ?.length || 0}
                                                    </Typography>
                                                    <Typography
                                                        variant="caption"
                                                        sx={{
                                                            color: "rgba(255, 255, 255, 0.6)",
                                                            fontSize: "0.7rem",
                                                        }}
                                                    >
                                                        Data Points
                                                    </Typography>
                                                </Box>
                                            </Grid>
                                        </Grid>
                                    </Box>
                                </Box>
                            </AccordionDetails>
                        </Accordion>
                    )}

                {/* GPU Cards Section */}
                {runner.gpus && runner.gpus.length > 0 ? (
                    <Grid container spacing={2}>
                        {(() => {
                            const gpuAllocatedMemory =
                                calculateGPUAllocatedMemory(runner);
                            const gpuAllocatedModels =
                                calculateGPUAllocatedModels(runner);
                            // Sort GPUs by index to ensure consistent ordering (GPU 0, GPU 1, etc.)
                            const sortedGPUs = [...(runner.gpus || [])].sort(
                                (a, b) => {
                                    const indexA = a.index ?? 999;
                                    const indexB = b.index ?? 999;
                                    return indexA - indexB;
                                },
                            );
                            return sortedGPUs.map((gpu) => (
                                <Grid
                                    item
                                    xs={12}
                                    sm={6}
                                    md={4}
                                    key={`gpu-card-${gpu.index}`}
                                >
                                    <GPUCard
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
                                        runner={runner}
                                    />
                                </Grid>
                            ));
                        })()}
                    </Grid>
                ) : (
                    // Fallback to aggregated memory display
                    <Grid container spacing={2} alignItems="center">
                        <Grid item xs={12} md={5}>
                            <Box
                                sx={{
                                    display: "flex",
                                    flexDirection: "column",
                                }}
                            >
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
                                            color: "#00c8ff",
                                            fontWeight: 600,
                                        }}
                                    >
                                        Actual: {prettyBytes(used_memory)} (
                                        {actualPercent}%)
                                    </Typography>
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            color: "rgba(255, 255, 255, 0.5)",
                                            fontWeight: 500,
                                        }}
                                    >
                                        Total: {prettyBytes(total_memory)}
                                    </Typography>
                                </Box>
                                <Box
                                    sx={{
                                        display: "flex",
                                        justifyContent: "space-between",
                                    }}
                                >
                                    <Typography
                                        variant="caption"
                                        sx={{
                                            color: "#7986cb",
                                            fontWeight: 600,
                                        }}
                                    >
                                        Allocated:{" "}
                                        {prettyBytes(allocated_memory)} (
                                        {allocatedPercent}%)
                                    </Typography>
                                </Box>
                            </Box>
                        </Grid>

                        <Grid item xs={12} md={7}>
                            <Box
                                sx={{
                                    position: "relative",
                                    display: "flex",
                                    alignItems: "center",
                                    height: 20,
                                }}
                            >
                                {/* Memory usage background with shine effect */}
                                <Box
                                    sx={{
                                        position: "absolute",
                                        width: "100%",
                                        height: 12,
                                        backgroundColor:
                                            "rgba(255, 255, 255, 0.03)",
                                        borderRadius: "4px",
                                        boxShadow:
                                            "inset 0 1px 3px rgba(0,0,0,0.3)",
                                        overflow: "hidden",
                                    }}
                                />

                                {/* Allocated memory bar */}
                                <LinearProgress
                                    variant="determinate"
                                    value={
                                        (100 * allocated_memory) / total_memory
                                    }
                                    sx={{
                                        width: "100%",
                                        height: 12,
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

                                {/* Actual memory bar */}
                                <LinearProgress
                                    variant="determinate"
                                    value={(100 * used_memory) / total_memory}
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
                        </Grid>
                    </Grid>
                )}

                {/* Model Status Section */}
                {runner.models && runner.models.length > 0 && (
                    <>
                        <Divider
                            sx={{
                                my: 2,
                                borderColor: "rgba(255, 255, 255, 0.06)",
                                boxShadow: "0 1px 2px rgba(0, 0, 0, 0.1)",
                            }}
                        />
                        <Typography
                            variant="caption"
                            sx={{
                                color: "rgba(255, 255, 255, 0.6)",
                                fontWeight: 500,
                                px: 1,
                                mb: 1,
                                display: "block",
                            }}
                        >
                            Available models:
                        </Typography>
                        <Grid container spacing={1} sx={{ px: 1, py: 0.5 }}>
                            {runner.models.map((modelStatus) => (
                                <Grid item key={modelStatus.model_id}>
                                    <Box
                                        sx={{
                                            display: "flex",
                                            alignItems: "center",
                                            gap: 0.5,
                                        }}
                                    >
                                        <Tooltip
                                            title={
                                                modelStatus.error ||
                                                `Runtime: ${modelStatus.runtime || "unknown"}`
                                            }
                                            disableHoverListener={
                                                !modelStatus.error &&
                                                !modelStatus.runtime
                                            }
                                        >
                                            <Chip
                                                size="small"
                                                label={
                                                    modelStatus.error
                                                        ? `${modelStatus.model_id} (Error)`
                                                        : modelStatus.download_in_progress
                                                          ? `${modelStatus.model_id} (Downloading: ${modelStatus.download_percent}%)`
                                                          : `${modelStatus.model_id}${modelStatus.memory ? ` (${prettyBytes(modelStatus.memory)})` : ""}`
                                                }
                                                sx={{
                                                    borderRadius: "3px",
                                                    backgroundColor:
                                                        modelStatus.error
                                                            ? "rgba(255, 0, 0, 0.15)" // Red tint for error
                                                            : modelStatus.download_in_progress
                                                              ? "rgba(255, 165, 0, 0.15)" // Orange tint for downloading
                                                              : modelStatus.runtime ===
                                                                  "vllm"
                                                                ? "rgba(147, 51, 234, 0.08)" // Purple tint for VLLM
                                                                : modelStatus.runtime ===
                                                                    "ollama"
                                                                  ? "rgba(0, 200, 255, 0.08)" // Blue tint for Ollama
                                                                  : "rgba(34, 197, 94, 0.08)", // Green tint for other runtimes
                                                    border: "1px solid",
                                                    borderColor:
                                                        modelStatus.error
                                                            ? "rgba(255, 0, 0, 0.3)" // Red border for error
                                                            : modelStatus.download_in_progress
                                                              ? "rgba(255, 165, 0, 0.3)" // Orange border for downloading
                                                              : modelStatus.runtime ===
                                                                  "vllm"
                                                                ? "rgba(147, 51, 234, 0.2)" // Purple border for VLLM
                                                                : modelStatus.runtime ===
                                                                    "ollama"
                                                                  ? "rgba(0, 200, 255, 0.2)" // Blue border for Ollama
                                                                  : "rgba(34, 197, 94, 0.2)", // Green border for other runtimes
                                                    color: modelStatus.error
                                                        ? "rgba(255, 0, 0, 0.9)" // Brighter red text for error
                                                        : modelStatus.download_in_progress
                                                          ? "rgba(255, 165, 0, 0.9)" // Brighter orange text for downloading
                                                          : "rgba(255, 255, 255, 0.85)",
                                                    "& .MuiChip-label": {
                                                        fontSize: "0.7rem",
                                                        px: 1.2,
                                                    },
                                                }}
                                            />
                                        </Tooltip>
                                        {modelStatus.download_in_progress && (
                                            <CircularProgress
                                                size={12}
                                                thickness={4}
                                                sx={{
                                                    color: "rgba(255, 165, 0, 0.9)",
                                                }}
                                            />
                                        )}
                                    </Box>
                                </Grid>
                            ))}
                        </Grid>
                    </>
                )}
            </Box>

            {runner.slots && runner.slots.length > 0 && (
                <Box
                    sx={{
                        mt: 1,
                        backgroundColor: "rgba(17, 17, 19, 0.9)",
                        pt: 1,
                        pb: 1,
                        borderTop: "1px solid rgba(255, 255, 255, 0.05)",
                        boxShadow: "inset 0 2px 4px rgba(0, 0, 0, 0.1)",
                    }}
                >
                    {runner.slots
                        ?.sort((a, b) => {
                            // Safely handle potentially undefined id properties
                            const idA = a.id || "";
                            const idB = b.id || "";
                            return idA.localeCompare(idB);
                        })
                        .map((slot) => (
                            <ModelInstanceSummary
                                key={slot?.id}
                                slot={slot}
                                models={runner.models}
                                onViewSession={onViewSession}
                            />
                        ))}
                </Box>
            )}

            {(!runner.slots || runner.slots.length === 0) && (
                <Box
                    sx={{
                        backgroundColor: "rgba(17, 17, 19, 0.9)",
                        p: 3,
                        textAlign: "center",
                        borderTop: "1px solid rgba(255, 255, 255, 0.05)",
                        boxShadow: "inset 0 2px 4px rgba(0, 0, 0, 0.1)",
                    }}
                >
                    <Typography
                        variant="body2"
                        sx={{
                            color: "rgba(255, 255, 255, 0.4)",
                            fontStyle: "italic",
                            letterSpacing: "0.5px",
                            display: "flex",
                            alignItems: "center",
                            justifyContent: "center",
                            gap: 1,
                            "&::before, &::after": {
                                content: '""',
                                height: "1px",
                                width: "50px",
                                background:
                                    "linear-gradient(90deg, rgba(255,255,255,0) 0%, rgba(255,255,255,0.1) 50%, rgba(255,255,255,0) 100%)",
                            },
                        }}
                    >
                        No active model instances
                    </Typography>
                </Box>
            )}

            {/* Model Instance Logs */}
            <Box sx={{ mt: 3 }}>
                <ModelInstanceLogs runner={runner} />
            </Box>
        </Paper>
    );
};

export default RunnerSummary;
