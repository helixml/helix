import React from 'react';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import AddCircleIcon from '@mui/icons-material/AddCircle';
import Row from '../widgets/Row';
import Cell from '../widgets/Cell';
import AppAPIKeysDataGrid from '../datagrid/AppAPIKeys';
import StringArrayEditor from '../widgets/StringArrayEditor';
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
  allowedDomains,
  setAllowedDomains,
  isReadOnly,
}) => {
  return (
    <Box sx={{ mt: 2, pr: 3 }}>
      <Typography variant="subtitle1" sx={{mb: 1}}>
        Agent-scoped API Keys
      </Typography>
      <Typography variant="caption" sx={{lineHeight: '3', color: '#999'}}>
        Using this key will automatically force all requests to use this agent.
      </Typography>
      <Row>
        <Cell grow>
          <Typography variant="subtitle1" sx={{mb: 1}}>
            API Keys
          </Typography>
        </Cell>
        <Cell>
          <Button
            size="small"
            variant="outlined"
            endIcon={<AddCircleIcon />}
            onClick={onAddAPIKey}
            disabled={isReadOnly}
          >
            Add API Key
          </Button>
        </Cell>
      </Row>
      <Box sx={{ height: '300px' }}>
        <AppAPIKeysDataGrid
          data={apiKeys}
          onDeleteKey={onDeleteKey}
        />
      </Box>
    </Box>
  );
};

export default APIKeysSection;