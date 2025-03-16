import React from 'react';
import Typography from '@mui/material/Typography';
import Button from '@mui/material/Button';
import Box from '@mui/material/Box';
import AddIcon from '@mui/icons-material/Add';
import DeleteIcon from '@mui/icons-material/Delete';
import { IAssistantGPTScript, ITool } from '../../types';

interface GPTScriptsSectionProps {
  app: any;
  onAddGptScript: () => void;
  onDeleteGptScript: (scriptIndex: number) => void;
  isReadOnly: boolean;
  isGithubApp: boolean;
  onEdit: (script: IAssistantGPTScript, index: number) => void;
}

const GPTScriptsSection: React.FC<GPTScriptsSectionProps> = ({
  app,
  onAddGptScript,
  onDeleteGptScript,
  isReadOnly,
  isGithubApp,
  onEdit,
}) => {
  const gptScripts = app?.config?.helix?.assistants?.[0]?.gptscripts || [];
  return (
    <Box sx={{ mt: 2 }}>
      <Typography variant="h6" sx={{ mb: 1 }}>
        GPTScripts
      </Typography>
      <Button
        variant="outlined"
        startIcon={<AddIcon />}
        onClick={onAddGptScript}
        sx={{ mb: 2 }}
        disabled={isReadOnly || isGithubApp}
      >
        Add GPTScript
      </Button>
      <Box sx={{ mb: 2, maxHeight: '300px', overflowY: 'auto' }}>
        {gptScripts.map((script: IAssistantGPTScript, index: number) => (
          <Box
            key={`script${index}`}
            sx={{
              p: 2,
              border: '1px solid #303047',
              mb: 2,
            }}
          >
            <Typography variant="subtitle1">{script.name}</Typography>
            <Typography variant="body2">{script.description}</Typography>
            <Box sx={{ mt: 1 }}>
              <Button
                variant="outlined"
                onClick={() => onEdit(script, index)}
                sx={{ mr: 1 }}
                disabled={isReadOnly || isGithubApp}
              >
                Edit
              </Button>
              <Button
                variant="outlined"
                color="error"
                onClick={() => onDeleteGptScript(index)}
                disabled={isReadOnly || isGithubApp}
                startIcon={<DeleteIcon />}
              >
                Delete
              </Button>
            </Box>
          </Box>
        ))}
      </Box>
    </Box>
  );
};

export default GPTScriptsSection;