import React, { FC, useState } from "react";
import {
    TableCell,
    CircularProgress,
    Tooltip,
    Box,
    Typography,
    IconButton,
    Menu,
    MenuItem,
    Chip,
} from "@mui/material";
import MemoryIcon from "@mui/icons-material/Memory";
import InfoIcon from "@mui/icons-material/Info";
import { TypesModel, ControllerMemoryEstimationResponse } from "../../api/api";
import {
    useModelMemoryEstimation,
    formatMemorySize,
} from "../../services/memoryEstimationService";

interface MemoryEstimateCellProps {
    model: TypesModel;
}

const MemoryEstimateCell: FC<MemoryEstimateCellProps> = ({ model }) => {
    const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
    const [selectedGPUCount, setSelectedGPUCount] = useState<number>(1);

    // For vLLM models, show the admin-set memory directly instead of estimates
    const isVllmModel = model.runtime === "vllm";

    const {
        data: memoryEstimate,
        isLoading,
        error,
    } = useModelMemoryEstimation(model.id || "", selectedGPUCount, {
        enabled: !!model.id && !isVllmModel,
    });

    const handleMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
        setAnchorEl(event.currentTarget);
    };

    const handleMenuClose = () => {
        setAnchorEl(null);
    };

    const handleGPUCountSelect = (gpuCount: number) => {
        setSelectedGPUCount(gpuCount);
        handleMenuClose();
    };

    const getMemoryEstimateContent = () => {
        // For vLLM models, show the admin-set memory directly
        if (isVllmModel) {
            const adminMemory = model.memory || 0;
            return (
                <Box display="flex" alignItems="center" gap={1}>
                    <Typography variant="body2" fontWeight="medium">
                        {formatMemorySize(adminMemory)}
                    </Typography>
                    <Chip
                        label="Admin Set"
                        size="small"
                        color="primary"
                        variant="outlined"
                        sx={{ fontSize: "0.6rem", height: "16px" }}
                    />
                    <Tooltip
                        title={
                            <Box>
                                <Typography variant="body2" fontWeight="medium">
                                    vLLM Model Memory:
                                </Typography>
                                <Typography variant="body2">
                                    Admin configured:{" "}
                                    {formatMemorySize(adminMemory)}
                                </Typography>
                                <Typography variant="body2">
                                    Runtime: vLLM
                                </Typography>
                            </Box>
                        }
                    >
                        <IconButton size="small">
                            <InfoIcon fontSize="small" />
                        </IconButton>
                    </Tooltip>
                </Box>
            );
        }

        // For non-vLLM models, use the estimation service
        if (isLoading) {
            return (
                <Box display="flex" alignItems="center" gap={1}>
                    <CircularProgress size={16} />
                    <Typography variant="body2" color="text.secondary">
                        Loading...
                    </Typography>
                </Box>
            );
        }

        if (error || !memoryEstimate || memoryEstimate.error) {
            return (
                <Tooltip
                    title={
                        error?.message ||
                        memoryEstimate?.error ||
                        "Failed to estimate"
                    }
                >
                    <Typography variant="body2" color="text.secondary">
                        -
                    </Typography>
                </Tooltip>
            );
        }

        const estimate = memoryEstimate.estimate;        

        if (!estimate) {
            return (
                <Typography variant="body2" color="error">
                    No estimate data
                </Typography>
            );
        }

        return (
            <Box display="flex" alignItems="center" gap={1}>
                <Box>
                    {selectedGPUCount === 1 ? (
                        <Typography variant="body2" fontWeight="medium">
                            {formatMemorySize(estimate.total_size || 0)}
                        </Typography>
                    ) : (
                        <>
                            <Typography variant="body2" fontWeight="medium">
                                {formatMemorySize(
                                    (estimate.total_size || 0) /
                                        selectedGPUCount,
                                )}{" "}
                                per GPU
                            </Typography>
                            <Typography
                                variant="caption"
                                color="text.secondary"
                            >
                                ({formatMemorySize(estimate.total_size || 0)}{" "}
                                total)
                            </Typography>
                        </>
                    )}
                </Box>

                <Tooltip
                    title={
                        <Box>
                            <Typography variant="body2" fontWeight="medium">
                                Memory Breakdown ({selectedGPUCount} GPU
                                {selectedGPUCount > 1 ? "s" : ""}):
                            </Typography>
                            {selectedGPUCount > 1 && (
                                <Typography variant="body2">
                                    Per GPU Memory:{" "}
                                    {formatMemorySize(
                                        (estimate.total_size || 0) /
                                            selectedGPUCount,
                                    )}
                                </Typography>
                            )}
                            <Typography variant="body2">
                                Total GPU Memory:{" "}
                                {formatMemorySize(estimate.total_size || 0)}
                            </Typography>
                            <Typography variant="body2">
                                VRAM Allocated:{" "}
                                {formatMemorySize(estimate.vram_size || 0)}
                            </Typography>
                            <Typography variant="body2">
                                Weights:{" "}
                                {formatMemorySize(estimate.weights || 0)}
                            </Typography>
                            <Typography variant="body2">
                                KV Cache:{" "}
                                {formatMemorySize(estimate.kv_cache || 0)}
                            </Typography>
                            <Typography variant="body2">
                                Graph:{" "}
                                {formatMemorySize(
                                    estimate.graph || estimate.graph_mem || 0,
                                )}
                            </Typography>
                            <Typography variant="body2">
                                Architecture:{" "}
                                {estimate.architecture || "Unknown"}
                            </Typography>
                            {estimate.requires_fallback && (
                                <Typography variant="body2" color="info.main">
                                    Supports CPU fallback
                                </Typography>
                            )}
                        </Box>
                    }
                >
                    <IconButton size="small">
                        <InfoIcon fontSize="small" />
                    </IconButton>
                </Tooltip>
            </Box>
        );
    };

    return (
        <TableCell>
            <Box display="flex" alignItems="center" gap={1}>
                {!isVllmModel && (
                    <IconButton
                        size="small"
                        onClick={handleMenuOpen}
                        color={selectedGPUCount > 1 ? "primary" : "default"}
                    >
                        <MemoryIcon fontSize="small" />
                        <Typography variant="caption" ml={0.5}>
                            {selectedGPUCount}GPU
                            {selectedGPUCount > 1 ? "s" : ""}
                        </Typography>
                    </IconButton>
                )}

                {getMemoryEstimateContent()}
            </Box>

            {!isVllmModel && (
                <Menu
                    anchorEl={anchorEl}
                    open={Boolean(anchorEl)}
                    onClose={handleMenuClose}
                >
                    <MenuItem onClick={() => handleGPUCountSelect(1)}>
                        <Box display="flex" alignItems="center" gap={1}>
                            <MemoryIcon fontSize="small" />
                            <Typography>1 GPU</Typography>
                        </Box>
                    </MenuItem>
                    <MenuItem onClick={() => handleGPUCountSelect(2)}>
                        <Box display="flex" alignItems="center" gap={1}>
                            <MemoryIcon fontSize="small" />
                            <Typography>2 GPUs</Typography>
                        </Box>
                    </MenuItem>
                    <MenuItem onClick={() => handleGPUCountSelect(4)}>
                        <Box display="flex" alignItems="center" gap={1}>
                            <MemoryIcon fontSize="small" />
                            <Typography>4 GPUs</Typography>
                        </Box>
                    </MenuItem>
                    <MenuItem onClick={() => handleGPUCountSelect(8)}>
                        <Box display="flex" alignItems="center" gap={1}>
                            <MemoryIcon fontSize="small" />
                            <Typography>8 GPUs</Typography>
                        </Box>
                    </MenuItem>
                </Menu>
            )}
        </TableCell>
    );
};

export default MemoryEstimateCell;
