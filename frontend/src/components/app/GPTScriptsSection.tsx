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
  setEditingTool: (tool: ITool | null) => void;
  onDeleteTool: (id: string) => void;
  isReadOnly: boolean;
  isGithubApp: boolean;
}

const GPTScriptsSection: React.FC<GPTScriptsSectionProps> = ({
  app,
  onAddGptScript,
  setEditingTool,
  onDeleteTool,
  isReadOnly,
  isGithubApp,
}) => {
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
        {app?.config.helix?.assistants?.flatMap((assistant: { id: string, gptscripts?: IAssistantGPTScript[] }) => 
          assistant.gptscripts?.map((script: IAssistantGPTScript, index: number) => (
            <Box
              key={`${assistant.id}-${script.file}`}
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
                  onClick={() => setEditingTool({
                    id: script.file,
                    name: script.name,
                    description: script.description,
                    tool_type: 'gptscript',
                    global: false,
                    config: {
                      gptscript: {
                        script: script.content,
                      }
                    },
                    created: '',
                    updated: '',
                    owner: '',
                    owner_type: 'user',
                  })}
                  sx={{ mr: 1 }}
                  disabled={isReadOnly || isGithubApp}
                >
                  Edit
                </Button>
                <Button
                  variant="outlined"
                  color="error"
                  onClick={() => onDeleteTool(script.file)}
                  disabled={isReadOnly || isGithubApp}
                  startIcon={<DeleteIcon />}
                >
                  Delete
                </Button>
              </Box>
            </Box>
          )) || []
        )}
      </Box>
    </Box>
  );
};

export default GPTScriptsSection;