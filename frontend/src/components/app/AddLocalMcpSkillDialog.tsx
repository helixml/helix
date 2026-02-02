import React, { useState, useEffect } from 'react';
import { AxiosError } from 'axios';
import {
  DialogContent,
  DialogActions,
  Button,
  TextField,
  Box,
  Typography,
  IconButton,
  List,
  ListItem,
  Grid,
  Alert,
} from '@mui/material';
import DeleteIcon from '@mui/icons-material/Delete';
import AddIcon from '@mui/icons-material/Add';
import { IAppFlatState } from '../../types';
import { styled } from '@mui/material/styles';
import DarkDialog from '../dialog/DarkDialog';
import useLightTheme from '../../hooks/useLightTheme';
import { TypesAssistantMCP } from '../../api/api';

interface AddLocalMcpSkillDialogProps {
  open: boolean;
  onClose: () => void;
  onClosed?: () => void;
  skill?: TypesAssistantMCP;
  app: IAppFlatState;
  onUpdate: (updates: IAppFlatState) => Promise<void>;
  isEnabled: boolean;
}

const NameTypography = styled(Typography)(({ theme }) => ({
  fontSize: '2rem',
  fontWeight: 700,
  color: '#F8FAFC',
  marginBottom: theme.spacing(1),
}));

const SectionCard = styled(Box)(({ theme }) => ({
  background: '#23262F',
  borderRadius: 12,
  padding: theme.spacing(3),
  marginBottom: theme.spacing(3),
  boxShadow: '0 2px 8px rgba(0,0,0,0.15)',
}));

const DarkTextField = styled(TextField)(({ theme }) => ({
  '& .MuiInputBase-root': {
    background: '#23262F',
    color: '#F1F1F1',
    borderRadius: 8,
  },
  '& .MuiInputLabel-root': {
    color: '#A0AEC0',
  },
  '& .MuiOutlinedInput-notchedOutline': {
    borderColor: '#353945',
  },
  '&:hover .MuiOutlinedInput-notchedOutline': {
    borderColor: '#6366F1',
  },
}));

const AddLocalMcpSkillDialog: React.FC<AddLocalMcpSkillDialogProps> = ({
  open,
  onClose,
  onClosed,
  skill: initialSkill,
  app,
  onUpdate,
  isEnabled: initialIsEnabled,
}) => {
  const lightTheme = useLightTheme();

  const [name, setName] = useState('');
  const [description, setDescription] = useState('');
  const [commandLine, setCommandLine] = useState(''); // Single field for "command arg1 arg2..."
  const [env, setEnv] = useState<Record<string, string>>({});

  const [existingSkillIndex, setExistingSkillIndex] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  // Parse command line into command and args, handling quoted strings
  const parseCommandLine = (cmdLine: string): { command: string; args: string[] } => {
    const parts: string[] = [];
    let current = '';
    let inQuote = false;
    let quoteChar = '';

    for (let i = 0; i < cmdLine.length; i++) {
      const char = cmdLine[i];

      if (!inQuote && (char === '"' || char === "'")) {
        // Start of quoted string
        inQuote = true;
        quoteChar = char;
      } else if (inQuote && char === quoteChar) {
        // End of quoted string
        inQuote = false;
        quoteChar = '';
      } else if (!inQuote && /\s/.test(char)) {
        // Space outside quotes - end current part
        if (current) {
          parts.push(current);
          current = '';
        }
      } else {
        current += char;
      }
    }

    // Don't forget the last part
    if (current) {
      parts.push(current);
    }

    if (parts.length === 0) return { command: '', args: [] };
    return { command: parts[0], args: parts.slice(1) };
  };

  // Join command and args into a single string for display
  const joinCommandLine = (command: string, args: string[]): string => {
    if (!command) return '';
    if (!args || args.length === 0) return command;
    return `${command} ${args.join(' ')}`;
  };

  useEffect(() => {
    if (initialSkill) {
      setName(initialSkill.name || '');
      setDescription(initialSkill.description || '');
      setCommandLine(joinCommandLine(initialSkill.command || '', initialSkill.args || []));
      setEnv(initialSkill.env || {});

      // Find existing skill in app.mcpTools
      const existingIndex = app.mcpTools?.findIndex(
        mcp => mcp.name === initialSkill.name && mcp.transport === 'stdio'
      ) ?? -1;
      if (existingIndex !== -1) {
        setExistingSkillIndex(existingIndex);
      }
    } else {
      // Reset form when opening for new skill
      setName('');
      setDescription('');
      setCommandLine('');
      setEnv({});
      setExistingSkillIndex(null);
    }
  }, [initialSkill, open, app.mcpTools]);

  const handleEnvChange = (key: string, value: string) => {
    const newEnv = { ...env };
    if (value === '') {
      delete newEnv[key];
    } else {
      newEnv[key] = value;
    }
    setEnv(newEnv);
  };

  const handleEnvKeyChange = (oldKey: string, newKey: string, value: string) => {
    const newEnv = { ...env };
    delete newEnv[oldKey];
    newEnv[newKey] = value;
    setEnv(newEnv);
  };

  const addEnv = () => {
    setEnv({ ...env, '': '' });
  };

  const removeEnv = (key: string) => {
    const newEnv = { ...env };
    delete newEnv[key];
    setEnv(newEnv);
  };

  const handleSave = async () => {
    try {
      setError(null);

      // Validate required fields
      if (!name.trim()) {
        setError('Name is required');
        return;
      }
      if (!commandLine.trim()) {
        setError('Command is required');
        return;
      }

      // Parse command line into command and args
      const { command, args } = parseCommandLine(commandLine);

      // Construct the MCP skill object
      const mcpSkill: TypesAssistantMCP = {
        name: name.trim(),
        description: description.trim(),
        transport: 'stdio',
        command,
        args,
        env,
      };

      // Copy app object
      const appCopy = JSON.parse(JSON.stringify(app));

      // Initialize mcpTools array if it doesn't exist
      if (!appCopy.mcpTools) {
        appCopy.mcpTools = [];
      }

      // Update or add the skill
      if (existingSkillIndex !== null) {
        appCopy.mcpTools[existingSkillIndex] = mcpSkill;
      } else {
        appCopy.mcpTools.push(mcpSkill);
      }

      await onUpdate(appCopy);
      onClose();
    } catch (err) {
      console.log(err);
      const axiosError = err as AxiosError;
      const errMessage = axiosError.response?.data
        ? JSON.stringify(axiosError.response.data)
        : axiosError.message || 'Failed to save skill';
      setError(errMessage);
    }
  };

  const handleDisable = async () => {
    if (existingSkillIndex !== null) {
      const appCopy = JSON.parse(JSON.stringify(app));
      appCopy.mcpTools = appCopy.mcpTools?.filter((_: TypesAssistantMCP, index: number) => index !== existingSkillIndex);
      await onUpdate(appCopy);
    }
    onClose();
  };

  const handleClose = () => {
    onClose();
  };

  return (
    <DarkDialog
      open={open}
      onClose={handleClose}
      maxWidth="md"
      fullWidth
      PaperProps={{
        sx: {
          height: '80vh',
          maxHeight: '700px',
          minHeight: '500px',
        },
      }}
      TransitionProps={{
        onExited: () => {
          setName('');
          setDescription('');
          setCommandLine('');
          setEnv({});
          setExistingSkillIndex(null);
          setError(null);
          onClosed?.();
        },
      }}
    >
      <DialogContent sx={{ ...lightTheme.scrollbar, height: '100%', display: 'flex', flexDirection: 'column' }}>
        <Box
          sx={{
            mt: 2,
            flex: 1,
            display: 'flex',
            flexDirection: 'column',
            overflow: 'auto',
          }}
        >
          <NameTypography>{name || 'New Local MCP Server'}</NameTypography>

          <Typography variant="body2" sx={{ color: '#A0AEC0', mb: 3 }}>
            Configure a local MCP server that runs inside the dev container.
          </Typography>

          {/* Examples Section - at top for easy copy/paste */}
          <SectionCard>
            <Typography variant="h6" sx={{ mb: 2, color: '#F8FAFC' }}>
              Examples
            </Typography>
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 1 }}>
              <Box
                sx={{
                  bgcolor: '#1A1D24',
                  borderRadius: 1,
                  p: 1.5,
                  fontFamily: 'monospace',
                  fontSize: '0.875rem',
                  color: '#10B981',
                  cursor: 'pointer',
                  '&:hover': { bgcolor: '#1E2129' },
                }}
                onClick={() => {
                  setName('Drone CI');
                  setCommandLine('npx -y drone-ci-mcp');
                  setEnv({
                    'DRONE_SERVER_URL': 'https://drone.example.com',
                    'DRONE_ACCESS_TOKEN': 'your-drone-api-token',
                  });
                }}
              >
                npx -y drone-ci-mcp
              </Box>
              <Box
                sx={{
                  bgcolor: '#1A1D24',
                  borderRadius: 1,
                  p: 1.5,
                  fontFamily: 'monospace',
                  fontSize: '0.875rem',
                  color: '#10B981',
                  cursor: 'pointer',
                  '&:hover': { bgcolor: '#1E2129' },
                }}
                onClick={() => setCommandLine('npx -y @anthropic/mcp-server-filesystem /workspace')}
              >
                npx -y @anthropic/mcp-server-filesystem /workspace
              </Box>
              <Box
                sx={{
                  bgcolor: '#1A1D24',
                  borderRadius: 1,
                  p: 1.5,
                  fontFamily: 'monospace',
                  fontSize: '0.875rem',
                  color: '#10B981',
                  cursor: 'pointer',
                  '&:hover': { bgcolor: '#1E2129' },
                }}
                onClick={() => setCommandLine('uvx mcp-server-git --repository /workspace')}
              >
                uvx mcp-server-git --repository /workspace
              </Box>
            </Box>
            <Typography variant="body2" sx={{ color: '#A0AEC0', mt: 2 }}>
              Click an example to use it as a starting point.
            </Typography>
          </SectionCard>

          {/* Configuration Section */}
          <SectionCard>
            <DarkTextField
              fullWidth
              label="Name"
              value={name}
              helperText="A unique name for this MCP server"
              onChange={(e) => setName(e.target.value)}
              margin="normal"
              required
            />

            <DarkTextField
              fullWidth
              label="Command"
              value={commandLine}
              helperText={'The full command to run. Use quotes for arguments with spaces, e.g., npx -y my-mcp "arg with spaces"'}
              onChange={(e) => setCommandLine(e.target.value)}
              margin="normal"
              required
              placeholder="npx -y drone-ci-mcp"
            />

            {/* Preview how command is parsed */}
            {commandLine.trim() && (
              <Box
                sx={{
                  mt: 1,
                  p: 1.5,
                  bgcolor: '#1A1D24',
                  borderRadius: 1,
                  fontFamily: 'monospace',
                  fontSize: '0.8rem',
                }}
              >
                <Typography variant="caption" sx={{ color: '#A0AEC0', display: 'block', mb: 0.5 }}>
                  Parsed as:
                </Typography>
                <Box sx={{ color: '#10B981' }}>
                  <div>
                    <span style={{ color: '#A0AEC0' }}>command:</span> {parseCommandLine(commandLine).command}
                  </div>
                  <div>
                    <span style={{ color: '#A0AEC0' }}>args:</span> [{parseCommandLine(commandLine).args.map((a, i) => (
                      <span key={i}>
                        {i > 0 && ', '}
                        "{a}"
                      </span>
                    ))}]
                  </div>
                </Box>
              </Box>
            )}

            <DarkTextField
              fullWidth
              label="Description"
              value={description}
              helperText="Optional description of what this MCP server does"
              onChange={(e) => setDescription(e.target.value)}
              margin="normal"
              multiline
              rows={2}
            />

            {/* Environment Variables Section */}
            <Box sx={{ mt: 3 }}>
              <Typography variant="h6" sx={{ mb: 2, color: '#F8FAFC' }}>
                Environment Variables
              </Typography>
              <Typography variant="body2" sx={{ color: '#A0AEC0', mb: 2 }}>
                Environment variables for the MCP process (e.g., API tokens). Use this instead of passing secrets in the command.
              </Typography>
              <List>
                {Object.entries(env).map(([key, value], index) => (
                  <ListItem key={`env-${index}`} sx={{ px: 0 }}>
                    <Grid container spacing={1}>
                      <Grid item xs={5}>
                        <DarkTextField
                          size="small"
                          placeholder="Variable Name"
                          value={key}
                          onChange={(e) => handleEnvKeyChange(key, e.target.value, value)}
                          fullWidth
                        />
                      </Grid>
                      <Grid item xs={5}>
                        <DarkTextField
                          size="small"
                          placeholder="Value"
                          value={value}
                          onChange={(e) => handleEnvChange(key, e.target.value)}
                          fullWidth
                        />
                      </Grid>
                      <Grid item xs={2}>
                        <IconButton size="small" onClick={() => removeEnv(key)} sx={{ color: '#F87171' }}>
                          <DeleteIcon />
                        </IconButton>
                      </Grid>
                    </Grid>
                  </ListItem>
                ))}
              </List>
              <Button startIcon={<AddIcon />} onClick={addEnv} size="small" sx={{ mt: 1 }}>
                Add Variable
              </Button>
            </Box>
          </SectionCard>
        </Box>
      </DialogContent>
      <DialogActions
        sx={{
          background: '#181A20',
          borderTop: '1px solid #23262F',
          flexDirection: 'column',
          alignItems: 'stretch',
        }}
      >
        {error && (
          <Box sx={{ width: '100%', pl: 2, pr: 2, mb: 3 }}>
            <Alert variant="outlined" severity="error" sx={{ width: '100%' }}>
              {error}
            </Alert>
          </Box>
        )}
        <Box sx={{ display: 'flex', width: '100%' }}>
          <Button onClick={handleClose} size="small" variant="outlined" color="primary">
            Close
          </Button>
          <Box sx={{ flex: 1 }} />
          <Box sx={{ display: 'flex', gap: 1, mr: 2 }}>
            {existingSkillIndex !== null && (
              <Button
                onClick={handleDisable}
                size="small"
                variant="outlined"
                color="error"
                sx={{
                  borderColor: '#EF4444',
                  color: '#EF4444',
                  '&:hover': { borderColor: '#DC2626', color: '#DC2626' },
                }}
              >
                Remove
              </Button>
            )}
            <Button
              onClick={handleSave}
              size="small"
              variant="outlined"
              color="secondary"
              disabled={!name.trim() || !commandLine.trim()}
            >
              {existingSkillIndex !== null ? 'Save' : 'Add'}
            </Button>
          </Box>
        </Box>
      </DialogActions>
    </DarkDialog>
  );
};

export default AddLocalMcpSkillDialog;
