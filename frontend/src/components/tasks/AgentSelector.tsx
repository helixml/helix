import React, { useState } from 'react';
import { 
  Box, 
  Typography, 
  Button, 
  Menu, 
  MenuItem,
  Tooltip,
} from '@mui/material';
import KeyboardArrowDownIcon from '@mui/icons-material/KeyboardArrowDown';
import { IApp } from '../../types'

interface AgentSelectorProps {
  apps: IApp[]; // Agents
  selectedAgent?: IApp;
  onAgentSelect: (agent: IApp) => void;
}

const AgentSelector: React.FC<AgentSelectorProps> = ({
  apps,
  selectedAgent,
  onAgentSelect,
}) => {
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const open = Boolean(anchorEl);

  const handleClick = (event: React.MouseEvent<HTMLElement>) => {
    if (apps.length > 0) {
      setAnchorEl(event.currentTarget);
    }
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const handleAgentSelect = (agent: IApp) => {
    onAgentSelect(agent);
    handleClose();
  };

  const hasAgents = apps.length > 0;
  const buttonText = selectedAgent ? selectedAgent.config.helix.name : 'Select Agent';

  return (
    <Box>
      <Tooltip 
        title={!hasAgents ? "You need to first create a new agent" : ""}
        placement="top"
      >
        <span>
          <Button
            variant="outlined"
            onClick={handleClick}
            endIcon={<KeyboardArrowDownIcon />}
            disabled={!hasAgents}
            sx={{
              borderRadius: '8px',
              textTransform: 'none',
              borderColor: hasAgents ? '#4A5568' : '#2D3748',
              color: hasAgents ? '#A0AEC0' : '#4A5568',
              '&:hover': {
                borderColor: hasAgents ? 'secondary.main' : '#2D3748',
                color: hasAgents ? 'secondary.main' : '#4A5568',
              },
              '&:disabled': {
                borderColor: '#2D3748',
                color: '#4A5568',
              },
              minWidth: '200px',
              justifyContent: 'space-between',
            }}
          >
            {buttonText}
          </Button>
        </span>
      </Tooltip>
      {hasAgents && (
        <Menu
          anchorEl={anchorEl}
          open={open}
          onClose={handleClose}
          PaperProps={{
            sx: {
              backgroundColor: '#181A20',
              border: '1px solid #23262F',
              mt: 1,
            }
          }}
        >
          {apps.map((app) => (
            <MenuItem
              key={app.id}
              onClick={() => handleAgentSelect(app)}
              selected={selectedAgent?.id === app.id}
              sx={{
                color: '#F1F1F1',
                '&:hover': {
                  backgroundColor: '#23262F',
                },
                '&.Mui-selected': {
                  backgroundColor: '#3182CE',
                  '&:hover': {
                    backgroundColor: '#2B6CB0',
                  },
                },
              }}
            >
              <Box>
                <Typography variant="body2" sx={{ fontWeight: 500 }}>
                  {app.config.helix.name}
                </Typography>
                <Typography variant="caption" sx={{ color: '#A0AEC0' }}>
                  {app.config.helix.description}
                </Typography>
              </Box>
            </MenuItem>
          ))}
        </Menu>
      )}
    </Box>
  );
};

export default AgentSelector; 