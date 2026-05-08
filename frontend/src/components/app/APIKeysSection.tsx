import React, { FC } from 'react';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import Table from '@mui/material/Table';
import TableBody from '@mui/material/TableBody';
import TableCell from '@mui/material/TableCell';
import TableContainer from '@mui/material/TableContainer';
import TableHead from '@mui/material/TableHead';
import TableRow from '@mui/material/TableRow';
import IconButton from '@mui/material/IconButton';
import Tooltip from '@mui/material/Tooltip';
import { Trash2, Plus } from 'lucide-react';

import MaskedSecret from '../widgets/MaskedSecret';
import { IApiKey } from '../../types';

interface APIKeysSectionProps {
  apiKeys: IApiKey[];
  onAddAPIKey: () => void;
  onDeleteKey: (key: string) => void;
  allowedDomains: string[];
  setAllowedDomains: (domains: string[]) => void;
  isReadOnly: boolean;
}

const APIKeysSection: React.FC<APIKeysSectionProps> = ({
  apiKeys,
  onAddAPIKey,
  onDeleteKey,
  isReadOnly,
}) => {
  return (
    <Box sx={{ mt: 2, pr: 3 }}>
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 1 }}>
        <Box>
          <Typography variant="subtitle1">
            Agent-scoped API Keys
          </Typography>
          <Typography variant="caption" sx={{ color: '#999' }}>
            Using this key will automatically force all requests to use this agent.
          </Typography>
        </Box>
        <Button
          size="small"
          variant="outlined"
          color="secondary"
          startIcon={<Plus size={16} />}
          onClick={onAddAPIKey}
          disabled={isReadOnly}
        >
          Add API Key
        </Button>
      </Box>

      <TableContainer>
        <Table size="small">
          <TableHead>
            <TableRow>
              <TableCell sx={{ fontWeight: 600 }}>Key</TableCell>
              <TableCell align="right" sx={{ fontWeight: 600 }}>Actions</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {apiKeys.length === 0 ? (
              <TableRow>
                <TableCell colSpan={2} align="center" sx={{ py: 4 }}>
                  <Typography variant="body2" color="text.secondary">
                    No API keys yet. Add one to get started.
                  </Typography>
                </TableCell>
              </TableRow>
            ) : (
              apiKeys.map((apiKey) => (
                <TableRow key={apiKey.key} hover>
                  <TableCell>
                    <MaskedSecret value={apiKey.key} ariaLabel="API key" />
                  </TableCell>
                  <TableCell align="right">
                    <Tooltip title="Delete API Key">
                      <span>
                        <IconButton
                          size="small"
                          onClick={() => onDeleteKey(apiKey.key)}
                          disabled={isReadOnly}
                        >
                          <Trash2 size={16} />
                        </IconButton>
                      </span>
                    </Tooltip>
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
      </TableContainer>
    </Box>
  );
};

export default APIKeysSection;
