import React from 'react';
import Typography from '@mui/material/Typography';
import Box from '@mui/material/Box';
import Button from '@mui/material/Button';
import AddIcon from '@mui/icons-material/Add';
import StringMapEditor from '../widgets/StringMapEditor';
import { IAssistantGPTScript, ITool } from '../../types';

interface GPTScriptsManagerProps {
  gptScripts: IAssistantGPTScript[];
  onAddGptScript: () => void;
  onEditTool: (tool: ITool) => void;
  isReadOnly: boolean;
  isGithubApp: boolean;
  secrets: Record<string, string>;
  setSecrets: (secrets: Record<string, string>) => void;
}

const GPTScriptsManager: React.FC<GPTScriptsManagerProps> = ({
  gptScripts,
  onAddGptScript,
  onEditTool,
  isReadOnly,
  isGithubApp,
  secrets,
  setSecrets,
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
        {gptScripts.map((script) => (
          <Box
            key={script.file}
            sx={{
              p: 2,
              border: '1px solid #303047',
              mb: 2,
            }}
          >
            <Typography variant="subtitle1">{script.name}</Typography>
            <Typography variant="body2">{script.description}</Typography>
            <Button
              variant="outlined"
              onClick={() => onEditTool({
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
              sx={{ mt: 1 }}
              disabled={isReadOnly || isGithubApp}
            >
              Edit
            </Button>
          </Box>
        ))}
      </Box>
      <Typography variant="subtitle1" sx={{ mt: 4 }}>
        Environment Variables
      </Typography>
      <Typography variant="caption" sx={{lineHeight: '3', color: '#999'}}>
        These will be available to your GPTScripts as environment variables
      </Typography>
      <StringMapEditor
        entityTitle="variable"
        disabled={isReadOnly}
        data={secrets}
        onChange={setSecrets}
      />
    </Box>
  );
};

export default GPTScriptsManager;