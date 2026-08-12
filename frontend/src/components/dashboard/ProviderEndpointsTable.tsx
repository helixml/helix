import React, { FC, useEffect, useMemo, useState } from 'react';
import {
  Typography,
  Box,
  Button,
  IconButton,
  Menu,
  MenuItem,
  Tooltip,
} from '@mui/material';
import { IProviderEndpoint } from '../../types';
import { TypesAggregatedUsageMetric } from '../../api/api';
import { Boxes, EllipsisVertical, ListChecks, Pencil, Plus, Trash2 } from 'lucide-react';
import CreateProviderEndpointDialog from './CreateProviderEndpointDialog';
import DeleteProviderEndpointDialog from './DeleteProviderEndpointDialog';
import EditProviderEndpointDialog from './EditProviderEndpointDialog';
import EditProviderModelsDialog from './EditProviderModelsDialog';
import ProviderEndpointUsageBarChart from './ProviderEndpointUsageBarChart';
import { useApi } from '../../hooks/useApi';
import useAccount from '../../hooks/useAccount';
import { useListProviders } from '../../services/providersService';
import Dialog from '@mui/material/Dialog';
import DialogTitle from '@mui/material/DialogTitle';
import DialogContent from '@mui/material/DialogContent';
import LMStudioModels from '../providers/LMStudioModels';
import { getUserById } from '../../services/userService';
import { useGetOrgById } from '../../services/orgService';
import useRouter from '../../hooks/useRouter';
import SimpleTable, { ITableField } from '../widgets/SimpleTable';
import ProviderEndpointIcon from '../providers/ProviderEndpointIcon';

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

interface ProviderEndpointsTableProps {
  onOpenProvider?: (providerId: string) => void
}

const ProviderEndpointsTable: FC<ProviderEndpointsTableProps> = ({ onOpenProvider }) => {
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false);
  const [editDialogOpen, setEditDialogOpen] = useState(false);
  const [editModelsDialogOpen, setEditModelsDialogOpen] = useState(false);
  const [localModelsDialogOpen, setLocalModelsDialogOpen] = useState(false);
  const [selectedEndpoint, setSelectedEndpoint] = useState<IProviderEndpoint | null>(null);
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [usageData, setUsageData] = useState<{[key: string]: TypesAggregatedUsageMetric[] | null}>({});
  const api = useApi()
  const apiClient = api.getApiClient()
  const account = useAccount()
  const router = useRouter()
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
      
      const usagePromises = endpoints.map(endpoint => {
        const providerId = endpoint.id && endpoint.id !== '-' ? endpoint.id : endpoint.name
        return apiClient.v1ProviderEndpointsDailyUsageDetail(providerId)
          .then(response => ({ [providerId]: response.data as TypesAggregatedUsageMetric[] }))
          .catch(() => ({ [providerId]: null }))
      })
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
    // Don't clear selectedEndpoint here - it's needed by dialogs that open from menu items.
    // The endpoint is cleared when dialogs close (handleDeleteDialogClose, handleEditDialogClose, etc.)
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

  const openProvider = (endpoint: IProviderEndpoint) => {
    const providerId = endpoint.id && endpoint.id !== '-' ? endpoint.id : endpoint.name
    if (onOpenProvider) {
      onOpenProvider(providerId)
      return
    }
    router.navigate('org_provider_detail', {
      org_id: router.params.org_id,
      provider_id: providerId,
    })
  };

  const isSystemEndpoint = (endpoint: IProviderEndpoint) => {
    // Only synthetic env-var endpoints are read-only. They are injected from
    // config with a sentinel id of "-" (no DB row to edit). A real DB row —
    // including a global one — always has a real id and is admin-editable, even
    // if a past bug stamped its owner as "system".
    return endpoint.id === '-';
  };

  // Helper function to render owner information
  const renderOwnerInfo = (endpoint: IProviderEndpoint) => {
    if (endpoint.owner === 'system') {
      return <Typography variant="body2" color="text.secondary">System</Typography>;
    }

    // For non-system endpoints, fetch and display user email
    return <OwnerInfo ownerId={endpoint.owner} ownerType={endpoint.owner_type} />;
  };

  const fields: ITableField[] = [
    { name: 'name', title: 'Name' },
    { name: 'type', title: 'Type' },
    { name: 'owner', title: 'Owner' },
    { name: 'baseUrl', title: 'Base URL' },
    { name: 'billing', title: 'Billing' },
    { name: 'usage', title: 'Usage' },
  ]

  const tableData = useMemo(() => (providerEndpoints as IProviderEndpoint[]).map(endpoint => ({
    id: endpoint.id && endpoint.id !== '-' ? endpoint.id : endpoint.name,
    _data: endpoint,
    name: (
      <Box sx={{ minWidth: 220, maxWidth: 430, display: 'flex', gap: 1.25, alignItems: 'flex-start' }}>
        <Box sx={{ width: 28, height: 28, display: 'flex', alignItems: 'center', justifyContent: 'center', flexShrink: 0, mt: 0.125 }}>
          <ProviderEndpointIcon endpoint={endpoint} size={22} />
        </Box>
        <Box>
          <Typography
            variant="body2"
            component="a"
            href="#"
            onClick={(event: React.MouseEvent) => {
              event.preventDefault()
              event.stopPropagation()
              openProvider(endpoint)
            }}
            sx={{ color: 'text.primary', fontWeight: 600, textDecoration: 'none', '&:hover': { textDecoration: 'underline' } }}
          >
            {endpoint.name}
          </Typography>
          {endpoint.description && (
            <Typography variant="caption" display="block" color="text.secondary" sx={{ mt: 0.25 }}>
              {endpoint.description}
            </Typography>
          )}
        </Box>
      </Box>
    ),
    type: <Typography variant="body2" color="text.secondary">{endpoint.endpoint_type}</Typography>,
    owner: renderOwnerInfo(endpoint),
    baseUrl: (
      <Typography variant="body2" color="text.secondary" sx={{ maxWidth: 310, overflowWrap: 'anywhere' }}>
        {endpoint.base_url || 'Default endpoint'}
      </Typography>
    ),
    billing: (
      <Typography variant="body2" color={endpoint.billing_enabled ? 'success.main' : 'text.secondary'}>
        {endpoint.billing_enabled ? 'Enabled' : 'Disabled'}
      </Typography>
    ),
    usage: (
      <ProviderEndpointUsageBarChart
        data={usageData[endpoint.id && endpoint.id !== '-' ? endpoint.id : endpoint.name]}
        onClick={() => openProvider(endpoint)}
      />
    ),
  })), [providerEndpoints, usageData])

  if (!providerEndpoints || providerEndpoints.length === 0) {
    return (
      <Box sx={{ width: '100%' }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <Typography variant="body1">No provider endpoints configured.</Typography>
          <Button
            variant="outlined"
            color="secondary"
            startIcon={<Plus size={18} />}
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
      </Box>
    );
  }

  return (
    <Box sx={{ width: '100%', overflow: 'hidden' }}>
      <Box sx={{ pb: 2, display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
        <Typography variant="h6">Global Provider Endpoints</Typography>
        <Button
          variant="outlined"
          color="secondary"
          startIcon={<Plus size={18} />}
          onClick={() => setCreateDialogOpen(true)}
        >
          Add Endpoint
        </Button>
      </Box>
      <SimpleTable
        authenticated
        fields={fields}
        data={tableData}
        loading={isLoadingProviders}
        onRowClick={row => openProvider(row._data as IProviderEndpoint)}
        getActions={row => {
          const endpoint = row._data as IProviderEndpoint
          if (isSystemEndpoint(endpoint)) {
            return (
              <Tooltip title="System endpoints can only be configured through environment variables in your Helix instance">
                <span>
                  <IconButton aria-label={`Actions for ${endpoint.name}`} disabled size="small">
                    <EllipsisVertical size={18} />
                  </IconButton>
                </span>
              </Tooltip>
            )
          }
          return (
            <Tooltip title="Provider actions">
              <IconButton
                aria-label={`Actions for ${endpoint.name}`}
                size="small"
                onClick={event => {
                  event.stopPropagation()
                  handleMenuOpen(event, endpoint)
                }}
              >
                <EllipsisVertical size={18} />
              </IconButton>
            </Tooltip>
          )
        }}
      />
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
        <MenuItem onClick={event => { event.stopPropagation(); handleEditClick() }} sx={{ gap: 1.25 }}>
          <Pencil size={20} /> Edit Details
        </MenuItem>
        <MenuItem onClick={event => { event.stopPropagation(); handleEditModelsClick() }} sx={{ gap: 1.25 }}>
          <ListChecks size={20} /> Edit Models
        </MenuItem>
        {selectedEndpoint && (selectedEndpoint.name === 'lmstudio' || selectedEndpoint.name === 'ollama' || selectedEndpoint.name?.includes('lmstudio') || selectedEndpoint.name?.includes('ollama')) && (
          <MenuItem onClick={() => {
            handleMenuClose();
            setLocalModelsDialogOpen(true);
          }} sx={{ gap: 1.25 }}><Boxes size={20} /> Manage Local Models</MenuItem>
        )}
        <MenuItem onClick={event => { event.stopPropagation(); handleDeleteClick() }} sx={{ gap: 1.25 }}>
          <Trash2 size={20} /> Delete
        </MenuItem>
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
      <EditProviderModelsDialog
        open={editModelsDialogOpen}
        endpoint={selectedEndpoint}
        onClose={handleEditModelsDialogClose}
        refreshData={loadData}
      />
      <Dialog
        open={localModelsDialogOpen}
        onClose={() => { setLocalModelsDialogOpen(false); setSelectedEndpoint(null); }}
        maxWidth="lg"
        fullWidth
        PaperProps={{ sx: { maxHeight: '85vh' } }}
      >
        <DialogTitle sx={{ fontSize: "1rem", fontWeight: 600, display: "flex", justifyContent: "space-between", alignItems: "center" }}>
          Manage Local Models — {selectedEndpoint?.name}
        </DialogTitle>
        <DialogContent sx={{ overflow: "auto" }}>
          {selectedEndpoint?.id && localModelsDialogOpen && (
            <LMStudioModels endpointId={selectedEndpoint.id} />
          )}
        </DialogContent>
      </Dialog>
    </Box>
  );
};

export default ProviderEndpointsTable;
