import React, { useState, useEffect, useCallback, useRef } from "react";
import {
    Box,
    Card,
    CardContent,
    Typography,
    Slider,
    TextField,
    Grid,
    Chip,
    CircularProgress,
    Alert,
    Tooltip,
    IconButton,
} from "@mui/material";
import { Refresh as RefreshIcon } from "@mui/icons-material";
import { useMemoryEstimation } from "../../hooks/useMemoryEstimation";

interface MemoryEstimationWidgetProps {
    modelId: string;
    currentContextLength: number;
    onContextLengthChange: (value: number) => void;
    currentConcurrency: number;
    onConcurrencyChange: (value: number) => void;
    disabled?: boolean;
}

// Common context length values in K (thousands)
const CONTEXT_PRESETS = [4, 8, 16, 32, 40, 64, 100, 128, 200, 256, 512, 1000];

const MemoryEstimationWidget: React.FC<MemoryEstimationWidgetProps> = ({
    modelId,
    currentContextLength,
    onContextLengthChange,
    currentConcurrency,
    onConcurrencyChange,
    disabled = false,
}) => {
    // Convert context length to logarithmic scale for better distribution
    const contextToLogScale = (contextK: number) => Math.log10(contextK);
    const logScaleToContext = (logValue: number) => Math.pow(10, logValue);

    const [sliderValue, setSliderValue] = useState(
        contextToLogScale(currentContextLength / 1000),
    );

    const {
        estimates,
        loading,
        error,
        fetchMultipleEstimates,
        formatMemorySize,
        GPU_SCENARIOS,
        clearEstimates,
    } = useMemoryEstimation();

    // Debounce timer ref
    const debounceTimerRef = useRef<NodeJS.Timeout>();

    // Update slider when external context length changes
    useEffect(() => {
        setSliderValue(contextToLogScale(currentContextLength / 1000));
    }, [currentContextLength]);

    // Debounced fetch estimates when model, context length, or concurrency changes
    useEffect(() => {
        // Clear existing timer
        if (debounceTimerRef.current) {
            clearTimeout(debounceTimerRef.current);
        }

        // Set new timer
        debounceTimerRef.current = setTimeout(() => {
            if (modelId && currentContextLength > 0) {
                fetchMultipleEstimates(
                    modelId,
                    currentContextLength,
                    currentConcurrency,
                );
            } else {
                clearEstimates();
            }
        }, 300); // 300ms debounce

        // Cleanup timer on unmount
        return () => {
            if (debounceTimerRef.current) {
                clearTimeout(debounceTimerRef.current);
            }
        };
    }, [modelId, currentContextLength, currentConcurrency]);

    const handleSliderChange = useCallback(
        (_: Event, value: number | number[]) => {
            const logValue = Array.isArray(value) ? value[0] : value;
            const contextK = logScaleToContext(logValue);

            // Snap to preset if close enough, otherwise use the exact value
            const snapThreshold = contextK * 0.1; // 10% threshold for snapping
            const closestPreset = CONTEXT_PRESETS.reduce((closest, preset) => {
                const distance = Math.abs(contextK - preset);
                const closestDistance = Math.abs(contextK - closest);
                return distance < closestDistance ? preset : closest;
            });

            const shouldSnap =
                Math.abs(contextK - closestPreset) <= snapThreshold;
            const finalContextK = shouldSnap
                ? closestPreset
                : Math.round(contextK);

            setSliderValue(contextToLogScale(finalContextK));
            const contextLength = finalContextK * 1000; // Convert K back to tokens
            onContextLengthChange(contextLength);
        },
        [onContextLengthChange],
    );

    const handlePresetClick = useCallback(
        (preset: number) => {
            setSliderValue(contextToLogScale(preset));
            onContextLengthChange(preset * 1000);
        },
        [onContextLengthChange],
    );

    const handleManualRefresh = useCallback(() => {
        if (modelId && currentContextLength > 0) {
            fetchMultipleEstimates(
                modelId,
                currentContextLength,
                currentConcurrency,
            );
        }
    }, [
        modelId,
        currentContextLength,
        currentConcurrency,
        fetchMultipleEstimates,
    ]);

    const renderEstimateCard = (scenario: {
        name: string;
        gpuCount: number;
    }) => {
        const estimate = estimates[scenario.gpuCount];

        return (
            <Grid item xs={6} sm={4} md={3} key={scenario.gpuCount}>
                <Card
                    variant="outlined"
                    sx={{ height: "100%", minHeight: "140px" }}
                >
                    <CardContent sx={{ p: 1.5 }}>
                        <Typography
                            variant="body2"
                            gutterBottom
                            sx={{ fontSize: "0.875rem", fontWeight: 600 }}
                            color="primary"
                        >
                            {scenario.name}
                        </Typography>

                        {loading ? (
                            <Box display="flex" justifyContent="center" py={2}>
                                <CircularProgress size={20} />
                            </Box>
                        ) : estimate ? (
                            <Box>
                                {scenario.gpuCount === 1 ? (
                                    <Typography
                                        variant="body1"
                                        color="text.primary"
                                        gutterBottom
                                        sx={{
                                            fontSize: "1rem",
                                            fontWeight: 500,
                                        }}
                                    >
                                        {formatMemorySize(estimate.total_size)}
                                    </Typography>
                                ) : (
                                    <Box>
                                        <Typography
                                            variant="body1"
                                            color="text.primary"
                                            gutterBottom
                                            sx={{
                                                fontSize: "1rem",
                                                fontWeight: 500,
                                            }}
                                        >
                                            {formatMemorySize(
                                                (estimate.total_size || 0) /
                                                    scenario.gpuCount,
                                            )}{" "}
                                            per GPU
                                        </Typography>
                                        <Typography
                                            variant="body2"
                                            color="text.secondary"
                                            gutterBottom
                                        >
                                            (
                                            {formatMemorySize(
                                                estimate.total_size,
                                            )}{" "}
                                            total)
                                        </Typography>
                                    </Box>
                                )}

                                <Box sx={{ mt: 0.5 }}>
                                    <Typography
                                        variant="caption"
                                        display="block"
                                        color="text.secondary"
                                        sx={{ lineHeight: 1.2, mb: 0.25 }}
                                    >
                                        VRAM:{" "}
                                        {formatMemorySize(estimate.vram_size)}
                                    </Typography>
                                    <Typography
                                        variant="caption"
                                        display="block"
                                        color="text.secondary"
                                        sx={{ lineHeight: 1.2, mb: 0.25 }}
                                    >
                                        Weights:{" "}
                                        {formatMemorySize(estimate.weights)}
                                    </Typography>
                                    <Typography
                                        variant="caption"
                                        display="block"
                                        color="text.secondary"
                                        sx={{ lineHeight: 1.2, mb: 0.25 }}
                                    >
                                        KV Cache:{" "}
                                        {formatMemorySize(estimate.kv_cache)}
                                    </Typography>
                                    {estimate.requires_fallback && (
                                        <Chip
                                            label="CPU Fallback"
                                            size="small"
                                            color="info"
                                            sx={{
                                                mt: 0.25,
                                                fontSize: "0.6rem",
                                                height: 16,
                                            }}
                                        />
                                    )}
                                </Box>
                            </Box>
                        ) : (
                            <Typography variant="body2" color="text.secondary">
                                No estimate available
                            </Typography>
                        )}
                    </CardContent>
                </Card>
            </Grid>
        );
    };

    if (!modelId) {
        return null;
    }

    return (
        <Card variant="outlined" sx={{ mt: 2 }}>
            <CardContent>
                <Box
                    display="flex"
                    alignItems="center"
                    justifyContent="space-between"
                    mb={2}
                >
                    <Typography variant="h6">Memory Estimation</Typography>
                    <Box display="flex" alignItems="center" gap={1}>
                        <Tooltip title="Refresh estimates">
                            <IconButton
                                size="small"
                                onClick={handleManualRefresh}
                                disabled={disabled || loading}
                            >
                                <RefreshIcon />
                            </IconButton>
                        </Tooltip>
                    </Box>
                </Box>

                {error && (
                    <Alert severity="warning" sx={{ mb: 2 }}>
                        {error}
                    </Alert>
                )}

                {/* Context Length Slider */}
                <Box mb={3}>
                    <Typography variant="subtitle2" gutterBottom>
                        Context Length:{" "}
                        {(
                            logScaleToContext(sliderValue) * 1000
                        ).toLocaleString()}{" "}
                        tokens ({Math.round(logScaleToContext(sliderValue))}K)
                    </Typography>

                    <Box px={1}>
                        <Slider
                            value={sliderValue}
                            onChange={handleSliderChange}
                            min={contextToLogScale(4)}
                            max={contextToLogScale(1000)}
                            step={0.01}
                            marks={CONTEXT_PRESETS.map((value) => ({
                                value: contextToLogScale(value),
                                label: `${value}K`,
                            }))}
                            valueLabelDisplay="auto"
                            valueLabelFormat={(logValue) =>
                                `${Math.round(logScaleToContext(logValue))}K`
                            }
                            disabled={disabled}
                            sx={{
                                mb: 3,
                                "& .MuiSlider-markLabel": {
                                    fontSize: "0.7rem",
                                    transform: "translateX(-50%)",
                                    whiteSpace: "nowrap",
                                },
                                "& .MuiSlider-mark": {
                                    height: 8,
                                    backgroundColor: "primary.main",
                                },
                                "& .MuiSlider-rail": {
                                    opacity: 0.3,
                                },
                                "& .MuiSlider-track": {
                                    border: "none",
                                },
                            }}
                        />
                    </Box>

                    {/* Preset Buttons */}
                    <Box
                        display="flex"
                        flexWrap="wrap"
                        justifyContent="center"
                        gap={0.5}
                        sx={{ maxWidth: "1000px", mx: "auto" }}
                    >
                        {CONTEXT_PRESETS.map((preset) => (
                            <Chip
                                key={preset}
                                label={`${preset}K`}
                                size="small"
                                clickable
                                disabled={disabled}
                                color={
                                    Math.abs(
                                        sliderValue - contextToLogScale(preset),
                                    ) < 0.01
                                        ? "primary"
                                        : "default"
                                }
                                onClick={() => handlePresetClick(preset)}
                                sx={{
                                    minWidth: "60px",
                                    fontSize: "0.75rem",
                                    "& .MuiChip-label": {
                                        whiteSpace: "nowrap",
                                        overflow: "visible",
                                    },
                                }}
                            />
                        ))}
                    </Box>
                </Box>

                {/* Concurrency Control */}
                <Box mb={3}>
                    <Typography variant="subtitle2" gutterBottom>
                        Concurrent Requests:{" "}
                        <Box
                            component="span"
                            sx={{
                                fontWeight: "bold",
                                color: "primary.main",
                            }}
                        >
                            {currentConcurrency === 0
                                ? "Default (2)"
                                : currentConcurrency}
                        </Box>
                    </Typography>
                    <Box px={1}>
                        <Slider
                            value={currentConcurrency}
                            onChange={(_, value) => {
                                const newValue = Array.isArray(value)
                                    ? value[0]
                                    : value;
                                onConcurrencyChange(newValue);
                            }}
                            min={0}
                            max={8}
                            step={1}
                            marks={[
                                { value: 0, label: "Default" },
                                { value: 1, label: "1" },
                                { value: 2, label: "2" },
                                { value: 4, label: "4" },
                                { value: 8, label: "8" },
                            ]}
                            valueLabelDisplay="auto"
                            disabled={disabled}
                            sx={{ mb: 2 }}
                        />
                    </Box>
                    <Alert severity="info" sx={{ mb: 2 }}>
                        <Typography variant="body2">
                            <strong>Memory Impact:</strong> Ollama allocates{" "}
                            <em>concurrency × context length</em> total memory
                            for the KV cache. Higher concurrency improves
                            throughput but increases memory usage.
                        </Typography>
                    </Alert>
                </Box>

                {/* Memory Estimates */}
                <Box>
                    <Typography variant="subtitle2" gutterBottom sx={{ mb: 2 }}>
                        GPU Memory Requirements
                    </Typography>

                    <Grid container spacing={1.5}>
                        {GPU_SCENARIOS.map(renderEstimateCard)}
                    </Grid>
                </Box>
            </CardContent>
        </Card>
    );
};

export default MemoryEstimationWidget;
