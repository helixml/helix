import React, { FC, useState, useEffect } from 'react';
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
  Button,
  IconButton,
  Menu,
  MenuItem,
  Tooltip,
} from '@mui/material';
import { IProviderEndpoint } from '../../types';
import useAccount from '../../hooks/useAccount';
import AddIcon from '@mui/icons-material/Add';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import LockIcon from '@mui/icons-material/Lock';
import LockOpenIcon from '@mui/icons-material/LockOpen';
import CreateProviderEndpointDialog from './CreateProviderEndpointDialog';
import DeleteProviderEndpointDialog from './DeleteProviderEndpointDialog';
import EditProviderEndpointDialog from './EditProviderEndpointDialog';
import useEndpointProviders from '../../hooks/useEndpointProviders';

const ProviderEndpointsTable: FC = () => {
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [selectedEndpoint, setSelectedEndpoint] = useState<IProviderEndpoint | null>(null);
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const account = useAccount();
  const { loadData } = useEndpointProviders();

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>, endpoint: IProviderEndpoint) => {
    setAnchorEl(event.currentTarget);
    setSelectedEndpoint(endpoint);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
    setSelectedEndpoint(null);
  };

  const handleDeleteClick = () => {
    setDeleteDialogOpen(true);
  };

  const handleEditClick = () => {
    setEditDialogOpen(true);
  };

  const handleDeleteDialogClose = () => {
    setDeleteDialogOpen(false);
    setSelectedEndpoint(null);
    handleMenuClose();
  };

  const handleEditDialogClose = () => {
    setEditDialogOpen(false);
    setSelectedEndpoint(null);
    handleMenuClose();
  };

  const isSystemEndpoint = (endpoint: IProviderEndpoint) => {
    return endpoint.endpoint_type === 'global' && endpoint.owner === 'system';
  };

  const renderAuthCell = (endpoint: IProviderEndpoint) => {
    if (endpoint.api_key === '********') {
      return (
        <Box sx={{ display: 'flex', alignItems: 'center' }}>
          <LockIcon fontSize="small" sx={{ mr: 1 }} />
          <Typography variant="body2">token</Typography>
        </Box>
      );
    }
    if (endpoint.api_key_file) {
      return (
        <Typography variant="body2">
          File: {endpoint.api_key_file}
        </Typography>
      );
    }
    return (
      <Box sx={{ display: 'flex', alignItems: 'center' }}>
        <LockOpenIcon fontSize="small" sx={{ mr: 1 }} />
        <Typography variant="body2">none</Typography>
      </Box>
    );
  };

  if (!account.providerEndpoints || account.providerEndpoints.length === 0) {
    return (
      <Paper sx={{ p: 2, width: '100%' }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <Typography variant="body1">No provider endpoints configured.</Typography>
          <Button
            variant="outlined"
            color="secondary"
            startIcon={<AddIcon />}
            onClick={() => setCreateDialogOpen(true)}
          >
            Add Endpoint
          </Button>
        </Box>
        <CreateProviderEndpointDialog
          open={createDialogOpen}
          onClose={() => setCreateDialogOpen(false)}
          existingEndpoints={account.providerEndpoints}
        />
      </Paper>
    );
  }

  return (
    <Paper sx={{ width: '100%', overflow: 'hidden' }}>
      <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h6">Provider Endpoints</Typography>
        <Button
          variant="outlined"
          color="secondary"
          startIcon={<AddIcon />}
          onClick={() => setCreateDialogOpen(true)}
        >
          Add Endpoint
        </Button>
      </Box>
      <TableContainer>
        <Table stickyHeader aria-label="provider endpoints table">
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Type</TableCell>
              <TableCell>Owner</TableCell>
              <TableCell>Base URL</TableCell>
              <TableCell>Auth</TableCell>
              <TableCell>Default</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {account.providerEndpoints.map((endpoint: IProviderEndpoint) => (
              <TableRow key={endpoint.name}>
                <TableCell>
                  <Typography variant="body2">
                    {endpoint.name}
                    {endpoint.description && (
                      <Typography variant="caption" display="block" color="text.secondary">
                        {endpoint.description}
                      </Typography>
                    )}
                  </Typography>
                </TableCell>
                <TableCell>{endpoint.endpoint_type}</TableCell>
                <TableCell>{endpoint.owner_type ? `${endpoint.owner} (${endpoint.owner_type})` : endpoint.owner}</TableCell>
                <TableCell>{endpoint.base_url}</TableCell>
                <TableCell>{renderAuthCell(endpoint)}</TableCell>
                <TableCell>{endpoint.default ? 'Yes' : 'No'}</TableCell>
                <TableCell>
                  {isSystemEndpoint(endpoint) ? (
                    <Tooltip title="System endpoints can only be configured through environment variables in your Helix instance">
                      <span>
                        <IconButton
                          aria-label="more"
                          disabled={true}
                        >
                          <MoreVertIcon />
                        </IconButton>
                      </span>
                    </Tooltip>
                  ) : (
                    <IconButton
                      aria-label="more"
                      onClick={(e) => handleMenuOpen(e, endpoint)}
                    >
                      <MoreVertIcon />
                    </IconButton>
                  )}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </TableContainer>
      <CreateProviderEndpointDialog
        open={createDialogOpen}
        onClose={() => setCreateDialogOpen(false)}
        existingEndpoints={account.providerEndpoints}
      />
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleEditClick}>Edit</MenuItem>
        <MenuItem onClick={handleDeleteClick}>Delete</MenuItem>
      </Menu>
      <DeleteProviderEndpointDialog
        open={deleteDialogOpen}
        endpoint={selectedEndpoint}
        onClose={handleDeleteDialogClose}
        onDeleted={loadData}
      />
      <EditProviderEndpointDialog
        open={editDialogOpen}
        endpoint={selectedEndpoint}
        onClose={handleEditDialogClose}
      />
    </Paper>
  );
};

export default ProviderEndpointsTable;
