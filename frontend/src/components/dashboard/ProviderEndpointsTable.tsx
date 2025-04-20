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
import { SparkLineChart } from '@mui/x-charts';
import { IProviderEndpoint } from '../../types';
import AddIcon from '@mui/icons-material/Add';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import LockIcon from '@mui/icons-material/Lock';
import LockOpenIcon from '@mui/icons-material/LockOpen';
import CreateProviderEndpointDialog from './CreateProviderEndpointDialog';
import DeleteProviderEndpointDialog from './DeleteProviderEndpointDialog';
import EditProviderEndpointDialog from './EditProviderEndpointDialog';
import ProviderEndpointUsageDialog from './ProviderEndpointUsageDialog';
import EditProviderModelsDialog from './EditProviderModelsDialog';
import { useApi } from '../../hooks/useApi';
import { useListProviders } from '../../services/providersService';

interface TypesAggregatedUsageMetric {
  date: string;
  total_tokens: number;
}

const ProviderEndpointsTable: FC = () => {
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [usageDialogOpen, setUsageDialogOpen] = useState(false);
  const [editModelsDialogOpen, setEditModelsDialogOpen] = useState(false);
  const [selectedEndpoint, setSelectedEndpoint] = useState<IProviderEndpoint | null>(null);
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [usageData, setUsageData] = useState<{[key: string]: TypesAggregatedUsageMetric[] | null}>({});  
  const api = useApi()
  const apiClient = api.getApiClient()    

  const { data: providerEndpoints = [], isLoading: isLoadingProviders, refetch: loadData } = useListProviders(true);

  // Fetch usage data for all providers
  useEffect(() => {
    const fetchUsageData = async () => {      
      if (isLoadingProviders) return;      

      let endpoints = providerEndpoints as IProviderEndpoint[]
      
      const usagePromises = endpoints.map(endpoint => 
        apiClient.v1ProviderEndpointsDailyUsageDetail(endpoint.id && endpoint.id !== "-" ? endpoint.id : endpoint.name)
          .then(response => ({ [endpoint.name]: response.data as TypesAggregatedUsageMetric[] }))
          .catch(() => ({ [endpoint.name]: null }))
      )
      const results = await Promise.all(usagePromises)
      const combinedData = results.reduce((acc, curr) => ({ ...acc, ...curr }), {} as {[key: string]: TypesAggregatedUsageMetric[] | null})
      setUsageData(combinedData)
    }
    fetchUsageData()
  }, [providerEndpoints])

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

  const handleEditModelsClick = () => {
    setEditModelsDialogOpen(true);
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

  const handleEditModelsDialogClose = () => {
    setEditModelsDialogOpen(false);
    setSelectedEndpoint(null);
    handleMenuClose();
  };

  const handleUsageClick = (endpoint: IProviderEndpoint) => {
    setSelectedEndpoint(endpoint);
    setUsageDialogOpen(true);
  };

  const isSystemEndpoint = (endpoint: IProviderEndpoint) => {
    return endpoint.endpoint_type === 'global' && endpoint.owner === 'system';
  };

  const renderAuthCell = (endpoint: IProviderEndpoint) => {
    // If endpoint is global, don't show anything
    if (isSystemEndpoint(endpoint)) {
      return null;
    }

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

  if (!providerEndpoints || providerEndpoints.length === 0) {
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
          existingEndpoints={providerEndpoints as IProviderEndpoint[]}
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
              <TableCell>Default</TableCell>
              <TableCell>Usage</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {(providerEndpoints as IProviderEndpoint[]).map((endpoint: IProviderEndpoint) => (
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
                <TableCell>{endpoint.default ? 'Yes' : 'No'}</TableCell>
                <TableCell>
                  <Box sx={{ width: 200, height: 50 }}>
                    <Tooltip
                      title={
                        <Box>
                          <Typography variant="body2">Daily usage:</Typography>
                          {(usageData[endpoint.name] || []).map((day: TypesAggregatedUsageMetric, i: number) => (
                            <Typography key={i} variant="caption" component="div">
                              {new Date(day.date).toLocaleDateString()}: {day.total_tokens || 0} tokens
                            </Typography>
                          ))}
                          <Typography variant="body2" sx={{ mt: 1 }}>
                            Today: {usageData[endpoint.name]?.[(usageData[endpoint.name] || []).length - 1]?.total_tokens || 0} tokens
                          </Typography>
                          <Typography variant="body2">
                            Total (7 days): {(usageData[endpoint.name] || []).reduce((sum: number, day: TypesAggregatedUsageMetric) => sum + (day.total_tokens || 0), 0)} tokens
                          </Typography>
                        </Box>
                      }
                    >
                      <Box onClick={() => handleUsageClick(endpoint)} sx={{ cursor: 'pointer' }}>
                        <SparkLineChart
                          data={(usageData[endpoint.name] || []).map(day => day.total_tokens || 0)}
                          height={50}
                          width={200}
                          showTooltip={true}
                          curve="linear"
                        />
                      </Box>
                    </Tooltip>
                  </Box>
                </TableCell>
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
        existingEndpoints={providerEndpoints as IProviderEndpoint[]}
      />
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleEditClick}>Edit Details</MenuItem>
        <MenuItem onClick={handleEditModelsClick}>Edit Models</MenuItem>
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
        refreshData={loadData}
      />
      <ProviderEndpointUsageDialog
        open={usageDialogOpen}
        endpoint={selectedEndpoint}
        onClose={() => setUsageDialogOpen(false)}
      />
      <EditProviderModelsDialog
        open={editModelsDialogOpen}
        endpoint={selectedEndpoint}
        onClose={handleEditModelsDialogClose}
        refreshData={loadData}
      />
    </Paper>
  );
};

export default ProviderEndpointsTable;
