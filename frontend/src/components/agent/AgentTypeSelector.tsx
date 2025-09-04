import React from 'react';
import {
  Box,
  FormControl,
  FormControlLabel,
  Radio,
  RadioGroup,
  Typography,
  Card,
  CardContent,
  Collapse,
  TextField,
  Chip,
  IconButton,
  Tooltip,
  Alert,
} from '@mui/material';
import {
  Chat as ChatIcon,
  Code as CodeIcon,
  Computer as ComputerIcon,
  AutoAwesome as AutoAwesomeIcon,
  Info as InfoIcon,
  Add as AddIcon,
  Delete as DeleteIcon,
} from '@mui/icons-material';
import { 
  IAgentType, 
  IExternalAgentConfig,
  AGENT_TYPE_HELIX_BASIC,
  AGENT_TYPE_HELIX_AGENT,
  AGENT_TYPE_ZED_EXTERNAL,
  AGENT_TYPE_OPTIONS 
} from '../../types';

interface AgentTypeSelectorProps {
  value: IAgentType;
  onChange: (agentType: IAgentType, config?: IExternalAgentConfig) => void;
  externalAgentConfig?: IExternalAgentConfig;
  disabled?: boolean;
  showExternalConfig?: boolean;
  size?: 'small' | 'medium';
}

const AgentTypeSelector: React.FC<AgentTypeSelectorProps> = ({
  value,
  onChange,
  externalAgentConfig = {},
  disabled = false,
  showExternalConfig = true,
  size = 'medium',
}) => {
  const [localConfig, setLocalConfig] = React.useState<IExternalAgentConfig>(externalAgentConfig);

  const handleAgentTypeChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const newAgentType = event.target.value as IAgentType;
    onChange(newAgentType, newAgentType === AGENT_TYPE_ZED_EXTERNAL ? localConfig : undefined);
  };

  const handleConfigChange = (newConfig: IExternalAgentConfig) => {
    setLocalConfig(newConfig);
    if (value === AGENT_TYPE_ZED_EXTERNAL) {
      onChange(value, newConfig);
    }
  };

  const handleEnvVarAdd = () => {
    const newEnvVars = [...(localConfig.env_vars || []), ''];
    handleConfigChange({ ...localConfig, env_vars: newEnvVars });
  };

  const handleEnvVarChange = (index: number, value: string) => {
    const newEnvVars = [...(localConfig.env_vars || [])];
    newEnvVars[index] = value;
    handleConfigChange({ ...localConfig, env_vars: newEnvVars });
  };

  const handleEnvVarDelete = (index: number) => {
    const newEnvVars = (localConfig.env_vars || []).filter((_, i) => i !== index);
    handleConfigChange({ ...localConfig, env_vars: newEnvVars });
  };

  const getAgentIcon = (agentType: IAgentType) => {
    switch (agentType) {
      case AGENT_TYPE_HELIX_BASIC:
        return <ChatIcon />;
      case AGENT_TYPE_HELIX_AGENT:
        return <AutoAwesomeIcon />;
      case AGENT_TYPE_ZED_EXTERNAL:
        return <CodeIcon />;
      default:
        return <ComputerIcon />;
    }
  };

  return (
    <Box>
      <FormControl component="fieldset" disabled={disabled} fullWidth>
        <RadioGroup
          value={value}
          onChange={handleAgentTypeChange}
          name="agent-type-selector"
        >
          {AGENT_TYPE_OPTIONS.map((option) => (
            <Card 
              key={option.value}
              variant="outlined"
              sx={{
                mb: 2,
                border: value === option.value ? 2 : 1,
                borderColor: value === option.value ? 'primary.main' : 'grey.300',
                transition: 'all 0.2s ease-in-out',
                '&:hover': {
                  borderColor: 'primary.main',
                  boxShadow: 1,
                },
              }}
            >
              <CardContent sx={{ p: size === 'small' ? 2 : 3 }}>
                <FormControlLabel
                  value={option.value}
                  control={<Radio />}
                  label={
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, width: '100%' }}>
                      {getAgentIcon(option.value)}
                      <Box sx={{ flexGrow: 1 }}>
                        <Typography 
                          variant={size === 'small' ? 'body1' : 'h6'} 
                          component="div"
                          sx={{ fontWeight: 'medium' }}
                        >
                          {option.label}
                        </Typography>
                        <Typography 
                          variant={size === 'small' ? 'caption' : 'body2'} 
                          color="text.secondary"
                          sx={{ mt: 0.5 }}
                        >
                          {option.description}
                        </Typography>
                      </Box>
                      {option.value === AGENT_TYPE_ZED_EXTERNAL && (
                        <Tooltip title="External agents provide full development environments with code editing capabilities via RDP">
                          <InfoIcon color="action" fontSize="small" />
                        </Tooltip>
                      )}
                    </Box>
                  }
                  disabled={option.disabled}
                  sx={{ 
                    margin: 0,
                    width: '100%',
                    '& .MuiFormControlLabel-label': {
                      width: '100%',
                    },
                  }}
                />
              </CardContent>
            </Card>
          ))}
        </RadioGroup>
      </FormControl>

      {/* External Agent Configuration */}
      {showExternalConfig && (
        <Collapse in={value === AGENT_TYPE_ZED_EXTERNAL}>
          <Card variant="outlined" sx={{ mt: 2, backgroundColor: 'background.paper', border: '1px solid', borderColor: 'divider' }}>
            <CardContent sx={{ color: 'text.primary' }}>
              <Typography variant="h6" gutterBottom sx={{ display: 'flex', alignItems: 'center', gap: 1, color: 'text.primary' }}>
                <CodeIcon />
                External Agent Configuration
              </Typography>
              
              <Box sx={{ display: 'flex', flexDirection: 'column', gap: 2 }}>
                <TextField
                  label="Project Path"
                  placeholder="my-project"
                  size="small"
                  fullWidth
                  value={localConfig.project_path || ''}
                  onChange={(e) => handleConfigChange({ ...localConfig, project_path: e.target.value })}
                  helperText="Relative path for the project directory"
                />
                
                <TextField
                  label="Workspace Directory"
                  placeholder="/workspace/custom-path"
                  size="small"
                  fullWidth
                  value={localConfig.workspace_dir || ''}
                  onChange={(e) => handleConfigChange({ ...localConfig, workspace_dir: e.target.value })}
                  helperText="Custom working directory (optional)"
                />

                {/* Environment Variables */}
                <Box>
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                    <Typography variant="subtitle2" sx={{ color: 'text.primary' }}>Environment Variables</Typography>
                    <IconButton size="small" onClick={handleEnvVarAdd}>
                      <AddIcon />
                    </IconButton>
                  </Box>
                  
                  {(localConfig.env_vars || []).map((envVar, index) => (
                    <Box key={index} sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                      <TextField
                        placeholder="KEY=value"
                        size="small"
                        fullWidth
                        value={envVar}
                        onChange={(e) => handleEnvVarChange(index, e.target.value)}
                      />
                      <IconButton size="small" onClick={() => handleEnvVarDelete(index)}>
                        <DeleteIcon />
                      </IconButton>
                    </Box>
                  ))}
                  
                  {(!localConfig.env_vars || localConfig.env_vars.length === 0) && (
                    <Typography variant="caption" color="text.secondary">
                      No environment variables configured
                    </Typography>
                  )}
                </Box>

                <Alert severity="info" sx={{ mt: 1 }}>
                  <Typography variant="body2">
                    External agents run in isolated containers with RDP access for visual development.
                    You can connect to watch the agent work in real-time.
                  </Typography>
                </Alert>
              </Box>
            </CardContent>
          </Card>
        </Collapse>
      )}
    </Box>
  );
};

export default AgentTypeSelector;