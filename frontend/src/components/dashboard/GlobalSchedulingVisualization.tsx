import React, { FC, useState, useMemo } from "react";
import {
    Box,
    Card,
    CardContent,
    Typography,
    Accordion,
    AccordionSummary,
    AccordionDetails,
    Chip,
    Grid,
    LinearProgress,
    Tooltip,
    IconButton,
    Avatar,
    Divider,
    Stack,
    Paper,
    Table,
    TableBody,
    TableCell,
    TableContainer,
    TableHead,
    TableRow,
    useTheme,
    alpha,
    Badge,
    Fade,
    Collapse,
} from "@mui/material";
import {
    ExpandMore as ExpandMoreIcon,
    Speed as SpeedIcon,
    Memory as MemoryIcon,
    Computer as CpuIcon,
    CheckCircle as CheckIcon,
    Error as ErrorIcon,
    Schedule as ScheduleIcon,
    Timeline as TimelineIcon,
    Visibility as VisibilityIcon,
    VisibilityOff as VisibilityOffIcon,
    Star as StarIcon,
    TrendingUp as OptimizationIcon,
    AccessTime as TimerIcon,
    Storage as StorageIcon,
    DeviceHub as NetworkIcon,
    Layers as LayersIcon,
} from "@mui/icons-material";
// Manual type definitions until API types are regenerated
interface TypesGPUState {
    index: number;
    total_memory: number;
    allocated_memory: number;
    free_memory: number;
    active_slots: string[];
    utilization: number;
}

interface TypesRunnerStateView {
    runner_id: string;
    gpu_states: { [key: string]: TypesGPUState };
    total_slots: number;
    active_slots: number;
    warm_slots: number;
    is_connected: boolean;
}

interface TypesAllocationPlanView {
    id: string;
    runner_id: string;
    gpus: number[];
    gpu_count: number;
    is_multi_gpu: boolean;
    total_memory_required: number;
    memory_per_gpu: number;
    cost: number;
    requires_eviction: boolean;
    evictions_needed: string[];
    tensor_parallel_size: number;
    runtime: string;
    is_valid: boolean;
    validation_error?: string;
    runner_memory_state: { [key: string]: number };
    runner_capacity: { [key: string]: number };
}

interface TypesGlobalAllocationDecision {
    id: string;
    created: string;
    workload_id: string;
    session_id: string;
    model_name: string;
    runtime: string;
    considered_plans: TypesAllocationPlanView[];
    selected_plan: TypesAllocationPlanView | null;
    planning_time_ms: number;
    execution_time_ms: number;
    total_time_ms: number;
    success: boolean;
    reason: string;
    error_message?: string;
    before_state: { [key: string]: TypesRunnerStateView };
    after_state: { [key: string]: TypesRunnerStateView };
    total_runners_evaluated: number;
    total_plans_generated: number;
    optimization_score: number;
}

interface GlobalSchedulingVisualizationProps {
    decisions: TypesGlobalAllocationDecision[];
}

const GlobalSchedulingVisualization: FC<GlobalSchedulingVisualizationProps> = ({
    decisions,
}) => {
    const theme = useTheme();
    const [expanded, setExpanded] = useState<string | false>(false);
    const [showDetails, setShowDetails] = useState<{ [key: string]: boolean }>(
        {},
    );

    const handleAccordionChange =
        (panel: string) =>
        (event: React.SyntheticEvent, isExpanded: boolean) => {
            setExpanded(isExpanded ? panel : false);
        };

    const toggleDetails = (decisionId: string) => {
        setShowDetails((prev) => ({
            ...prev,
            [decisionId]: !prev[decisionId],
        }));
    };

    const formatMemory = (bytes: number) => {
        const gb = bytes / (1024 * 1024 * 1024);
        return `${gb.toFixed(1)}GB`;
    };

    const getSuccessColor = (success: boolean) =>
        success ? "#4caf50" : "#f44336";
    const getOptimizationColor = (score: number) => {
        if (score >= 0.8) return "#4caf50";
        if (score >= 0.6) return "#ff9800";
        return "#f44336";
    };

    const renderGPUVisualization = (
        gpuStates: { [key: string]: TypesGPUState },
        title: string,
        runnerName: string,
    ) => (
        <Paper
            elevation={1}
            sx={{
                p: 2,
                background: `linear-gradient(135deg, ${alpha(theme.palette.primary.main, 0.1)}, ${alpha(theme.palette.secondary.main, 0.1)})`,
                border: `1px solid ${alpha(theme.palette.primary.main, 0.2)}`,
            }}
        >
            <Typography
                variant="subtitle2"
                gutterBottom
                sx={{ fontWeight: 600, color: theme.palette.primary.main }}
            >
                {title} - {runnerName}
            </Typography>
            <Grid container spacing={1}>
                {Object.values(gpuStates || {}).map((gpu) => (
                    <Grid item xs={12} md={6} key={gpu.index}>
                        <Box
                            sx={{
                                p: 1.5,
                                backgroundColor: alpha(
                                    theme.palette.background.paper,
                                    0.8,
                                ),
                                borderRadius: 1,
                                border: `1px solid ${alpha(theme.palette.divider, 0.5)}`,
                            }}
                        >
                            <Stack
                                direction="row"
                                alignItems="center"
                                spacing={1}
                                mb={1}
                            >
                                <CpuIcon fontSize="small" color="primary" />
                                <Typography variant="body2" fontWeight={500}>
                                    GPU {gpu.index}
                                </Typography>
                                <Chip
                                    size="small"
                                    label={`${(gpu.utilization * 100).toFixed(1)}%`}
                                    color={
                                        gpu.utilization > 0.8
                                            ? "error"
                                            : gpu.utilization > 0.5
                                              ? "warning"
                                              : "success"
                                    }
                                    sx={{ ml: "auto", fontSize: "0.7rem" }}
                                />
                            </Stack>

                            <Box sx={{ mb: 1 }}>
                                <Stack
                                    direction="row"
                                    justifyContent="space-between"
                                    mb={0.5}
                                >
                                    <Typography
                                        variant="caption"
                                        color="text.secondary"
                                    >
                                        Memory Usage
                                    </Typography>
                                    <Typography
                                        variant="caption"
                                        fontWeight={500}
                                    >
                                        {formatMemory(gpu.allocated_memory)} /{" "}
                                        {formatMemory(gpu.total_memory)}
                                    </Typography>
                                </Stack>
                                <LinearProgress
                                    variant="determinate"
                                    value={
                                        (gpu.allocated_memory /
                                            gpu.total_memory) *
                                        100
                                    }
                                    sx={{
                                        height: 6,
                                        borderRadius: 3,
                                        backgroundColor: alpha(
                                            theme.palette.grey[300],
                                            0.3,
                                        ),
                                        "& .MuiLinearProgress-bar": {
                                            background: `linear-gradient(90deg, ${theme.palette.success.main}, ${theme.palette.warning.main})`,
                                            borderRadius: 3,
                                        },
                                    }}
                                />
                            </Box>

                            {gpu.active_slots &&
                                gpu.active_slots.length > 0 && (
                                    <Box>
                                        <Typography
                                            variant="caption"
                                            color="text.secondary"
                                            display="block"
                                            mb={0.5}
                                        >
                                            Active Slots (
                                            {gpu.active_slots.length})
                                        </Typography>
                                        <Stack
                                            direction="row"
                                            spacing={0.5}
                                            flexWrap="wrap"
                                        >
                                            {gpu.active_slots
                                                .slice(0, 3)
                                                .map((slotId) => (
                                                    <Chip
                                                        key={slotId}
                                                        label={slotId.slice(
                                                            0,
                                                            8,
                                                        )}
                                                        size="small"
                                                        variant="outlined"
                                                        sx={{
                                                            fontSize: "0.6rem",
                                                            height: 20,
                                                        }}
                                                    />
                                                ))}
                                            {gpu.active_slots.length > 3 && (
                                                <Chip
                                                    label={`+${gpu.active_slots.length - 3}`}
                                                    size="small"
                                                    color="secondary"
                                                    sx={{
                                                        fontSize: "0.6rem",
                                                        height: 20,
                                                    }}
                                                />
                                            )}
                                        </Stack>
                                    </Box>
                                )}
                        </Box>
                    </Grid>
                ))}
            </Grid>
        </Paper>
    );

    const renderPlanComparison = (
        plans: TypesAllocationPlanView[],
        selectedPlan: TypesAllocationPlanView | null,
    ) => (
        <Box>
            <Typography
                variant="h6"
                gutterBottom
                sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    color: theme.palette.primary.main,
                }}
            >
                <LayersIcon />
                Allocation Plans ({plans.length} considered)
            </Typography>

            <Grid container spacing={2}>
                {plans.map((plan, index) => {
                    const isSelected = selectedPlan?.id === plan.id;
                    const isOptimal =
                        index === 0 ||
                        plan.cost === Math.min(...plans.map((p) => p.cost));

                    return (
                        <Grid item xs={12} md={6} lg={4} key={plan.id}>
                            <Card
                                elevation={isSelected ? 8 : 2}
                                sx={{
                                    position: "relative",
                                    background: isSelected
                                        ? `linear-gradient(135deg, ${alpha(theme.palette.success.main, 0.15)}, ${alpha(theme.palette.success.light, 0.1)})`
                                        : "background.paper",
                                    border: isSelected
                                        ? `2px solid ${theme.palette.success.main}`
                                        : `1px solid ${alpha(theme.palette.divider, 0.3)}`,
                                    transition:
                                        "all 0.3s cubic-bezier(0.4, 0, 0.2, 1)",
                                    transform: isSelected
                                        ? "translateY(-4px)"
                                        : "none",
                                    "&:hover": {
                                        transform: isSelected
                                            ? "translateY(-6px)"
                                            : "translateY(-2px)",
                                        boxShadow:
                                            theme.shadows[isSelected ? 12 : 6],
                                    },
                                }}
                            >
                                {isSelected && (
                                    <Box
                                        sx={{
                                            position: "absolute",
                                            top: -1,
                                            right: -1,
                                            width: 0,
                                            height: 0,
                                            borderLeft:
                                                "20px solid transparent",
                                            borderTop: `20px solid ${theme.palette.success.main}`,
                                        }}
                                    />
                                )}
                                {isSelected && (
                                    <CheckIcon
                                        sx={{
                                            position: "absolute",
                                            top: 2,
                                            right: 2,
                                            color: "white",
                                            fontSize: 12,
                                            zIndex: 1,
                                        }}
                                    />
                                )}

                                <CardContent sx={{ p: 2 }}>
                                    <Stack spacing={2}>
                                        <Box>
                                            <Stack
                                                direction="row"
                                                alignItems="center"
                                                justifyContent="space-between"
                                            >
                                                <Typography
                                                    variant="subtitle2"
                                                    fontWeight={600}
                                                >
                                                    {plan.runner_id.slice(
                                                        0,
                                                        12,
                                                    )}
                                                    ...
                                                </Typography>
                                                <Stack
                                                    direction="row"
                                                    spacing={0.5}
                                                >
                                                    {isSelected && (
                                                        <Chip
                                                            icon={
                                                                <StarIcon
                                                                    sx={{
                                                                        fontSize: 12,
                                                                    }}
                                                                />
                                                            }
                                                            label="SELECTED"
                                                            size="small"
                                                            color="success"
                                                            sx={{
                                                                fontSize:
                                                                    "0.65rem",
                                                                height: 20,
                                                            }}
                                                        />
                                                    )}
                                                    {isOptimal &&
                                                        !isSelected && (
                                                            <Chip
                                                                label="OPTIMAL"
                                                                size="small"
                                                                color="primary"
                                                                variant="outlined"
                                                                sx={{
                                                                    fontSize:
                                                                        "0.65rem",
                                                                    height: 20,
                                                                }}
                                                            />
                                                        )}
                                                </Stack>
                                            </Stack>
                                        </Box>

                                        <Grid container spacing={1}>
                                            <Grid item xs={6}>
                                                <Box textAlign="center">
                                                    <Typography
                                                        variant="caption"
                                                        color="text.secondary"
                                                    >
                                                        GPUs
                                                    </Typography>
                                                    <Typography
                                                        variant="h6"
                                                        color="primary.main"
                                                    >
                                                        {plan.gpu_count}
                                                    </Typography>
                                                    <Typography
                                                        variant="caption"
                                                        color="text.secondary"
                                                    >
                                                        {plan.gpus?.join(", ")}
                                                    </Typography>
                                                </Box>
                                            </Grid>
                                            <Grid item xs={6}>
                                                <Box textAlign="center">
                                                    <Typography
                                                        variant="caption"
                                                        color="text.secondary"
                                                    >
                                                        Memory
                                                    </Typography>
                                                    <Typography
                                                        variant="h6"
                                                        color="secondary.main"
                                                    >
                                                        {formatMemory(
                                                            plan.total_memory_required,
                                                        )}
                                                    </Typography>
                                                    <Typography
                                                        variant="caption"
                                                        color="text.secondary"
                                                    >
                                                        {formatMemory(
                                                            plan.memory_per_gpu,
                                                        )}
                                                        /GPU
                                                    </Typography>
                                                </Box>
                                            </Grid>
                                        </Grid>

                                        <Stack spacing={1}>
                                            <Stack
                                                direction="row"
                                                justifyContent="space-between"
                                                alignItems="center"
                                            >
                                                <Typography
                                                    variant="caption"
                                                    color="text.secondary"
                                                >
                                                    Cost Score
                                                </Typography>
                                                <Chip
                                                    label={plan.cost}
                                                    size="small"
                                                    color={
                                                        plan.cost ===
                                                        Math.min(
                                                            ...plans.map(
                                                                (p) => p.cost,
                                                            ),
                                                        )
                                                            ? "success"
                                                            : "default"
                                                    }
                                                    sx={{ fontSize: "0.7rem" }}
                                                />
                                            </Stack>

                                            {plan.requires_eviction && (
                                                <Stack
                                                    direction="row"
                                                    justifyContent="space-between"
                                                    alignItems="center"
                                                >
                                                    <Typography
                                                        variant="caption"
                                                        color="text.secondary"
                                                    >
                                                        Evictions
                                                    </Typography>
                                                    <Chip
                                                        label={
                                                            plan
                                                                .evictions_needed
                                                                ?.length || 0
                                                        }
                                                        size="small"
                                                        color="warning"
                                                        sx={{
                                                            fontSize: "0.7rem",
                                                        }}
                                                    />
                                                </Stack>
                                            )}

                                            <Stack
                                                direction="row"
                                                justifyContent="space-between"
                                                alignItems="center"
                                            >
                                                <Typography
                                                    variant="caption"
                                                    color="text.secondary"
                                                >
                                                    Runtime
                                                </Typography>
                                                <Chip
                                                    label={plan.runtime}
                                                    size="small"
                                                    variant="outlined"
                                                    sx={{ fontSize: "0.7rem" }}
                                                />
                                            </Stack>

                                            {!plan.is_valid && (
                                                <Box>
                                                    <Chip
                                                        icon={
                                                            <ErrorIcon
                                                                sx={{
                                                                    fontSize: 12,
                                                                }}
                                                            />
                                                        }
                                                        label="Invalid"
                                                        size="small"
                                                        color="error"
                                                        sx={{
                                                            fontSize: "0.7rem",
                                                        }}
                                                    />
                                                    {plan.validation_error && (
                                                        <Typography
                                                            variant="caption"
                                                            color="error.main"
                                                            display="block"
                                                            mt={0.5}
                                                        >
                                                            {
                                                                plan.validation_error
                                                            }
                                                        </Typography>
                                                    )}
                                                </Box>
                                            )}
                                        </Stack>
                                    </Stack>
                                </CardContent>
                            </Card>
                        </Grid>
                    );
                })}
            </Grid>
        </Box>
    );

    const renderPerformanceMetrics = (
        decision: TypesGlobalAllocationDecision,
    ) => (
        <Grid container spacing={2}>
            <Grid item xs={12} sm={6} md={3}>
                <Paper
                    elevation={1}
                    sx={{
                        p: 2,
                        textAlign: "center",
                        background: `linear-gradient(135deg, ${alpha(theme.palette.info.main, 0.1)}, ${alpha(theme.palette.info.light, 0.05)})`,
                        border: `1px solid ${alpha(theme.palette.info.main, 0.2)}`,
                    }}
                >
                    <TimerIcon color="info" sx={{ fontSize: 32, mb: 1 }} />
                    <Typography variant="h6" color="info.main">
                        {decision.total_time_ms}ms
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                        Total Time
                    </Typography>
                </Paper>
            </Grid>

            <Grid item xs={12} sm={6} md={3}>
                <Paper
                    elevation={1}
                    sx={{
                        p: 2,
                        textAlign: "center",
                        background: `linear-gradient(135deg, ${alpha(theme.palette.success.main, 0.1)}, ${alpha(theme.palette.success.light, 0.05)})`,
                        border: `1px solid ${alpha(theme.palette.success.main, 0.2)}`,
                    }}
                >
                    <OptimizationIcon
                        color="success"
                        sx={{ fontSize: 32, mb: 1 }}
                    />
                    <Typography variant="h6" color="success.main">
                        {(decision.optimization_score * 100).toFixed(0)}%
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                        Optimization
                    </Typography>
                </Paper>
            </Grid>

            <Grid item xs={12} sm={6} md={3}>
                <Paper
                    elevation={1}
                    sx={{
                        p: 2,
                        textAlign: "center",
                        background: `linear-gradient(135deg, ${alpha(theme.palette.warning.main, 0.1)}, ${alpha(theme.palette.warning.light, 0.05)})`,
                        border: `1px solid ${alpha(theme.palette.warning.main, 0.2)}`,
                    }}
                >
                    <NetworkIcon color="warning" sx={{ fontSize: 32, mb: 1 }} />
                    <Typography variant="h6" color="warning.main">
                        {decision.total_runners_evaluated}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                        Runners
                    </Typography>
                </Paper>
            </Grid>

            <Grid item xs={12} sm={6} md={3}>
                <Paper
                    elevation={1}
                    sx={{
                        p: 2,
                        textAlign: "center",
                        background: `linear-gradient(135deg, ${alpha(theme.palette.secondary.main, 0.1)}, ${alpha(theme.palette.secondary.light, 0.05)})`,
                        border: `1px solid ${alpha(theme.palette.secondary.main, 0.2)}`,
                    }}
                >
                    <LayersIcon
                        color="secondary"
                        sx={{ fontSize: 32, mb: 1 }}
                    />
                    <Typography variant="h6" color="secondary.main">
                        {decision.total_plans_generated}
                    </Typography>
                    <Typography variant="caption" color="text.secondary">
                        Plans
                    </Typography>
                </Paper>
            </Grid>
        </Grid>
    );

    if (!decisions || decisions.length === 0) {
        return (
            <Paper
                sx={{
                    p: 4,
                    textAlign: "center",
                    background: `linear-gradient(135deg, ${alpha(theme.palette.primary.main, 0.05)}, ${alpha(theme.palette.secondary.main, 0.05)})`,
                    border: `1px solid ${alpha(theme.palette.divider, 0.1)}`,
                }}
            >
                <TimelineIcon
                    sx={{ fontSize: 64, color: "text.secondary", mb: 2 }}
                />
                <Typography variant="h6" color="text.secondary" gutterBottom>
                    No Global Scheduling Decisions Yet
                </Typography>
                <Typography variant="body2" color="text.secondary">
                    Global allocation decisions will appear here as the
                    scheduler makes placement choices.
                </Typography>
            </Paper>
        );
    }

    return (
        <Box>
            <Typography
                variant="h5"
                gutterBottom
                sx={{
                    display: "flex",
                    alignItems: "center",
                    gap: 1,
                    mb: 3,
                    background: `linear-gradient(45deg, ${theme.palette.primary.main}, ${theme.palette.secondary.main})`,
                    backgroundClip: "text",
                    WebkitBackgroundClip: "text",
                    WebkitTextFillColor: "transparent",
                    fontWeight: 700,
                }}
            >
                <TimelineIcon sx={{ color: theme.palette.primary.main }} />
                Global Scheduling Decisions
                <Chip
                    label={decisions.length}
                    size="small"
                    color="primary"
                    sx={{ ml: 1 }}
                />
            </Typography>

            <Stack spacing={2}>
                {decisions.map((decision, index) => (
                    <Fade
                        in={true}
                        timeout={300 + index * 100}
                        key={decision.id}
                    >
                        <Accordion
                            expanded={expanded === decision.id}
                            onChange={handleAccordionChange(decision.id)}
                            sx={{
                                background: decision.success
                                    ? `linear-gradient(135deg, ${alpha(theme.palette.success.main, 0.08)}, ${alpha(theme.palette.success.light, 0.03)})`
                                    : `linear-gradient(135deg, ${alpha(theme.palette.error.main, 0.08)}, ${alpha(theme.palette.error.light, 0.03)})`,
                                border: `1px solid ${alpha(decision.success ? theme.palette.success.main : theme.palette.error.main, 0.2)}`,
                                boxShadow: decision.success
                                    ? theme.shadows[2]
                                    : theme.shadows[1],
                                "&:before": { display: "none" },
                                "&.Mui-expanded": {
                                    margin: 0,
                                    boxShadow: decision.success
                                        ? theme.shadows[6]
                                        : theme.shadows[4],
                                },
                            }}
                        >
                            <AccordionSummary
                                expandIcon={<ExpandMoreIcon />}
                                sx={{
                                    "& .MuiAccordionSummary-content": {
                                        alignItems: "center",
                                    },
                                }}
                            >
                                <Grid container alignItems="center" spacing={2}>
                                    <Grid item>
                                        <Avatar
                                            sx={{
                                                bgcolor: getSuccessColor(
                                                    decision.success,
                                                ),
                                                width: 40,
                                                height: 40,
                                            }}
                                        >
                                            {decision.success ? (
                                                <CheckIcon />
                                            ) : (
                                                <ErrorIcon />
                                            )}
                                        </Avatar>
                                    </Grid>

                                    <Grid item xs>
                                        <Typography
                                            variant="h6"
                                            component="div"
                                        >
                                            {decision.model_name}
                                        </Typography>
                                        <Typography
                                            variant="body2"
                                            color="text.secondary"
                                        >
                                            {decision.reason}
                                        </Typography>
                                    </Grid>

                                    <Grid item>
                                        <Stack
                                            direction="row"
                                            spacing={1}
                                            alignItems="center"
                                        >
                                            <Chip
                                                icon={
                                                    <SpeedIcon
                                                        sx={{ fontSize: 16 }}
                                                    />
                                                }
                                                label={`${decision.total_time_ms}ms`}
                                                size="small"
                                                color={
                                                    decision.total_time_ms < 100
                                                        ? "success"
                                                        : decision.total_time_ms <
                                                            500
                                                          ? "warning"
                                                          : "error"
                                                }
                                            />
                                            <Chip
                                                icon={
                                                    <StarIcon
                                                        sx={{ fontSize: 16 }}
                                                    />
                                                }
                                                label={`${(decision.optimization_score * 100).toFixed(0)}%`}
                                                size="small"
                                                sx={{
                                                    bgcolor: alpha(
                                                        getOptimizationColor(
                                                            decision.optimization_score,
                                                        ),
                                                        0.1,
                                                    ),
                                                    color: getOptimizationColor(
                                                        decision.optimization_score,
                                                    ),
                                                }}
                                            />
                                            <Chip
                                                label={decision.runtime}
                                                size="small"
                                                variant="outlined"
                                            />
                                        </Stack>
                                    </Grid>

                                    <Grid item>
                                        <Typography
                                            variant="caption"
                                            color="text.secondary"
                                        >
                                            {new Date(
                                                decision.created,
                                            ).toLocaleTimeString()}
                                        </Typography>
                                    </Grid>
                                </Grid>
                            </AccordionSummary>

                            <AccordionDetails sx={{ pt: 0 }}>
                                <Stack spacing={3}>
                                    {/* Performance Metrics */}
                                    {renderPerformanceMetrics(decision)}

                                    <Divider />

                                    {/* Plan Comparison */}
                                    {decision.considered_plans &&
                                        decision.considered_plans.length >
                                            0 && (
                                            <>
                                                {renderPlanComparison(
                                                    decision.considered_plans,
                                                    decision.selected_plan,
                                                )}
                                                <Divider />
                                            </>
                                        )}

                                    {/* Before/After States */}
                                    <Box>
                                        <Stack
                                            direction="row"
                                            justifyContent="space-between"
                                            alignItems="center"
                                            mb={2}
                                        >
                                            <Typography
                                                variant="h6"
                                                sx={{
                                                    display: "flex",
                                                    alignItems: "center",
                                                    gap: 1,
                                                    color: theme.palette.primary
                                                        .main,
                                                }}
                                            >
                                                <StorageIcon />
                                                Resource State Comparison
                                            </Typography>
                                            <IconButton
                                                onClick={() =>
                                                    toggleDetails(decision.id)
                                                }
                                                size="small"
                                                color="primary"
                                            >
                                                {showDetails[decision.id] ? (
                                                    <VisibilityOffIcon />
                                                ) : (
                                                    <VisibilityIcon />
                                                )}
                                            </IconButton>
                                        </Stack>

                                        <Collapse in={showDetails[decision.id]}>
                                            <Grid container spacing={2}>
                                                {decision.before_state &&
                                                    Object.entries(
                                                        decision.before_state,
                                                    ).map(
                                                        ([runnerId, state]) => (
                                                            <Grid
                                                                item
                                                                xs={12}
                                                                lg={6}
                                                                key={`before-${runnerId}`}
                                                            >
                                                                {renderGPUVisualization(
                                                                    state.gpu_states,
                                                                    "Before",
                                                                    runnerId,
                                                                )}
                                                            </Grid>
                                                        ),
                                                    )}

                                                {decision.after_state &&
                                                    Object.entries(
                                                        decision.after_state,
                                                    ).map(
                                                        ([runnerId, state]) => (
                                                            <Grid
                                                                item
                                                                xs={12}
                                                                lg={6}
                                                                key={`after-${runnerId}`}
                                                            >
                                                                {renderGPUVisualization(
                                                                    state.gpu_states,
                                                                    "After",
                                                                    runnerId,
                                                                )}
                                                            </Grid>
                                                        ),
                                                    )}
                                            </Grid>
                                        </Collapse>
                                    </Box>
                                </Stack>
                            </AccordionDetails>
                        </Accordion>
                    </Fade>
                ))}
            </Stack>
        </Box>
    );
};

export default GlobalSchedulingVisualization;
