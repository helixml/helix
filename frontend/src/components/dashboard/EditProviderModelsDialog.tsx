import React, { FC, useState, useEffect, useCallback } from 'react';
import {
  Dialog,
  DialogTitle,
  DialogContent,
  DialogActions,
  Button,
  List,
  ListItem,
  ListItemText,
  Switch,
  Typography,
  CircularProgress,
  Box,
  Alert,
} from '@mui/material';
import { IProviderEndpoint } from '../../types';
import { useApi } from '../../hooks/useApi';
import { useUpdateProviderEndpoint } from '../../services/providersService';
import { TypesProviderEndpointType } from '../../api/api';

interface EditProviderModelsDialogProps {
  open: boolean;
  endpoint: IProviderEndpoint | null;
  onClose: () => void;
  refreshData: () => void;
}

const EditProviderModelsDialog: FC<EditProviderModelsDialogProps> = ({ open, endpoint, onClose, refreshData }) => {
  const [selectedModels, setSelectedModels] = useState<string[]>([]);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const api = useApi();
  const apiClient = api.getApiClient();
  const { mutate: updateProviderEndpoint } = useUpdateProviderEndpoint(endpoint?.id || '');

  useEffect(() => {
    if (endpoint?.models) {
      setSelectedModels(endpoint.models);
    } else {
      setSelectedModels([]);
    }
    setError(null);
    setIsLoading(false);
  }, [endpoint]);

  const saveModels = useCallback(async (modelsToSave: string[]) => {
    if (!endpoint) return;

    setIsLoading(true);
    setError(null);

    try {
      await updateProviderEndpoint({
        base_url: endpoint.base_url,
        endpoint_type: endpoint.endpoint_type as TypesProviderEndpointType,
        models: modelsToSave,
      });
      refreshData();
    } catch (err: any) {
      console.error("Failed to update provider endpoint models:", err);
      setError(err.message || "An error occurred while saving changes.");
      setSelectedModels(endpoint?.models || []);
    } finally {
      setIsLoading(false);
    }
  }, [endpoint, updateProviderEndpoint, refreshData]);

  const handleToggleModel = (modelName: string) => {
    const newSelectedModels = selectedModels.includes(modelName)
      ? selectedModels.filter((m) => m !== modelName)
      : [...selectedModels, modelName];
    
    setSelectedModels(newSelectedModels);
    saveModels(newSelectedModels);
  };

  if (!endpoint) return null;

  const availableModels: Array<{ id: string; [key: string]: any }> = 
    Array.isArray(endpoint.available_models) 
      ? (endpoint.available_models as unknown as Array<{ id: string; [key: string]: any }>)
      : [];

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Edit Models for {endpoint.name}</DialogTitle>
      <DialogContent dividers>
        {error && <Alert severity="error" sx={{ mb: 2 }}>{error}</Alert>}
        <Typography variant="body2" gutterBottom>
          Select the models you want to enable for this endpoint.
        </Typography>
        {availableModels.length === 0 ? (
          <Typography variant="body2" color="text.secondary">
            No available models listed for this endpoint. Models might be discovered dynamically.
          </Typography>
        ) : (
          <List dense>
            {availableModels.map((model) => (
              <ListItem key={model.id} secondaryAction={
                <Switch
                  edge="end"
                  onChange={() => handleToggleModel(model.id)}
                  checked={selectedModels.includes(model.id)}
                  disabled={isLoading}
                />
              }>
                <ListItemText primary={model.id} />
              </ListItem>
            ))}
          </List>
        )}
      </DialogContent>
    </Dialog>
  );
};

export default EditProviderModelsDialog; 