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

  // Sync localConfig with externalAgentConfig prop changes
  React.useEffect(() => {
    setLocalConfig(externalAgentConfig);
  }, [externalAgentConfig]);

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
                <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                  Zed agents will automatically clone git repositories from the Helix server into their workspace. 
                  No manual path configuration needed.
                </Typography>

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

                {/* Video Resolution Settings (Phase 3.5) */}
                <Box>
                  <Typography variant="subtitle2" sx={{ color: 'text.primary', mb: 1 }}>
                    Streaming Resolution
                  </Typography>

                  <FormControl fullWidth size="small" sx={{ mb: 1 }}>
                    <select
                      value={
                        localConfig.display_width === 2560 && localConfig.display_height === 1600 ? 'macbook-13' :
                        localConfig.display_width === 3456 && localConfig.display_height === 2234 ? 'macbook-16' :
                        localConfig.display_width === 2880 && localConfig.display_height === 1864 ? 'macbook-15' :
                        localConfig.display_width === 5120 && localConfig.display_height === 2880 ? '5k' :
                        localConfig.display_width === 3840 && localConfig.display_height === 2160 ? '4k' :
                        localConfig.display_width === 1920 && localConfig.display_height === 1080 ? 'fhd' :
                        localConfig.display_width === 1179 && localConfig.display_height === 2496 ? 'iphone-15-pro' :
                        'custom'
                      }
                      onChange={(e) => {
                        const presets: Record<string, {width: number, height: number, refresh: number}> = {
                          'macbook-13': { width: 2560, height: 1600, refresh: 60 },
                          'macbook-16': { width: 3456, height: 2234, refresh: 120 },
                          'macbook-15': { width: 2880, height: 1864, refresh: 60 },
                          '5k': { width: 5120, height: 2880, refresh: 60 },
                          '4k': { width: 3840, height: 2160, refresh: 60 },
                          'fhd': { width: 1920, height: 1080, refresh: 60 },
                          'iphone-15-pro': { width: 1179, height: 2496, refresh: 120 },
                        };
                        const preset = presets[e.target.value];
                        if (preset) {
                          handleConfigChange({
                            ...localConfig,
                            display_width: preset.width,
                            display_height: preset.height,
                            display_refresh_rate: preset.refresh
                          });
                        }
                      }}
                      style={{
                        padding: '8px 12px',
                        borderRadius: '4px',
                        border: '1px solid rgba(255, 255, 255, 0.23)',
                        backgroundColor: 'transparent',
                        color: 'inherit',
                        fontSize: '0.875rem'
                      }}
                    >
                      <option value="macbook-13">MacBook Pro 13" (2560x1600 @ 60Hz) - Default</option>
                      <option value="macbook-16">MacBook Pro 16" (3456x2234 @ 120Hz)</option>
                      <option value="macbook-15">MacBook Air 15" (2880x1864 @ 60Hz)</option>
                      <option value="5k">5K Display (5120x2880 @ 60Hz)</option>
                      <option value="4k">4K Display (3840x2160 @ 60Hz)</option>
                      <option value="fhd">Full HD (1920x1080 @ 60Hz)</option>
                      <option value="iphone-15-pro">iPhone 15 Pro - Vertical (1179x2496 @ 120Hz)</option>
                      <option value="custom">Custom...</option>
                    </select>
                  </FormControl>

                  {localConfig.display_width && localConfig.display_height && (
                    <Typography variant="caption" color="text.secondary">
                      Current: {localConfig.display_width || 2560}×{localConfig.display_height || 1600} @ {localConfig.display_refresh_rate || 60}Hz
                    </Typography>
                  )}
                </Box>

                <Alert severity="info" sx={{ mt: 1 }}>
                  <Typography variant="body2">
                    External agents run in isolated containers with streaming access for visual development.
                    Launch "Wolf UI" in Moonlight to connect and enter the lobby PIN.
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