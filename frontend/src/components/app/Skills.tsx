import React, { useState } from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Typography, Button, IconButton, Menu, MenuItem, useTheme } from '@mui/material';
import { PROVIDER_ICONS, PROVIDER_COLORS } from '../icons/ProviderIcons';
import ShowChartIcon from '@mui/icons-material/ShowChart';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import { IAppFlatState, IAgentSkill } from '../../types';
import { alphaVantageTool } from './examples/alphaVantageApi';
import AddApiSkillDialog from './AddApiSkillDialog';

interface ISkill {
  id: string;
  icon?: React.ReactNode;
  name: string;
  description: string;
  type: string;
  skill: IAgentSkill;
  apiSkill?: IAgentSkill['apiSkill'];
}

// Example static skills/plugins data
const SKILLS: ISkill[] = [
  // TODO: build skills in the backend for browser use and calculator
  // {
  //   id: 'browser-use',
  //   name: 'Browser Use',
  //   description: 'Use the browser to open links and search for information in real-time.',
  //   type: 'custom',
  //   enabled: false,
  // },
  // {
  //   id: 'simple-calculator',
  //   name: 'Simple Calculator',
  //   description: 'Calculate a math expression. For example, "2 + 2" or "2 * 2".',
  //   type: 'custom',
  //   enabled: false,
  // },
  {
    id: 'alpha-vantage',
    icon: <ShowChartIcon />,
    name: alphaVantageTool.name,
    description: alphaVantageTool.description,
    type: 'custom',
    skill: alphaVantageTool,
    apiSkill: alphaVantageTool.apiSkill,
  },  
];

interface SkillsProps {
  app: IAppFlatState,
  onUpdate: (updates: IAppFlatState) => Promise<void>,
}

const Skills: React.FC<SkillsProps> = ({ 
  app, 
  onUpdate,
}) => {
  const theme = useTheme();

  const [selectedSkill, setSelectedSkill] = useState<IAgentSkill | null>(null);
  const [isDialogOpen, setIsDialogOpen] = useState(false);
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null);
  const [selectedSkillForMenu, setSelectedSkillForMenu] = useState<string | null>(null);

  const isSkillEnabled = (skillName: string): boolean => {
    console.log('app.apiTools: ', app.apiTools);
    console.log('skillName: ', skillName);
    return app.apiTools?.some(tool => tool.name === skillName) ?? false;
  };

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>, skillName: string) => {
    setMenuAnchorEl(event.currentTarget);
    setSelectedSkillForMenu(skillName);
  };

  const handleMenuClose = () => {
    setMenuAnchorEl(null);
    setSelectedSkillForMenu(null);
  };

  const handleDisableSkill = () => {
    if (selectedSkillForMenu) {
      const skill = SKILLS.find(s => s.name === selectedSkillForMenu);
      if (skill) {
        setSelectedSkill(skill.skill);
        setIsDialogOpen(true);
      }
    }
    handleMenuClose();
  };

  const handleOpenDialog = (skill: ISkill) => {
    setSelectedSkill(skill.skill);
    setIsDialogOpen(true);
  };

  return (
    <Box sx={{ mt: 2, mr: 4 }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        Skills
      </Typography>
      <Grid container spacing={2}>
        {SKILLS.map((skill) => {
          const defaultIcon = PROVIDER_ICONS[skill.type] || PROVIDER_ICONS['custom'];
          const color = PROVIDER_COLORS[skill.type] || PROVIDER_COLORS['custom'];
          const isEnabled = isSkillEnabled(skill.name);
          
          return (
            <Grid item xs={12} sm={6} md={4} lg={3} key={skill.id}>
              <Card
                sx={{
                  height: '100%',
                  display: 'flex',
                  flexDirection: 'column',
                  transition: 'all 0.2s',
                  boxShadow: 2,
                  '&:hover': {
                    transform: 'translateY(-4px)',
                    boxShadow: 4,
                  },
                }}
              >
                <CardHeader
                  avatar={
                    <Avatar sx={{ bgcolor: color, color: 'white', width: 40, height: 40 }}>
                      {skill.icon || defaultIcon}
                    </Avatar>
                  }
                  title={skill.name}
                  titleTypographyProps={{ variant: 'h6' }}
                  action={
                    isEnabled && (
                      <IconButton
                        onClick={(e) => handleMenuOpen(e, skill.name)}
                        size="small"
                      >
                        <MoreVertIcon />
                      </IconButton>
                    )
                  }
                />
                <CardContent sx={{ flexGrow: 1 }}>
                  <Typography variant="body2" color="text.secondary">
                    {skill.description}
                  </Typography>
                </CardContent>
                <CardActions sx={{ justifyContent: 'center', px: 2, pb: 2 }}>
                  {isEnabled ? (
                    <Button
                      startIcon={<CheckCircleIcon sx={{ color: '#4caf50' }} />}
                      sx={{ 
                        color: theme.palette.success.main,
                        borderColor: theme.palette.success.main,
                        '&:hover': {
                          borderColor: theme.palette.success.main,
                          backgroundColor: 'rgba(76, 175, 80, 0.04)'
                        }
                      }}
                      variant="outlined"     
                      onClick={() => handleOpenDialog(skill)}                                       
                    >
                      Enabled
                    </Button>
                  ) : (
                    <Button
                      startIcon={<AddCircleOutlineIcon />}
                      color="secondary"
                      variant="outlined"
                      onClick={() => handleOpenDialog(skill)}
                    >
                      Enable
                    </Button>
                  )}
                </CardActions>
              </Card>
            </Grid>
          );
        })}
      </Grid>

      <Menu
        anchorEl={menuAnchorEl}
        open={Boolean(menuAnchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleDisableSkill}>Disable</MenuItem>
      </Menu>

      <AddApiSkillDialog
        open={isDialogOpen}
        onClose={() => {
          setIsDialogOpen(false);
          // setSelectedSkill(null);
        }}
        skill={selectedSkill || undefined}
        app={app}
        onUpdate={onUpdate}
        isEnabled={selectedSkill ? isSkillEnabled(selectedSkill.name) : false}
      />
    </Box>
  );
};

export default Skills;
