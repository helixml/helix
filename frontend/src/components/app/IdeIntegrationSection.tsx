import React, { useState } from 'react';
import Box from '@mui/material/Box';
import Typography from '@mui/material/Typography';
import TextField from '@mui/material/TextField';
import FormControl from '@mui/material/FormControl';
import InputLabel from '@mui/material/InputLabel';
import Select from '@mui/material/Select';
import MenuItem from '@mui/material/MenuItem';
import JsonWindowLink from '../widgets/JsonWindowLink';
import Link from '@mui/material/Link';
import Button from '@mui/material/Button';
import ContentCopyIcon from '@mui/icons-material/ContentCopy';
import useSnackbar from '../../hooks/useSnackbar';

interface IdeIntegrationSectionProps {
  appId: string;
  apiKey: string;
}

const IdeIntegrationSection: React.FC<IdeIntegrationSectionProps> = ({
  appId,
  apiKey,
}) => {
  const [selectedIde, setSelectedIde] = useState<string>('cline');
  const { success: snackbarSuccess } = useSnackbar();

  const getClineConfig = () => {
    return `{
  "mcpServers": {
    "helix-mcp": {
      "command": "/usr/local/bin/helix",
      "args": [
        "mcp",
        "run",
        "--app-id", "${appId}",
        "--api-key", "${apiKey}",
        "--url", "http://localhost:8080"
      ]
    }
  }
}`;
  };

  const getClaudeDesktopConfig = () => {
    return `{
  "mcpServers": {
    "helix-mcp": {
      "command": "/usr/local/bin/helix",
      "args": [
        "mcp",
        "run",
        "--app-id", "${appId}",
        "--api-key", "${apiKey}",
        "--url", "http://localhost:8080"
      ]
    }
  }
}`;
  };

  const renderIdeInstructions = () => {
    switch (selectedIde) {
      case 'cline':
        return (
          <>
            <Typography variant="body1" sx={{ mb: 2 }}>
              Follow these steps to configure <Link href="https://cline.bot/" target="_blank" rel="noopener noreferrer">Cline</Link>:
            </Typography>
            <Box sx={{ mb: 2 }}>
              <Typography variant="body2" sx={{ mb: 1 }}>1. Open the command palette (CMD+Shift+P)</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>2. Type "MCP Servers"</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>3. Add the following configuration:</Typography>
            </Box>
            <TextField
              value={getClineConfig()}
              fullWidth
              multiline
              
              disabled
              InputProps={{
                style: { fontFamily: 'monospace' }
              }}
            />
            <Box sx={{ textAlign: 'right', mb: 1 }}>
              <Button
                variant="text"
                size="small"
                startIcon={<ContentCopyIcon />}
                onClick={() => {
                  navigator.clipboard.writeText(getClineConfig());
                  snackbarSuccess('Configuration copied to clipboard');
                }}
                sx={{ textTransform: 'none' }}
              >
                Copy
              </Button>
            </Box>
          </>
        );
      case 'continue':
        return (
          <Typography variant="body1">
            Continue.dev integration coming soon...
          </Typography>
        );
      case 'claude':
        return (
          <>
            <Typography variant="body1" sx={{ mb: 2 }}>
              Follow these steps to configure <Link href="https://claude.ai/download" target="_blank" rel="noopener noreferrer">Claude Desktop</Link>:
            </Typography>
            <Box sx={{ mb: 2 }}>
              <Typography variant="body2" sx={{ mb: 1 }}>1. Open Claude Desktop settings</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>2. Select "Developer" section</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>3. Click "Edit Config"</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>4. Add the following configuration:</Typography>
            </Box>
            <TextField
              value={getClaudeDesktopConfig()}
              fullWidth
              multiline
              disabled
              InputProps={{
                style: { fontFamily: 'monospace' }
              }}
            />
            <Box sx={{ textAlign: 'right', mb: 1 }}>
              <Button
                variant="text"
                size="small"
                startIcon={<ContentCopyIcon />}
                onClick={() => {
                  navigator.clipboard.writeText(getClaudeDesktopConfig());
                  snackbarSuccess('Configuration copied to clipboard');
                }}
                sx={{ textTransform: 'none' }}
              >
                Copy
              </Button>
            </Box>
            <Typography variant="body1" sx={{ mb: 2 }}>
              You can find additional documentation in <Link href="https://modelcontextprotocol.io/quickstart/user" target="_blank" rel="noopener noreferrer">Model Context Protocol quickstart</Link>
            </Typography>
          </>
        );
      default:
        return null;
    }
  };

  return (
    <Box sx={{ mt: 4 }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        IDE Integration
      </Typography>
      <Typography variant="body1" sx={{ mb: 2 }}> 
        Helix provides an MCP server that can be used by a number of IDEs, chat clients or standalone agents.
      </Typography>
      <FormControl fullWidth sx={{ mb: 3 }}>
        <InputLabel>Select IDE</InputLabel>
        <Select
          value={selectedIde}
          label="Select IDE"
          onChange={(e) => setSelectedIde(e.target.value)}
        >
          <MenuItem value="cline">Cline</MenuItem>
          <MenuItem value="continue">Continue.dev</MenuItem>
          <MenuItem value="claude">Claude Desktop</MenuItem>
        </Select>
      </FormControl>
      {renderIdeInstructions()}
    </Box>
  );
};

export default IdeIntegrationSection;
