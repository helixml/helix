import React, { useState, useCallback, useEffect } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  TextField,
  FormControl,
  InputLabel,
  Select,
  MenuItem,
  Alert,
  Stack,
  SelectChangeEvent,
  CircularProgress,
  Switch,
  FormControlLabel,
  FormHelperText,
} from '@mui/material';
// Use actual types from the API definition
import { TypesModel, TypesModelType, TypesModelRuntimeType } from '../../api/api';
// import { useCreateModel, useUpdateModel } from '../../services/helixModelService'; // Placeholder hooks

interface EditHelixModelDialogProps {
  open: boolean;
  model: TypesModel | null; // null for creating a new model
  onClose: () => void;
  refreshData: () => void; // To refresh the list after save
}

const EditHelixModelDialog: React.FC<EditHelixModelDialogProps> = ({
  open,
  model,
  onClose,
  refreshData,
}) => {
  const isEditing = !!model;
  // Placeholder hooks - replace with your actual implementation
  // const { mutateAsync: createModel, isPending: isCreating } = useCreateModel();
  // const { mutateAsync: updateModel, isPending: isUpdating } = useUpdateModel(model?.id || '');
  const isCreating = false; // Placeholder
  const isUpdating = false; // Placeholder
  const loading = isCreating || isUpdating;

  const [error, setError] = useState<string>('');
  const [formData, setFormData] = useState({
    name: '',
    description: '',
    type: TypesModelType.ModelTypeChat as TypesModelType,
    runtime: TypesModelRuntimeType.ModelRuntimeTypeOllama as TypesModelRuntimeType,
    memory: 0, // Initialize memory, might need better default or handling
    context_length: 0, // Initialize context length
    enabled: true,
    hide: false,
  });

  // Initialize form data when the dialog opens or the model changes
  useEffect(() => {
    if (open) {
      if (isEditing && model) {
        setFormData({
          name: model.name || '',
          description: model.description || '',
          type: model.type || TypesModelType.ModelTypeChat,
          runtime: model.runtime || TypesModelRuntimeType.ModelRuntimeTypeOllama,
          memory: model.memory || 0,
          context_length: model.context_length || 0,
          enabled: model.enabled !== undefined ? model.enabled : true, // Default to true if undefined
          hide: model.hide || false,
        });
      } else {
        // Reset for creating a new model
        setFormData({
          name: '',
          description: '',
          type: TypesModelType.ModelTypeChat,
          runtime: TypesModelRuntimeType.ModelRuntimeTypeOllama,
          memory: 0,
          context_length: 0,
          enabled: true,
          hide: false,
        });
      }
      setError(''); // Clear errors when dialog opens/changes mode
    }
  }, [open, model, isEditing]);

  const handleTextFieldChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const { name, value } = e.target;
    // Handle number inputs specifically
    if (name === 'memory' || name === 'context_length') {
      const numValue = value === '' ? 0 : parseInt(value, 10);
      if (!isNaN(numValue) && numValue >= 0) {
          setFormData((prev) => ({ ...prev, [name]: numValue }));
      } else if (value === '') {
          // Allow clearing the field, treating it as 0 for state
          setFormData((prev) => ({ ...prev, [name]: 0 }));
      }
    } else {
      setFormData((prev) => ({ ...prev, [name]: value }));
    }
    setError('');
  };
  
  const handleSelectChange = (e: SelectChangeEvent<string>) => {
    const { name, value } = e.target;
    setFormData((prev) => ({
      ...prev,
      [name]: value as TypesModelType | TypesModelRuntimeType, // Cast based on the field name
    }));
    setError('');
  };

  const handleSwitchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, checked } = e.target;
    setFormData((prev) => ({ ...prev, [name]: checked }));
    setError('');
  };

  const validateForm = useCallback(() => {
    if (!formData.name.trim()) {
      setError('Model Name is required');
      return false;
    }
    if (!formData.type) {
      setError('Model Type is required');
      return false;
    }
    if (!formData.runtime) {
      setError('Runtime is required');
      return false;
    }
    if (formData.memory <= 0) {
        setError('Memory (in bytes) is required and must be a positive number.');
        return false;
    }
    // Context length is optional, but should be non-negative if provided
    if (formData.context_length < 0) {
        setError('Context Length must be a non-negative number.');
        return false;
    }

    return true;
  }, [formData]);

  const handleSubmit = async () => {
    if (!validateForm()) return;

    setError(''); // Clear previous errors

    const payload = {
      name: formData.name.trim(),
      description: formData.description.trim() || undefined,
      type: formData.type,
      runtime: formData.runtime,
      memory: formData.memory,
      context_length: formData.context_length > 0 ? formData.context_length : undefined,
      enabled: formData.enabled,
      hide: formData.hide,
    };

    try {
      if (isEditing && model) {
        // await updateModel({ ...payload, id: model.id }); // Include ID for update
        console.log("Update Payload:", { ...payload, id: model.id }); // Placeholder for API call
      } else {
        // await createModel(payload);
         console.log("Create Payload:", payload); // Placeholder for API call
      }
      refreshData(); // Refresh the data in the parent component
      onClose(); // Close the dialog on success
    } catch (err) {
      console.error("Failed to save model:", err);
      setError(err instanceof Error ? err.message : 'Failed to save model');
    }
  };

  const handleClose = () => {
    // No need to reset form here if useEffect handles it based on `open`
    onClose();
  };

  return (
    <Dialog open={open} onClose={handleClose} maxWidth="md" fullWidth>
      <DialogTitle>{isEditing ? `Edit Helix Model: ${model?.name}` : 'Register New Helix Model'}</DialogTitle>
      <DialogContent>
        <Stack spacing={3} sx={{ mt: 2 }}>
          {error && <Alert severity="error">{error}</Alert>}

          {isEditing && model && (
             <TextField
              label="Model ID"
              value={model.id}
              fullWidth
              disabled // ID is usually not editable, it's assigned by the system/provider
              sx={{ mb: 2 }} // Add margin if needed
             />
          )}

          <TextField
            name="name"
            label="Model Name / ID"
            value={formData.name}
            onChange={handleTextFieldChange}
            fullWidth
            required
            autoComplete="off"
            placeholder="e.g., llama3:70b-instruct-q4_0 or custom-model-name"
            helperText="Identifier for the model (may be provider-specific for Ollama, etc.)"
            disabled={loading}
          />

          <TextField
            name="description"
            label="Description"
            value={formData.description}
            onChange={handleTextFieldChange}
            fullWidth
            multiline
            rows={2}
            autoComplete="off"
            placeholder="Optional: A brief description of the model or its purpose."
            disabled={loading}
          />

          <FormControl fullWidth required disabled={loading}>
            <InputLabel id="model-type-label">Model Type</InputLabel>
            <Select
              labelId="model-type-label"
              name="type" // Changed from model_type
              value={formData.type}
              onChange={handleSelectChange}
              label="Model Type"
            >
              <MenuItem value={TypesModelType.ModelTypeChat}>Chat</MenuItem>
              <MenuItem value={TypesModelType.ModelTypeImage}>Image</MenuItem>
              <MenuItem value={TypesModelType.ModelTypeEmbed}>Embed</MenuItem>
            </Select>
          </FormControl>

          <FormControl fullWidth required disabled={loading}>
            <InputLabel id="runtime-label">Runtime</InputLabel>
            <Select
              labelId="runtime-label"
              name="runtime"
              value={formData.runtime}
              onChange={handleSelectChange}
              label="Runtime"
            >
              {/* Use enum values */}
              <MenuItem value={TypesModelRuntimeType.ModelRuntimeTypeOllama}>Ollama</MenuItem>
              <MenuItem value={TypesModelRuntimeType.ModelRuntimeTypeVLLM}>vLLM</MenuItem>
              <MenuItem value={TypesModelRuntimeType.ModelRuntimeTypeDiffusers}>Diffusers (Image)</MenuItem>
              {/* Add more runtime options if they become available in the enum */}
            </Select>
          </FormControl>

           <TextField
                name="memory"
                label="Memory Required (Bytes)"
                type="number"
                value={formData.memory === 0 ? '' : formData.memory} // Show empty if 0
                onChange={handleTextFieldChange}
                fullWidth
                required
                autoComplete="off"
                placeholder="e.g., 16000000000 for 16GB"
                helperText="Estimated memory needed to run this model (in bytes)."
                disabled={loading}
                InputProps={{
                    inputProps: { 
                        min: 1 // Ensure positive value visually, though validation handles 0
                    }
                }}
            />

           <TextField
                name="context_length"
                label="Context Length (Optional)"
                type="number"
                value={formData.context_length === 0 ? '' : formData.context_length} // Show empty if 0
                onChange={handleTextFieldChange}
                fullWidth
                autoComplete="off"
                placeholder="e.g., 4096"
                helperText="Maximum context window size (in tokens). Leave blank if unknown."
                disabled={loading}
                InputProps={{
                    inputProps: { 
                        min: 0
                    }
                }}
            />
            
           <Stack direction="row" spacing={2} justifyContent="start">
                <FormControlLabel
                    control={<Switch checked={formData.enabled} onChange={handleSwitchChange} name="enabled" disabled={loading} />}
                    label="Enabled"
                 />
                 <FormHelperText>Allow users to select and use this model.</FormHelperText>
            </Stack>
             
            <Stack direction="row" spacing={2} justifyContent="start">
                <FormControlLabel
                    control={<Switch checked={formData.hide} onChange={handleSwitchChange} name="hide" disabled={loading} />}
                    label="Hide"
                 />
                  <FormHelperText>Hide this model from the default selection lists (still usable if selected directly).</FormHelperText>
           </Stack>

        </Stack>
      </DialogContent>
      <DialogActions>
        <Button onClick={handleClose} disabled={loading}>Cancel</Button>
        <Button
          onClick={handleSubmit}
          variant="contained"
          color="primary"
          disabled={loading}
          startIcon={loading ? <CircularProgress size={20} /> : null}
        >
          {loading ? (isEditing ? 'Saving...' : 'Creating...') : (isEditing ? 'Save Changes' : 'Register Model')}
        </Button>
      </DialogActions>
    </Dialog>
  );
};

export default EditHelixModelDialog;
