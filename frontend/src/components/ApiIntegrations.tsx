import React, { useState } from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import AddIcon from '@mui/icons-material/Add';
import { ITool } from '../types';
import ToolEditor from './ToolEditor';
import Window from './widgets/Window';

interface ApiIntegrationsProps {
  tools: ITool[];
  onAddApiTool: () => void;
  onSaveApiTool: (tool: ITool) => void;
  isReadOnly: boolean;
}

const ApiIntegrations: React.FC<ApiIntegrationsProps> = ({
  tools,
  onAddApiTool,
  onSaveApiTool,
  isReadOnly,
}) => {
  const [editingTool, setEditingTool] = useState<ITool | null>(null);

  const handleEditTool = (tool: ITool) => {
    setEditingTool(tool);
  };

  const handleSaveTool = (tool: ITool) => {
    onSaveApiTool(tool);
    setEditingTool(null);
  };

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
              onClick={() => handleEditTool(apiTool)}
              sx={{ mt: 1 }}
              disabled={isReadOnly}
            >
              Edit
            </Button>
          </Box>
        ))}
      </Box>
      {editingTool && (
        <Window
          title={`${editingTool.id ? 'Edit' : 'Add'} API Tool`}
          fullHeight
          size="lg"
          open
          withCancel
          cancelTitle="Close"
          onCancel={() => setEditingTool(null)}
        >
          <ToolEditor
            initialData={editingTool}
            onSave={handleSaveTool}
            onCancel={() => setEditingTool(null)}
            isReadOnly={isReadOnly}
          />
        </Window>
      )}
    </Box>
  );
};

export default ApiIntegrations;