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
  Link,
} from '@mui/material';
// Use actual types from the API definition
import { TypesModel, TypesModelType, TypesRuntime } from '../../api/api';
import { useCreateHelixModel, useUpdateHelixModel } from '../../services/helixModelsService';

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
  const { mutateAsync: createModel, isPending: isCreating } = useCreateHelixModel();
  const { mutateAsync: updateModel, isPending: isUpdating } = useUpdateHelixModel();
  const loading = isCreating || isUpdating;

  const [error, setError] = useState<string>('');
  // Store memory in GB in the form state
  const [formData, setFormData] = useState({
    id: '',
    name: '',
    description: '',
    type: TypesModelType.ModelTypeChat as TypesModelType,
    runtime: TypesRuntime.RuntimeOllama as TypesRuntime,
    memory: 0, // Store memory in GB
    context_length: 0, // Initialize context length
    enabled: true,
    hide: false,
    auto_pull: false, // Add auto_pull state
  });

  // Initialize form data when the dialog opens or the model changes
  useEffect(() => {
    if (open) {
      if (isEditing && model) {
        setFormData({
          id: model.id || '',
          name: model.name || '',
          description: model.description || '',
          type: model.type || TypesModelType.ModelTypeChat,
          runtime: model.runtime || TypesRuntime.RuntimeOllama,
          memory: model.memory ? model.memory / (1024 * 1024 * 1024) : 0, // Convert bytes to GB
          context_length: model.context_length || 0,
          enabled: model.enabled !== undefined ? model.enabled : false, // Default to false if undefined
          hide: model.hide || false,
          auto_pull: model.auto_pull || false, // Initialize auto_pull
        });
      } else {
        // Reset for creating a new model
        setFormData({
          id: '',
          name: '',
          description: '',
          type: TypesModelType.ModelTypeChat,
          runtime: TypesRuntime.RuntimeOllama,
          memory: 0, // Default GB
          context_length: 0,
          enabled: true,
          hide: false,
          auto_pull: false, // Default auto_pull to false
        });
      }
      setError(''); // Clear errors when dialog opens/changes mode
    }
  }, [open, model, isEditing]);

  const handleTextFieldChange = (e: React.ChangeEvent<HTMLInputElement | HTMLTextAreaElement>) => {
    const { name, value } = e.target;
    // Handle number inputs specifically
    if (name === 'memory') {
        // Parse as float for GB
        const floatValue = value === '' ? 0 : parseFloat(value);
        if (!isNaN(floatValue) && floatValue >= 0) {
            setFormData((prev) => ({ ...prev, [name]: floatValue }));
        } else if (value === '') {
            // Allow clearing the field, treating it as 0 for state
            setFormData((prev) => ({ ...prev, [name]: 0 }));
        }
    } else if (name === 'context_length') {
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
      [name]: value as TypesModelType | TypesRuntime, // Cast based on the field name
    }));
    setError('');
  };

  const handleSwitchChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const { name, checked } = e.target;
    setFormData((prev) => ({ ...prev, [name]: checked }));
    setError('');
  };

  const validateForm = useCallback(() => {
    // Only validate ID when creating a new model
    if (!isEditing && !formData.id.trim()) {
        setError('Model ID is required');
        return false;
    }
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
        setError('Memory (in GB) is required and must be a positive number.');
        return false;
    }
    // Context length is optional, but should be non-negative if provided
    if (formData.context_length < 0) {
        setError('Context Length must be a non-negative number.');
        return false;
    }

    return true;
  }, [formData, isEditing]);

  const handleSubmit = async () => {
    if (!validateForm()) return;

    setError(''); // Clear previous errors

    // Construct the base payload
    const payloadBase: Partial<TypesModel> = {
      // ID is handled differently for create vs update below
      name: formData.name.trim(),
      description: formData.description.trim() || undefined,
      type: formData.type,
      runtime: formData.runtime,
      memory: Math.round(formData.memory * 1024 * 1024 * 1024), // Convert GB to Bytes
      context_length: formData.context_length > 0 ? formData.context_length : undefined,
      enabled: formData.enabled,
      auto_pull: formData.auto_pull,
      hide: formData.hide,
    };

    try {
      if (isEditing && model) {
        // For update, use the existing model's ID (which is not editable)
        await updateModel({ id: model.id || '', helixModel: { ...payloadBase } });
      } else {
        // For create, include the user-provided ID from the form
        await createModel({ ...payloadBase, id: formData.id.trim() });
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
              disabled // ID is not editable when editing
              sx={{ mb: 2 }} // Add margin if needed
             />
          )}

          {!isEditing && (
            <TextField
              name="id"
              label="Model ID"
              value={formData.id}
              onChange={handleTextFieldChange}
              fullWidth
              required
              autoComplete="off"
              placeholder="e.g., llama3:70b-instruct-q4_0 or custom-model-name"
              helperText={<>Unique identifier for the model (may be provider-specific). Find models at <Link href="https://ollama.com/search" target="_blank" rel="noopener noreferrer">https://ollama.com/search</Link></>}
              disabled={loading}
            />
          )}

          <TextField
            name="name"
            label="Model Name"
            value={formData.name}
            onChange={handleTextFieldChange}
            fullWidth
            required
            autoComplete="off"
            placeholder="e.g., Llama 3 70B Instruct Q4"
            helperText="User-friendly name shown in model selection lists."
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
              <MenuItem value={TypesRuntime.RuntimeOllama}>Ollama</MenuItem>
              <MenuItem value={TypesRuntime.RuntimeVLLM} disabled>vLLM (coming soon)</MenuItem>
              <MenuItem value={TypesRuntime.RuntimeDiffusers} disabled>Diffusers (Image) (coming soon)</MenuItem>
              {/* Add more runtime options if they become available in the enum */}
            </Select>
          </FormControl>

           <TextField
                name="memory"
                label="Memory Required (GB)"
                type="number" // Allows decimals
                value={formData.memory === 0 ? '' : formData.memory} // Show empty if 0
                onChange={handleTextFieldChange}
                fullWidth
                required
                autoComplete="off"
                placeholder="e.g., 16 for 16GB"
                helperText="Estimated memory needed to run this model (in GB)."
                disabled={loading}
                InputProps={{
                    inputProps: {
                        min: 0, // Allow 0 input, validation ensures > 0 on submit
                        step: "any" // Allow floating point numbers
                    }
                }}
            />

           {/* Conditionally render Context Length field */}
           {formData.type === TypesModelType.ModelTypeChat && (
             <TextField
                  name="context_length"
                  label="Context Length"
                  type="number"
                  value={formData.context_length === 0 ? '' : formData.context_length} // Show empty if 0
                  onChange={handleTextFieldChange}
                  fullWidth
                  autoComplete="off"
                  placeholder="e.g., 128000"
                  required
                  helperText="Maximum context window size (in tokens)"
                  disabled={loading}
                  InputProps={{
                      inputProps: {
                          min: 0
                      }
                  }}
              />
           )}
            
           <Stack direction="row" spacing={2} justifyContent="start" alignItems="center">
                <FormControlLabel
                    control={<Switch checked={formData.enabled} onChange={handleSwitchChange} name="enabled" disabled={loading} />}
                    label="Enabled"
                 />
                 <FormHelperText>Allow users to select and use this model.</FormHelperText>
            </Stack>
             
            <Stack direction="row" spacing={2} justifyContent="start" alignItems="center">
                <FormControlLabel
                    control={<Switch checked={formData.hide} onChange={handleSwitchChange} name="hide" disabled={loading} />}
                    label="Hide"
                 />
                  <FormHelperText>Hide this model from the default selection lists (still usable if selected directly).</FormHelperText>
           </Stack>

           <Stack direction="row" spacing={2} justifyContent="start" alignItems="center">
               <FormControlLabel                  
                   control={<Switch checked={formData.auto_pull} onChange={handleSwitchChange} name="auto_pull" disabled={loading} />}
                   label="Auto Pull"
                />
                 <FormHelperText>Automatically pull the latest version of this model if available (Ollama only).</FormHelperText>
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
