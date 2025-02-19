import React, { FC } from 'react';
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
} from '@mui/material';
import { IProviderEndpoint } from '../../types';
import useAccount from '../../hooks/useAccount';

const ProviderEndpointsTable: FC = () => {
  const account = useAccount();

  if (!account.providerEndpoints || account.providerEndpoints.length === 0) {
    return (
      <Paper sx={{ p: 2, width: '100%' }}>
        <Typography variant="body1">No provider endpoints configured.</Typography>
      </Paper>
    );
  }

  return (
    <Paper sx={{ width: '100%', overflow: 'hidden' }}>
      <Box sx={{ p: 2 }}>
        <Typography variant="h6">Provider Endpoints</Typography>
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
              <TableRow key={endpoint.id}>
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
    </Paper>
  );
};

export default ProviderEndpointsTable;
