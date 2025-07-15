import React, { useState } from 'react';
import { 
  Box, 
  Typography, 
  Button, 
  Menu, 
  MenuItem,   
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
    setAnchorEl(event.currentTarget);
  };

  const handleClose = () => {
    setAnchorEl(null);
  };

  const handleAgentSelect = (agent: IApp) => {
    onAgentSelect(agent);
    handleClose();
  };

  return (
    <Box>
      <Button
        variant="outlined"
        onClick={handleClick}
        endIcon={<KeyboardArrowDownIcon />}
        sx={{
          borderRadius: '8px',
          textTransform: 'none',
          borderColor: '#4A5568',
          color: '#A0AEC0',
          '&:hover': {
            borderColor: '#3182CE',
            color: '#3182CE',
          },
          minWidth: '200px',
          justifyContent: 'space-between',
        }}
      >
        {selectedAgent ? selectedAgent.config.helix.name : 'Select Agent'}
      </Button>
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
    </Box>
  );
};

export default AgentSelector; 