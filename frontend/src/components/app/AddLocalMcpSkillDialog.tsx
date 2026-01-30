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

  const [skill, setSkill] = useState<TypesAssistantMCP>({
    name: '',
    description: '',
    transport: 'stdio',
    command: '',
    args: [],
    env: {},
  });

  const [existingSkillIndex, setExistingSkillIndex] = useState<number | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (initialSkill) {
      setSkill({
        name: initialSkill.name || '',
        description: initialSkill.description || '',
        transport: 'stdio',
        command: initialSkill.command || '',
        args: initialSkill.args || [],
        env: initialSkill.env || {},
      });

      // Find existing skill in app.mcpTools
      const existingIndex = app.mcpTools?.findIndex(
        mcp => mcp.name === initialSkill.name && mcp.transport === 'stdio'
      ) ?? -1;
      if (existingIndex !== -1) {
        setExistingSkillIndex(existingIndex);
      }
    } else {
      // Reset form when opening for new skill
      setSkill({
        name: '',
        description: '',
        transport: 'stdio',
        command: '',
        args: [],
        env: {},
      });
      setExistingSkillIndex(null);
    }
  }, [initialSkill, open, app.mcpTools]);

  const handleChange = (field: keyof TypesAssistantMCP, value: string) => {
    setSkill((prev) => ({
      ...prev,
      [field]: value,
    }));
  };

  const handleArgChange = (index: number, value: string) => {
    const newArgs = [...(skill.args || [])];
    newArgs[index] = value;
    setSkill((prev) => ({
      ...prev,
      args: newArgs,
    }));
  };

  const addArg = () => {
    setSkill((prev) => ({
      ...prev,
      args: [...(prev.args || []), ''],
    }));
  };

  const removeArg = (index: number) => {
    const newArgs = [...(skill.args || [])];
    newArgs.splice(index, 1);
    setSkill((prev) => ({
      ...prev,
      args: newArgs,
    }));
  };

  const handleEnvChange = (key: string, value: string) => {
    const newEnv = { ...(skill.env || {}) };
    if (value === '') {
      delete newEnv[key];
    } else {
      newEnv[key] = value;
    }
    setSkill((prev) => ({
      ...prev,
      env: newEnv,
    }));
  };

  const handleEnvKeyChange = (oldKey: string, newKey: string, value: string) => {
    const newEnv = { ...(skill.env || {}) };
    delete newEnv[oldKey];
    newEnv[newKey] = value;
    setSkill((prev) => ({
      ...prev,
      env: newEnv,
    }));
  };

  const addEnv = () => {
    setSkill((prev) => ({
      ...prev,
      env: { ...(prev.env || {}), '': '' },
    }));
  };

  const removeEnv = (key: string) => {
    const newEnv = { ...(skill.env || {}) };
    delete newEnv[key];
    setSkill((prev) => ({
      ...prev,
      env: newEnv,
    }));
  };

  const handleSave = async () => {
    try {
      setError(null);

      // Validate required fields
      if (!skill.name?.trim()) {
        setError('Name is required');
        return;
      }
      if (!skill.command?.trim()) {
        setError('Command is required');
        return;
      }

      // Construct the MCP skill object
      const mcpSkill: TypesAssistantMCP = {
        name: skill.name,
        description: skill.description,
        transport: 'stdio',
        command: skill.command,
        args: (skill.args || []).filter(a => a.trim() !== ''),
        env: skill.env,
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
          setSkill({
            name: '',
            description: '',
            transport: 'stdio',
            command: '',
            args: [],
            env: {},
          });
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
          <NameTypography>{skill.name || 'New Local MCP Server'}</NameTypography>

          <Typography variant="body2" sx={{ color: '#A0AEC0', mb: 3 }}>
            Configure a local MCP server that runs inside the dev container. Use this for stdio-based MCPs like npx packages.
          </Typography>

          <SectionCard>
            <DarkTextField
              fullWidth
              label="Name"
              value={skill.name}
              helperText="A unique name for this MCP server"
              onChange={(e) => handleChange('name', e.target.value)}
              margin="normal"
              required
            />

            <DarkTextField
              fullWidth
              label="Description"
              value={skill.description}
              helperText="Optional description of what this MCP server does"
              onChange={(e) => handleChange('description', e.target.value)}
              margin="normal"
              multiline
              rows={2}
            />

            <DarkTextField
              fullWidth
              label="Command"
              value={skill.command}
              helperText="The command to run (e.g., 'npx', 'node', 'python')"
              onChange={(e) => handleChange('command', e.target.value)}
              margin="normal"
              required
            />

            {/* Arguments Section */}
            <Box sx={{ mt: 3 }}>
              <Typography variant="h6" sx={{ mb: 2, color: '#F8FAFC' }}>
                Command Arguments
              </Typography>
              <Typography variant="body2" sx={{ color: '#A0AEC0', mb: 2 }}>
                Arguments to pass to the command (e.g., for npx: "-y", "@example/mcp-server")
              </Typography>
              <List>
                {(skill.args || []).map((arg, index) => (
                  <ListItem key={`arg-${index}`} sx={{ px: 0 }}>
                    <Grid container spacing={1} alignItems="center">
                      <Grid item xs={10}>
                        <DarkTextField
                          size="small"
                          placeholder={`Argument ${index + 1}`}
                          value={arg}
                          onChange={(e) => handleArgChange(index, e.target.value)}
                          fullWidth
                        />
                      </Grid>
                      <Grid item xs={2}>
                        <IconButton size="small" onClick={() => removeArg(index)} sx={{ color: '#F87171' }}>
                          <DeleteIcon />
                        </IconButton>
                      </Grid>
                    </Grid>
                  </ListItem>
                ))}
              </List>
              <Button startIcon={<AddIcon />} onClick={addArg} size="small" sx={{ mt: 1 }}>
                Add Argument
              </Button>
            </Box>

            {/* Environment Variables Section */}
            <Box sx={{ mt: 3 }}>
              <Typography variant="h6" sx={{ mb: 2, color: '#F8FAFC' }}>
                Environment Variables
              </Typography>
              <Typography variant="body2" sx={{ color: '#A0AEC0', mb: 2 }}>
                Environment variables to set for the MCP process. Use project secrets by referencing them here.
              </Typography>
              <List>
                {Object.entries(skill.env || {}).map(([key, value], index) => (
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

          {/* Example Section */}
          <SectionCard>
            <Typography variant="h6" sx={{ mb: 2, color: '#F8FAFC' }}>
              Example: Drone CI MCP Server
            </Typography>
            <Typography variant="body2" sx={{ color: '#A0AEC0', mb: 2 }}>
              Access Drone CI build logs and info. Configure credentials via environment variables:
            </Typography>
            <Box
              sx={{
                bgcolor: '#1A1D24',
                borderRadius: 1,
                p: 2,
                fontFamily: 'monospace',
                fontSize: '0.875rem',
                color: '#10B981',
              }}
            >
              <div>Command: npx</div>
              <div>Arguments: -y, drone-ci-mcp</div>
              <div>Environment:</div>
              <div style={{ marginLeft: '1rem' }}>DRONE_SERVER_URL=https://drone.example.com</div>
              <div style={{ marginLeft: '1rem' }}>DRONE_ACCESS_TOKEN=your-drone-api-token</div>
            </Box>
            <Typography variant="body2" sx={{ color: '#A0AEC0', mt: 2 }}>
              <strong>Security tip:</strong> Use environment variables for secrets instead of CLI arguments.
              Tokens in args may be visible in process listings.
            </Typography>
            <Typography variant="body2" sx={{ color: '#A0AEC0', mt: 1 }}>
              <strong>Version pinning:</strong> For stability, pin versions (e.g., <code>drone-ci-mcp@0.0.3</code>).
            </Typography>
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
              disabled={!skill.name?.trim() || !skill.command?.trim()}
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
