import React, { FC, useState, useEffect } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Typography,
  Alert,
  CircularProgress,
} from '@mui/material';
import { TypesDynamicModelInfo, TypesModelInfo, TypesPricing } from '../../api/api';
import { useCreateModelInfo, useUpdateModelInfo } from '../../services/modelInfoService';

interface ModelPricingDialogProps {
  open: boolean;
  model: TypesDynamicModelInfo | undefined;
  onClose: () => void;
  refreshData: () => void;
}

const ModelPricingDialog: FC<ModelPricingDialogProps> = ({
  open,
  model,
  onClose,
  refreshData,
}) => {
  const [formData, setFormData] = useState({
    provider: '',
    name: '',
    prompt: '',
    completion: '',
  });
  const [error, setError] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);

  const createModelInfo = useCreateModelInfo();
  const updateModelInfo = useUpdateModelInfo();

  const isEditing = Boolean(model?.id);

  // Helper function to convert per-million token price to display format
  const convertToDisplayFormat = (price: string): string => {
    if (!price.trim()) return '';
    const numPrice = parseFloat(price);
    if (isNaN(numPrice)) return price; // Return original if not a valid number
    return `$${numPrice.toFixed(2)} per 1M tokens`;
  };

  // Helper function to convert per-token price back to per-million token price for display
  const convertToPerMillionDisplay = (perTokenPrice: string): string => {
    if (!perTokenPrice.trim()) return '';
    const numPrice = parseFloat(perTokenPrice);
    if (isNaN(numPrice)) return perTokenPrice; // Return original if not a valid number
    // Convert per-token price to per-million token price for display
    const perMillionPrice = numPrice * 1000000;
    return perMillionPrice.toFixed(2);
  };

  // Helper function to convert per-million token price to per-token price with proper decimal formatting
  const convertToPerTokenPrice = (perMillionPrice: number): string => {
    const perTokenPrice = perMillionPrice / 1000000;
    // Use toFixed with enough precision to avoid scientific notation
    // For prices like 0.4 per million, we want 0.0000004, not 4e-7
    return perTokenPrice.toFixed(7);
  };

  // Reset form when dialog opens/closes or model changes
  useEffect(() => {
    if (open) {
      if (model) {
        // Edit mode - populate with existing data
        setFormData({
          provider: model.provider || '',
          name: model.name || '',
          prompt: convertToPerMillionDisplay(model.model_info?.pricing?.prompt || ''),
          completion: convertToPerMillionDisplay(model.model_info?.pricing?.completion || ''),
        });
      } else {
        // Create mode - reset form
        setFormData({
          provider: '',
          name: '',
          prompt: '',
          completion: '',
        });
      }
      setError('');
    }
  }, [open, model]);

  const validateForm = () => {
    if (!formData.provider.trim()) {
      setError('Provider name is required');
      return false;
    }
    if (!formData.name.trim()) {
      setError('Model name is required');
      return false;
    }
    if (!formData.prompt.trim() && !formData.completion.trim()) {
      setError('At least one pricing field (prompt or completion) is required');
      return false;
    }
    
    // Validate that pricing values are valid numbers
    if (formData.prompt.trim() && isNaN(parseFloat(formData.prompt))) {
      setError('Prompt price must be a valid number (e.g., 1.60)');
      return false;
    }
    if (formData.completion.trim() && isNaN(parseFloat(formData.completion))) {
      setError('Completion price must be a valid number (e.g., 3.20)');
      return false;
    }
    
    return true;
  };

  const handleSubmit = async () => {
    if (!validateForm()) return;

    setError('');
    setIsSubmitting(true);

    try {
      const pricing: TypesPricing = {};
      if (formData.prompt.trim()) {
        // Convert per-million token price to per-token price
        const promptPricePerMillion = parseFloat(formData.prompt.trim());
        pricing.prompt = convertToPerTokenPrice(promptPricePerMillion);
      }
      if (formData.completion.trim()) {
        // Convert per-million token price to per-token price
        const completionPricePerMillion = parseFloat(formData.completion.trim());
        pricing.completion = convertToPerTokenPrice(completionPricePerMillion);
      }

      const modelInfo: TypesModelInfo = {
        name: formData.name.trim(),
        pricing,
      };

      const dynamicModelInfo: TypesDynamicModelInfo = {
        provider: formData.provider.trim(),
        name: formData.name.trim(),
        model_info: modelInfo,
      };

      if (isEditing && model?.id) {
        // Update existing model info
        await updateModelInfo.mutateAsync({
          id: model.id,
          modelInfo: {
            ...model,
            ...dynamicModelInfo,
          },
        });
      } else {
        // Create new model info - ID should be <provider>/<model>
        const id = `${formData.provider.trim()}/${formData.name.trim()}`;
        await createModelInfo.mutateAsync({
          ...dynamicModelInfo,
          id,
        });
      }

      // React Query will automatically invalidate and refetch the data
      // But we also call refreshData to ensure immediate UI update
      refreshData();
      onClose();
    } catch (err) {
      console.error('Failed to save model pricing:', err);
      setError(err instanceof Error ? err.message : 'Failed to save model pricing');
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleClose = () => {
    onClose();
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="sm" fullWidth>
      <DialogTitle>
        {isEditing ? 'Update Model Pricing' : 'Create Model Pricing'}
      </DialogTitle>
      <DialogContent>
        <Box sx={{ mt: 2 }}>
          {error && (
            <Alert severity="error" sx={{ mb: 2 }}>
              {error}
            </Alert>
          )}
          
          <TextField
            fullWidth
            label="Provider"
            value={formData.provider}
            onChange={(e) => setFormData({ ...formData, provider: e.target.value })}
            disabled={isEditing} // Cannot change provider in edit mode
            margin="normal"
            required
            helperText={isEditing ? "Provider cannot be changed in edit mode" : "e.g., helix, openai, anthropic"}
          />
          
          <TextField
            fullWidth
            label="Model Name"
            value={formData.name}
            onChange={(e) => setFormData({ ...formData, name: e.target.value })}
            disabled={isEditing} // Cannot change model name in edit mode
            margin="normal"
            required
            helperText={isEditing ? "Model name cannot be changed in edit mode" : "e.g., gpt-4, claude-3-sonnet"}
          />
          
          <Typography variant="h6" sx={{ mt: 3, mb: 2 }}>
            Pricing
          </Typography>
          
          <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
            Enter prices per 1 million tokens. The system will automatically convert these to per-token prices for storage.
          </Typography>
          
          <TextField
            fullWidth
            label="Prompt Price"
            value={formData.prompt}
            onChange={(e) => setFormData({ ...formData, prompt: e.target.value })}
            margin="normal"
            placeholder="e.g., 1.60"
            helperText="Price per 1M input/prompt tokens (e.g., 1.60 = $1.60 per 1M tokens). This will be stored as per-token price in the API."
            inputProps={{
              step: "0.01",
              min: "0"
            }}
          />
          
          <TextField
            fullWidth
            label="Completion Price"
            value={formData.completion}
            onChange={(e) => setFormData({ ...formData, completion: e.target.value })}
            margin="normal"
            placeholder="e.g., 3.20"
            helperText="Price per 1M output/completion tokens (e.g., 3.20 = $3.20 per 1M tokens). This will be stored as per-token price in the API."
            inputProps={{
              step: "0.01",
              min: "0"
            }}
          />
        </Box>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={isSubmitting}>
          Cancel
        </Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          disabled={isSubmitting}
          startIcon={isSubmitting ? <CircularProgress size={20} /> : null}
        >
          {isSubmitting ? 'Saving...' : (isEditing ? 'Update' : 'Create')}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default ModelPricingDialog;
