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
  Tooltip,
} from '@mui/material';
import {
  Chat as ChatIcon,
  Code as CodeIcon,
  Computer as ComputerIcon,
  AutoAwesome as AutoAwesomeIcon,
  Info as InfoIcon,
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
  size?: 'small' | 'medium';
}

const AgentTypeSelector: React.FC<AgentTypeSelectorProps> = ({
  value,
  onChange,
  externalAgentConfig = {},
  disabled = false,
  size = 'medium',
}) => {
  const handleAgentTypeChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    const newAgentType = event.target.value as IAgentType;
    onChange(newAgentType, newAgentType === AGENT_TYPE_ZED_EXTERNAL ? externalAgentConfig : undefined);
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
                borderColor: value === option.value ? 'primary.main' : 'divider',
                backgroundColor: 'transparent',
                transition: 'all 0.2s ease-in-out',
                '&:hover': {
                  borderColor: 'primary.main',
                  boxShadow: 1,
                },
              }}
            >
              <CardContent sx={{ p: size === 'small' ? 1.5 : 3, '&:last-child': { pb: size === 'small' ? 1.5 : 3 } }}>
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
                        <Tooltip title="External agents provide full development environments with code editing via browser or Moonlight client">
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

      {/* External Agent Configuration - settings are now in AppSettings.tsx Display Settings */}
    </Box>
  );
};

export default AgentTypeSelector;