import React, { FC, useState } from 'react';
import {
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  Paper,
  Typography,
  Box,
  IconButton,
  Menu,
  MenuItem,
  CircularProgress,
  Tooltip,
  SvgIcon,
} from '@mui/material';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import { TypesModel, TypesModelRuntimeType } from '../../api/api'; // Assuming TypesModel is the correct type
import { useListHelixModels, useDeleteHelixModel } from '../../services/helixModelsService';
import AddIcon from '@mui/icons-material/Add';
import Button from '@mui/material/Button';

import { OllamaIcon, VllmIcon, HuggingFaceIcon } from '../icons/ProviderIcons';
import EditHelixModel from './EditHelixModel';
import DeleteHelixModelDialog from './DeleteHelixModelDialog';

// Helper function to get the icon based on runtime
const getRuntimeIcon = (runtime: TypesModelRuntimeType | undefined) => {
  switch (runtime) {
    case TypesModelRuntimeType.ModelRuntimeTypeOllama:
      return (
        <Tooltip title="Ollama">
          <OllamaIcon />
        </Tooltip>
      );
    case TypesModelRuntimeType.ModelRuntimeTypeVLLM:
      return (
        <Tooltip title="VLLM">
          <VllmIcon />
        </Tooltip>
      );
    case TypesModelRuntimeType.ModelRuntimeTypeDiffusers:
      return (
        <Tooltip title="Diffusers (Hugging Face)">
          <HuggingFaceIcon />
        </Tooltip>
      );
    default:
      return null; // Or a default icon
  }
};

const HelixModelsTable: FC = () => {
  const [dialogOpen, setDialogOpen] = useState(false);
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [selectedModel, setSelectedModel] = useState<TypesModel | null>(null);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);

  // TODO: Add filtering by runtime if needed, e.g., pass "gpu" or "cpu"
  const { data: helixModels = [], isLoading, refetch } = useListHelixModels();
  
  // Note: The useDeleteHelixModel hook requires an ID. 
  // We'll call this hook inside the confirmation dialog, passing the selectedModel.id
  // const deleteMutation = useDeleteHelixModel(selectedModel?.id || ''); // Example usage pattern

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>, model: TypesModel) => {
    setAnchorEl(event.currentTarget);
    setSelectedModel(model);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleEditClick = () => {
    if (selectedModel) {
      setDialogOpen(true);
      handleMenuClose();
    }
  };

  const handleCreateClick = () => {
    setSelectedModel(null);
    setDialogOpen(true);
  };

  const handleDialogClose = () => {
    setDialogOpen(false);
    setSelectedModel(null);
  };

  const handleDeleteClick = () => {
    setDeleteDialogOpen(true);
    handleMenuClose();
  };

  const handleDeleteDialogClose = () => {
    setDeleteDialogOpen(false);
    setSelectedModel(null);
  };

  const handleDeleteSuccess = () => {
    handleDeleteDialogClose();
    refetch();
  };

  if (isLoading) {
    return (
      <Paper sx={{ p: 2, display: 'flex', justifyContent: 'center', alignItems: 'center' }}>
        <CircularProgress />
      </Paper>
    );
  }

  if (!helixModels || helixModels.length === 0) {
    return (
      <Paper sx={{ p: 2, width: '100%' }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <Typography variant="body1">No Helix models found.</Typography>
          <Button variant="outlined" color="secondary" startIcon={<AddIcon />} onClick={handleCreateClick}>
            Create Model
          </Button>
        </Box>
        <EditHelixModel
          open={dialogOpen}
          model={selectedModel}
          onClose={handleDialogClose}
          refreshData={refetch}
        />
      </Paper>
    );
  }

  return (
    <Paper sx={{ width: '100%', overflow: 'hidden' }}>
      <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h6">Helix Models</Typography>
        <Button variant="outlined" color="secondary" startIcon={<AddIcon />} onClick={handleCreateClick}>
          Create Model
        </Button>
      </Box>
      <EditHelixModel
        open={dialogOpen}
        model={selectedModel}
        onClose={handleDialogClose}
        refreshData={refetch}
      />
      <DeleteHelixModelDialog
        open={deleteDialogOpen}
        model={selectedModel}
        onClose={handleDeleteDialogClose}
        onDeleted={handleDeleteSuccess}
      />
      <TableContainer>
        <Table stickyHeader aria-label="helix models table">
          <TableHead>
            <TableRow>
              <TableCell>ID</TableCell>
              <TableCell>Name</TableCell>
              <TableCell>Context Length</TableCell>
              <TableCell>Type</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {helixModels.map((model: TypesModel) => (
              <TableRow key={model.id}>
                <TableCell>
                   <Tooltip title={model.id || 'N/A'}>
                     <Typography variant="body2" sx={{ maxWidth: 150, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                      {model.id || 'N/A'}
                     </Typography>
                   </Tooltip>
                </TableCell>
                <TableCell>
                   <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                     {getRuntimeIcon(model.runtime)}
                     <Typography variant="body2">
                       {model.name}
                       {model.description && (
                         <Typography variant="caption" display="block" color="text.secondary">
                           {model.description}
                         </Typography>
                       )}
                     </Typography>
                   </Box>
                </TableCell>
                <TableCell>{model.context_length || 'N/A'}</TableCell>
                <TableCell>{model.type || 'N/A'}</TableCell>
                <TableCell>
                  <IconButton
                    aria-label="more"
                    onClick={(e) => handleMenuOpen(e, model)}
                  >
                    <MoreVertIcon />
                  </IconButton>
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleEditClick}>Edit</MenuItem>
        <MenuItem onClick={handleDeleteClick} sx={{ color: 'error.main' }}>Delete</MenuItem>
      </Menu>
    </Paper>
  );
};

export default HelixModelsTable;
