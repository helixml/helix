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
} from '@mui/material';
import { IProviderEndpoint } from '../../types';
import useAccount from '../../hooks/useAccount';
import AddIcon from '@mui/icons-material/Add';
import CreateProviderEndpointDialog from './CreateProviderEndpointDialog';
import useEndpointProviders from '../../hooks/useEndpointProviders';

const ProviderEndpointsTable: FC = () => {
  const [createDialogOpen, setCreateDialogOpen] = useState(false);
  const account = useAccount();
  
  if (!account.providerEndpoints || account.providerEndpoints.length === 0) {
    return (
      <Paper sx={{ p: 2, width: '100%' }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
          <Typography variant="body1">No provider endpoints configured.</Typography>
          <Button
            variant="contained"
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
          variant="contained"
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
              <TableCell>API Key File</TableCell>
              <TableCell>Default</TableCell>
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
                <TableCell>{endpoint.api_key_file || 'N/A'}</TableCell>
                <TableCell>{endpoint.default ? 'Yes' : 'No'}</TableCell>
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
    </Paper>
  );
};

export default ProviderEndpointsTable;
