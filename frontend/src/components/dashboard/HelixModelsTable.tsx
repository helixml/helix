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
  TextField,
  Switch,
} from '@mui/material';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import { TypesModel, TypesRuntime } from '../../api/api'; // Assuming TypesModel is the correct type
import { useListHelixModels, useUpdateHelixModel } from '../../services/helixModelsService';
import AddIcon from '@mui/icons-material/Add';
import Button from '@mui/material/Button';

import { OllamaIcon, VllmIcon, HuggingFaceIcon } from '../icons/ProviderIcons';
import EditHelixModel from './EditHelixModel';
import DeleteHelixModelDialog from './DeleteHelixModelDialog';

// Helper function to format date for tooltip
const formatFullDate = (dateString: string | undefined): string => {
  if (!dateString) return 'N/A';
  try {
    const date = new Date(dateString);
    const hours = date.getHours().toString().padStart(2, '0');
    const minutes = date.getMinutes().toString().padStart(2, '0');
    const day = date.getDate();
    const monthNames = ["Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"];
    const month = monthNames[date.getMonth()];
    const year = date.getFullYear();

    let daySuffix = 'th';
    if (day % 10 === 1 && day !== 11) {
      daySuffix = 'st';
    } else if (day % 10 === 2 && day !== 12) {
      daySuffix = 'nd';
    } else if (day % 10 === 3 && day !== 13) {
      daySuffix = 'rd';
    }

    return `${hours}:${minutes} ${day}${daySuffix} ${month}, ${year}`;
  } catch (error) {
    console.error("Error formatting date:", dateString, error);
    return 'Invalid Date';
  }
};

// Helper function to format date shortly for the column
const formatShortDate = (dateString: string | undefined): string => {
  if (!dateString) return 'N/A';
  try {
    const date = new Date(dateString);
    const year = date.getFullYear();
    const month = (date.getMonth() + 1).toString().padStart(2, '0'); // Month is 0-indexed
    const day = date.getDate().toString().padStart(2, '0');
    const hours = date.getHours().toString().padStart(2, '0');
    const minutes = date.getMinutes().toString().padStart(2, '0');
    return `${year}-${month}-${day} ${hours}:${minutes}`;
  } catch (error) {
    console.error("Error formatting short date:", dateString, error);
    return 'Invalid Date';
  }
};

// Helper function to get the icon based on runtime
const getRuntimeIcon = (runtime: TypesRuntime | undefined) => {
  switch (runtime) {
    case TypesRuntime.RuntimeOllama:
      return (
        <Tooltip title="Ollama">
          <span><OllamaIcon /></span>
        </Tooltip>
      );
    case TypesRuntime.RuntimeVLLM:
      return (
        <Tooltip title="VLLM">
          <span><VllmIcon /></span>
        </Tooltip>
      );
    case TypesRuntime.RuntimeDiffusers:
      return (
        <Tooltip title="Diffusers (Hugging Face)">
          <span><HuggingFaceIcon /></span>
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
  const [searchQuery, setSearchQuery] = useState('');

  // TODO: Add filtering by runtime if needed, e.g., pass "gpu" or "cpu"
  const { data: helixModels = [], isLoading, refetch } = useListHelixModels();

  // Call the hook at the top level
  const { mutateAsync: updateModel, isPending: isUpdating } = useUpdateHelixModel();

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

  // Placeholder for the API call to update the model's enabled status
  const handleToggleEnable = (model: TypesModel) => {
    if (!model.id) {
      console.error("Cannot toggle model status: model ID is missing.");
      return;
    }
    const updatedModel = {
      ...model,
      enabled: !(model.enabled ?? false),
    };
    // Remove the hook call from here
    // const { mutateAsync: updateModel, isPending: isUpdating } = useUpdateHelixModel(model.id || '');

    // Call updateModel with id and data
    updateModel({ id: model.id, helixModel: updatedModel }, {
      onSuccess: () => {
        refetch();
        console.log(`Model ${model.id} enabled status updated successfully.`);
      },
      onError: (error) => {
        console.error(`Failed to update model ${model.id} enabled status:`, error);
        // TODO: Add user feedback for the error (e.g., Snackbar)
      },
    });
  };

  // Placeholder for the API call to update the model's auto_pull status
  const handleToggleAutoPull = (model: TypesModel) => {
    if (!model.id) {
      console.error("Cannot toggle model auto_pull status: model ID is missing.");
      return;
    }
    const updatedModel = {
      ...model,
      auto_pull: !(model.auto_pull ?? false), // Toggle auto_pull, default to false if undefined
    };

    // Call updateModel with id and data
    updateModel({ id: model.id, helixModel: updatedModel }, {
      onSuccess: () => {
        refetch();
        console.log(`Model ${model.id} auto_pull status updated successfully.`);
      },
      onError: (error) => {
        console.error(`Failed to update model ${model.id} auto_pull status:`, error);
        // TODO: Add user feedback for the error (e.g., Snackbar)
      },
    });
  };

  // Filter models based on search query
  const filteredModels = helixModels.filter((model) => {
    const query = searchQuery.toLowerCase();
    return (
      model.id?.toLowerCase().includes(query) ||
      model.name?.toLowerCase().includes(query) ||
      model.description?.toLowerCase().includes(query)
    );
  });

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
            Add Model
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
        <TextField
           label="Search models by ID, name, or description"           
           size="small"
           value={searchQuery}
           onChange={(e) => setSearchQuery(e.target.value)}
           sx={{ width: '40%' }}
         />
        <Button variant="outlined" color="secondary" startIcon={<AddIcon />} onClick={handleCreateClick}>
          Add Model
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
              <TableCell>Enabled</TableCell>
              <TableCell>Auto pull</TableCell>
              <TableCell>Created At</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {filteredModels.map((model: TypesModel) => (
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
                  <Switch
                    checked={model.enabled ?? false}
                    onChange={() => handleToggleEnable(model)}
                  />
                </TableCell>
                <TableCell>
                  <Tooltip title={model.runtime !== TypesRuntime.RuntimeOllama ? "This runtime currently doesn't support automatic pulls" : ""}>
                    {/* Wrap the Switch in a span if it's disabled for Tooltip to work correctly */}
                    <span>
                      <Switch
                        checked={model.auto_pull ?? false} // Default to false if undefined
                        onChange={() => handleToggleAutoPull(model)}
                        disabled={isUpdating || model.runtime !== TypesRuntime.RuntimeOllama}
                      />
                    </span>
                  </Tooltip>
                </TableCell>
                <TableCell>
                  <Tooltip title={formatFullDate(model.created)}>
                    <span>{formatShortDate(model.created)}</span>
                  </Tooltip>
                </TableCell>
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
