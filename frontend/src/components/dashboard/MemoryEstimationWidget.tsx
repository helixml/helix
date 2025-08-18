import React, { useState, useEffect, useCallback } from 'react';
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
  Collapse,
} from '@mui/material';
import {
  ExpandMore as ExpandMoreIcon,
  ExpandLess as ExpandLessIcon,
  Refresh as RefreshIcon,
} from '@mui/icons-material';
import { useMemoryEstimation } from '../../hooks/useMemoryEstimation';

interface MemoryEstimationWidgetProps {
  modelId: string;
  currentContextLength: number;
  onContextLengthChange: (value: number) => void;
  disabled?: boolean;
}

// Common context length values in K (thousands)
const CONTEXT_PRESETS = [4, 8, 16, 32, 40, 64, 100, 128, 200, 256, 512, 1000];

const MemoryEstimationWidget: React.FC<MemoryEstimationWidgetProps> = ({
  modelId,
  currentContextLength,
  onContextLengthChange,
  disabled = false,
}) => {
  const [sliderValue, setSliderValue] = useState(currentContextLength / 1000); // Convert to K
  const [expanded, setExpanded] = useState(false);
  const { estimates, loading, error, fetchMultipleEstimates, formatMemorySize, GPU_SCENARIOS, clearEstimates } = useMemoryEstimation();

  // Update slider when external context length changes
  useEffect(() => {
    setSliderValue(currentContextLength / 1000);
  }, [currentContextLength]);

  // Fetch estimates when model or context length changes
  useEffect(() => {
    if (modelId && currentContextLength > 0) {
      fetchMultipleEstimates(modelId, currentContextLength);
    } else {
      clearEstimates();
    }
  }, [modelId, currentContextLength, fetchMultipleEstimates, clearEstimates]);

  const handleSliderChange = useCallback((_: Event, value: number | number[]) => {
    const newValue = Array.isArray(value) ? value[0] : value;
    setSliderValue(newValue);
    const contextLength = newValue * 1000; // Convert K back to tokens
    onContextLengthChange(contextLength);
  }, [onContextLengthChange]);

  const handlePresetClick = useCallback((preset: number) => {
    setSliderValue(preset);
    onContextLengthChange(preset * 1000);
  }, [onContextLengthChange]);

  const handleManualRefresh = useCallback(() => {
    if (modelId && currentContextLength > 0) {
      fetchMultipleEstimates(modelId, currentContextLength);
    }
  }, [modelId, currentContextLength, fetchMultipleEstimates]);

  const renderEstimateCard = (scenario: { name: string; gpuCount: number }) => {
    const estimate = estimates[scenario.gpuCount];
    
    return (
      <Grid item xs={12} sm={6} md={4} key={scenario.gpuCount}>
        <Card variant="outlined" sx={{ height: '100%' }}>
          <CardContent sx={{ p: 2 }}>
            <Typography variant="subtitle2" gutterBottom color="primary">
              {scenario.name}
            </Typography>
            
            {loading ? (
              <Box display="flex" justifyContent="center" py={2}>
                <CircularProgress size={20} />
              </Box>
            ) : estimate ? (
              <Box>
                <Typography variant="h6" color="text.primary" gutterBottom>
                  {formatMemorySize(estimate.total_size)}
                </Typography>
                
                <Box sx={{ mt: 1 }}>
                  <Typography variant="caption" display="block" color="text.secondary">
                    VRAM: {formatMemorySize(estimate.vram_size)}
                  </Typography>
                  <Typography variant="caption" display="block" color="text.secondary">
                    Weights: {formatMemorySize(estimate.weights)}
                  </Typography>
                  <Typography variant="caption" display="block" color="text.secondary">
                    KV Cache: {formatMemorySize(estimate.kv_cache)}
                  </Typography>
                  {estimate.requires_fallback && (
                    <Chip 
                      label="CPU Fallback" 
                      size="small" 
                      color="info" 
                      sx={{ mt: 0.5, fontSize: '0.7rem' }}
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
        <Box display="flex" alignItems="center" justifyContent="space-between" mb={2}>
          <Typography variant="h6">
            Memory Estimation
          </Typography>
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
            <IconButton
              size="small"
              onClick={() => setExpanded(!expanded)}
              disabled={disabled}
            >
              {expanded ? <ExpandLessIcon /> : <ExpandMoreIcon />}
            </IconButton>
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
            Context Length: {(sliderValue * 1000).toLocaleString()} tokens ({sliderValue}K)
          </Typography>
          
          <Slider
            value={sliderValue}
            onChange={handleSliderChange}
            min={4}
            max={1000}
            step={1}
            marks={CONTEXT_PRESETS.map(value => ({ value, label: `${value}K` }))}
            valueLabelDisplay="auto"
            valueLabelFormat={(value) => `${value}K`}
            disabled={disabled}
            sx={{ mb: 2 }}
          />

          {/* Preset Buttons */}
          <Box display="flex" flexWrap="wrap" gap={0.5}>
            {CONTEXT_PRESETS.map(preset => (
              <Chip
                key={preset}
                label={`${preset}K`}
                size="small"
                clickable
                disabled={disabled}
                color={Math.abs(sliderValue - preset) < 0.1 ? 'primary' : 'default'}
                onClick={() => handlePresetClick(preset)}
              />
            ))}
          </Box>
        </Box>

        {/* Memory Estimates */}
        <Collapse in={expanded}>
          <Typography variant="subtitle2" gutterBottom>
            GPU Memory Requirements
          </Typography>
          
          <Grid container spacing={2}>
            {GPU_SCENARIOS.map(renderEstimateCard)}
          </Grid>
        </Collapse>

        {!expanded && Object.keys(estimates).length > 0 && (
          <Box>
            <Typography variant="body2" color="text.secondary">
              Single GPU: {estimates[1] ? formatMemorySize(estimates[1].total_size) : 'Calculating...'}
              {estimates[2] && ` • 2 GPUs: ${formatMemorySize(estimates[2].total_size)}`}
              {estimates[4] && ` • 4 GPUs: ${formatMemorySize(estimates[4].total_size)}`}
              {estimates[8] && ` • 8 GPUs: ${formatMemorySize(estimates[8].total_size)}`}
            </Typography>
          </Box>
        )}
      </CardContent>
    </Card>
  );
};

export default MemoryEstimationWidget;
