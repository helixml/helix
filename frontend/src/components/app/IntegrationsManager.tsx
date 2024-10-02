import React from 'react';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import AddIcon from '@mui/icons-material/Add';
import { ITool } from '../../types';

interface IntegrationsManagerProps {
  tools: ITool[];
  onAddApiTool: () => void;
  onEditTool: (tool: ITool) => void;
  isReadOnly: boolean;
}

const IntegrationsManager: React.FC<IntegrationsManagerProps> = ({
  tools,
  onAddApiTool,
  onEditTool,
  isReadOnly,
}) => {
  return (
    <Box sx={{ mt: 2 }}>
      <Typography variant="h6" sx={{ mb: 1 }}>
        API Tools
      </Typography>
      <Button
        variant="outlined"
        startIcon={<AddIcon />}
        onClick={onAddApiTool}
        sx={{ mb: 2 }}
        disabled={isReadOnly}
      >
        Add API Tool
      </Button>
      <Box sx={{ mb: 2, maxHeight: '300px', overflowY: 'auto' }}>
        {tools.filter(tool => tool.tool_type === 'api').map((apiTool) => (
          <Box
            key={apiTool.id}
            sx={{
              p: 2,
              border: '1px solid #303047',
              mb: 2,
            }}
          >
            <Typography variant="h6">{apiTool.name}</Typography>
            <Typography variant="body1">{apiTool.description}</Typography>
            <Button
              variant="outlined"
              onClick={() => onEditTool(apiTool)}
              sx={{ mt: 1 }}
              disabled={isReadOnly}
            >
              Edit
            </Button>
          </Box>
        ))}
      </Box>
    </Box>
  );
};

export default IntegrationsManager;