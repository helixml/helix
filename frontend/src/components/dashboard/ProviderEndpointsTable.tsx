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
import CreateProviderEndpointDialog from './CreateProviderEndpointDialog';
import DeleteProviderEndpointDialog from './DeleteProviderEndpointDialog';
import EditProviderEndpointDialog from './EditProviderEndpointDialog';
import ProviderEndpointUsageDialog from './ProviderEndpointUsageDialog';
import EditProviderModelsDialog from './EditProviderModelsDialog';
import { useApi } from '../../hooks/useApi';
import useAccount from '../../hooks/useAccount';
import { useListProviders } from '../../services/providersService';
import { getUserById } from '../../services/userService';
import { useGetOrgById } from '../../services/orgService';

interface TypesAggregatedUsageMetric {
  date: string;
  total_tokens: number;
}

// Component to display owner information
const OwnerInfo: FC<{ ownerId: string; ownerType?: string }> = ({ ownerId, ownerType }) => {
  
  
  const { data: user, isLoading, error } = getUserById(ownerId, ownerType === 'user');
  const { data: org, isLoading: isLoadingOrg, error: errorOrg } = useGetOrgById(ownerId, ownerType === 'org');

  if (isLoading || isLoadingOrg) {
    return <Typography variant="body2" color="text.secondary">Loading...</Typography>;
  }

  if (error || errorOrg || (!user && !org)) {
    return (
      <Typography variant="body2" color="error">
        {ownerId}
        {ownerType && ` (${ownerType})`}
      </Typography>
    );
  }

  return (
    <Typography variant="body2">
      {ownerType === 'user' ? user?.email || user?.username || ownerId : org?.name || ownerId}
    </Typography>
  );
};

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
  const account = useAccount()
  const providersManagementEnabled = account.serverConfig.providers_management_enabled ?? false    

  const { data: providerEndpoints = [], isLoading: isLoadingProviders, refetch: loadData } = useListProviders({
    loadModels: false,
    all: true,
    enabled: true,
  });

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

  // Helper function to render owner information
  const renderOwnerInfo = (endpoint: IProviderEndpoint) => {
    if (endpoint.owner === 'system') {
      return 'System';
    }

    // For non-system endpoints, fetch and display user email
    return <OwnerInfo ownerId={endpoint.owner} ownerType={endpoint.owner_type} />;
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
          providersManagementEnabled={providersManagementEnabled}
        />
      </Paper>
    );
  }

  return (
    <Paper sx={{ width: '100%', overflow: 'hidden' }}>
      <Box sx={{ p: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h6">Global Provider Endpoints</Typography>
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
              <TableCell>Billing</TableCell>
              <TableCell>Usage</TableCell>
              <TableCell>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {(providerEndpoints as IProviderEndpoint[]).map((endpoint: IProviderEndpoint) => (
              <TableRow key={endpoint.id && endpoint.id !== "-" ? endpoint.id : endpoint.name}>
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
                <TableCell>{renderOwnerInfo(endpoint)}</TableCell>
                <TableCell>{endpoint.base_url}</TableCell>
                <TableCell>
                  <Typography variant="body2" color={endpoint.billing_enabled ? "success.main" : "text.secondary"}>
                    {endpoint.billing_enabled ? "Enabled" : "Disabled"}
                  </Typography>
                </TableCell>
                <TableCell>
                  <Box sx={{ width: 200, height: 50 }}>
                    <Tooltip
                      title={
                        <Box>
                          <Typography variant="body2" sx={{ mb: 1 }}>
                            Owner: {endpoint.owner_type === 'system' ? 'System' : endpoint.owner_type === 'org' ? 'Organization' : 'User'}
                          </Typography>
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
        providersManagementEnabled={providersManagementEnabled}
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
