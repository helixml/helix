import React, { useState, useEffect } from 'react';
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
import useAccount from '../../hooks/useAccount'
import { useGetUserAPIKeys } from '../../services/userService'

interface IdeIntegrationSectionProps {
  appId: string;
}

const IdeIntegrationSection: React.FC<IdeIntegrationSectionProps> = ({
  appId,
}) => {
  const account = useAccount()

  const [selectedIde, setSelectedIde] = useState<string>('cline');
  const { success: snackbarSuccess } = useSnackbar();

  const { data: apiKeys, isLoading: isLoadingApiKeys } = useGetUserAPIKeys()

  const apiKey = apiKeys?.length && apiKeys.length > 0 ? apiKeys[0].key : ''


  const getGenericMCPConfig = () => {
    const serverUrl = window.location.origin;
    return `{
  "mcpServers": {
    "helix-mcp": {
      "command": "/usr/local/bin/helix",
      "args": [
        "mcp",
        "run",
        "--app-id", "${appId}",
        "--api-key", "${apiKey}",
        "--url", "${serverUrl}"
      ]
    }
  }
}`;
  };

  const renderIdeInstructions = () => {
    var installCliInstructions = <>
      <div>
        <Typography variant="body2" sx={{ mb: 1 }}>1. Download Helix client: </Typography>
        <Box sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 2,
          mb: 2
        }}>
          <Typography component="pre"
            sx={{
              wordBreak: 'break-all',
              wordWrap: 'break-all',
              whiteSpace: 'pre-wrap',
              fontSize: '0.8rem',
              fontFamily: "monospace",
              flexGrow: 1,
              mb: 0,
              mt: 0,
            }}
          >
            curl -Ls -O https://get.helixml.tech/install.sh && bash install.sh --cli
          </Typography>
          <Button
            variant="text"
            size="small"
            startIcon={<ContentCopyIcon />}
            onClick={() => {
              navigator.clipboard.writeText("curl -Ls -O https://get.helixml.tech/install.sh && bash install.sh --cli");
              snackbarSuccess('Command copied to clipboard');
            }}
            sx={{ textTransform: 'none', flexShrink: 0 }}
          >
            Copy
          </Button>
        </Box>
      </div>
    </>
    switch (selectedIde) {
      case 'cline':
        return (
          <>
            <Typography variant="body1" sx={{ mb: 2 }}>
              Follow these steps to configure <Link href="https://cline.bot/" target="_blank" rel="noopener noreferrer">Cline</Link>:
            </Typography>
            <Box sx={{ mb: 2 }}>
              {installCliInstructions}
              <Typography variant="body2" sx={{ mb: 1 }}>2. Open the command palette (CMD+Shift+P)</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>3. Type "MCP Servers"</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>4. Add the following configuration:</Typography>
            </Box>
            <TextField
              value={getGenericMCPConfig()}
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
                  navigator.clipboard.writeText(getGenericMCPConfig());
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
              {installCliInstructions}
              <Typography variant="body2" sx={{ mb: 1 }}>2. Open Claude Desktop settings</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>3. Select "Developer" section</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>4. Click "Edit Config"</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>4. Add the following configuration:</Typography>
            </Box>
            <TextField
              value={getGenericMCPConfig()}
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
                  navigator.clipboard.writeText(getGenericMCPConfig());
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
      case 'cursor':
        return (
          <>
            <Typography variant="body1" sx={{ mb: 2 }}>
              Follow these steps to configure <Link href="https://www.cursor.com/" target="_blank" rel="noopener noreferrer">Cursor</Link>:
            </Typography>
            <Box sx={{ mb: 2 }}>
              {installCliInstructions}
              <Typography variant="body2" sx={{ mb: 1 }}>2. For a specific project open `.cursor/mcp.json` file, for global configuration open `~/.cursor/mcp.json`</Typography>
              <Typography variant="body2" sx={{ mb: 1 }}>3. Add the following configuration:</Typography>
            </Box>
            <TextField
              value={getGenericMCPConfig()}
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
                  navigator.clipboard.writeText(getGenericMCPConfig());
                  snackbarSuccess('Configuration copied to clipboard');
                }}
                sx={{ textTransform: 'none' }}
              >
                Copy
              </Button>
            </Box>
            <Typography variant="body1" sx={{ mb: 2 }}>
              You can find additional information in the official <Link href="https://docs.cursor.com/context/model-context-protocol" target="_blank" rel="noopener noreferrer">Model Context Protocol</Link> documentation.
            </Typography>
          </>
        );
      default:
        return null;
    }
  };

  return (
    <Box sx={{ mt: 2, mr: 2 }}>
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
          <MenuItem value="cursor">Cursor</MenuItem>
        </Select>
      </FormControl>
      {renderIdeInstructions()}
    </Box>
  );
};

export default IdeIntegrationSection;
