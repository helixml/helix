import React from 'react';
import {
  Box,
  Chip,
  IconButton,
  Tooltip,
  Typography,
  Button,
} from '@mui/material';
import {
  Chat as ChatIcon,
  Code as CodeIcon,
  AutoAwesome as AutoAwesomeIcon,
  Visibility as VisibilityIcon,
  Circle as CircleIcon,
  Warning as WarningIcon,
  CheckCircle as CheckCircleIcon,
} from '@mui/icons-material';
import { 
  IAgentType, 
  AGENT_TYPE_HELIX_BASIC,
  AGENT_TYPE_HELIX_AGENT,
  AGENT_TYPE_ZED_EXTERNAL 
} from '../../types';

interface AgentStatusIndicatorProps {
  agentType: IAgentType;
  sessionId?: string;
  status?: 'starting' | 'active' | 'waiting' | 'failed' | 'completed';
  rdpUrl?: string;
  onOpenRDP?: () => void;
  size?: 'small' | 'medium' | 'large';
  showLabel?: boolean;
  interactive?: boolean;
}

const AgentStatusIndicator: React.FC<AgentStatusIndicatorProps> = ({
  agentType,
  sessionId,
  status = 'active',
  rdpUrl,
  onOpenRDP,
  size = 'medium',
  showLabel = true,
  interactive = true,
}) => {
  const getAgentIcon = () => {
    switch (agentType) {
      case AGENT_TYPE_HELIX_BASIC:
        return <ChatIcon fontSize={size} />;
      case AGENT_TYPE_HELIX_AGENT:
        return <AutoAwesomeIcon fontSize={size} />;
      case AGENT_TYPE_ZED_EXTERNAL:
        return <CodeIcon fontSize={size} />;
      default:
        return <ChatIcon fontSize={size} />;
    }
  };

  const getAgentLabel = () => {
    switch (agentType) {
      case AGENT_TYPE_HELIX_BASIC:
        return 'Basic Helix Agent';
      case AGENT_TYPE_HELIX_AGENT:
        return 'Multi-Turn Helix Agent';
      case AGENT_TYPE_ZED_EXTERNAL:
        return 'Zed External Agent';
      default:
        return 'Unknown Agent';
    }
  };

  const getStatusColor = () => {
    switch (status) {
      case 'starting':
        return 'warning';
      case 'active':
        return 'success';
      case 'waiting':
        return 'info';
      case 'failed':
        return 'error';
      case 'completed':
        return 'success';
      default:
        return 'default';
    }
  };

  const getStatusIcon = () => {
    switch (status) {
      case 'starting':
        return <CircleIcon sx={{ fontSize: 8, color: 'warning.main' }} />;
      case 'active':
        return <CheckCircleIcon sx={{ fontSize: 8, color: 'success.main' }} />;
      case 'waiting':
        return <CircleIcon sx={{ fontSize: 8, color: 'info.main' }} />;
      case 'failed':
        return <WarningIcon sx={{ fontSize: 8, color: 'error.main' }} />;
      case 'completed':
        return <CheckCircleIcon sx={{ fontSize: 8, color: 'success.main' }} />;
      default:
        return <CircleIcon sx={{ fontSize: 8, color: 'grey.400' }} />;
    }
  };

  const getStatusText = () => {
    switch (status) {
      case 'starting':
        return 'Starting...';
      case 'active':
        return 'Active';
      case 'waiting':
        return 'Waiting';
      case 'failed':
        return 'Failed';
      case 'completed':
        return 'Completed';
      default:
        return 'Unknown';
    }
  };

  const isExternalAgent = agentType === AGENT_TYPE_ZED_EXTERNAL;

  return (
    <Box 
      sx={{ 
        display: 'flex', 
        alignItems: 'center', 
        gap: 1,
        p: size === 'small' ? 0.5 : 1,
      }}
    >
      {/* Agent Type Indicator */}
      <Chip
        icon={getAgentIcon()}
        label={showLabel ? getAgentLabel() : undefined}
        color={agentType === AGENT_TYPE_HELIX_BASIC || agentType === AGENT_TYPE_HELIX_AGENT ? 'primary' : 'secondary'}
        size={size === 'large' ? 'medium' : 'small'}
        variant={agentType === AGENT_TYPE_HELIX_BASIC || agentType === AGENT_TYPE_HELIX_AGENT ? 'filled' : 'outlined'}
      />

      {/* Status Indicator for External Agents */}
      {isExternalAgent && (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          {getStatusIcon()}
          {showLabel && size !== 'small' && (
            <Typography variant="caption" color="text.secondary">
              {getStatusText()}
            </Typography>
          )}
        </Box>
      )}

      {/* RDP Access Button for External Agents */}
      {isExternalAgent && interactive && status === 'active' && (
        <Tooltip title="Open RDP Viewer">
          <IconButton
            size="small"
            onClick={onOpenRDP}
            disabled={!rdpUrl}
            sx={{ 
              ml: 'auto',
              color: 'primary.main',
              '&:hover': {
                backgroundColor: 'primary.main',
                color: 'primary.contrastText',
              },
            }}
          >
            <VisibilityIcon fontSize="small" />
          </IconButton>
        </Tooltip>
      )}

      {/* Session ID for debugging (only in development) */}
      {process.env.NODE_ENV === 'development' && sessionId && size === 'large' && (
        <Typography variant="caption" color="text.disabled" sx={{ ml: 1 }}>
          {sessionId.slice(-8)}
        </Typography>
      )}
    </Box>
  );
};

export default AgentStatusIndicator;