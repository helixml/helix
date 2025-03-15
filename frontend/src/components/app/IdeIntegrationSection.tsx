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

interface IdeIntegrationSectionProps {
  appId: string;
  apiKey: string;
}

const IdeIntegrationSection: React.FC<IdeIntegrationSectionProps> = ({
  appId,
  apiKey,
}) => {
  const [selectedIde, setSelectedIde] = useState<string>('cline');

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
              rows={10}
              disabled
              InputProps={{
                style: { fontFamily: 'monospace' }
              }}
            />
            <Box sx={{ textAlign: 'right', mb: 1 }}>
              <JsonWindowLink
                sx={{textDecoration: 'underline'}}
                data={getClineConfig()}
              >
                expand
              </JsonWindowLink>
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
          <Typography variant="body1">
            Claude Desktop integration coming soon...
          </Typography>
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
