import React, { useState, useMemo } from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Typography, Button, IconButton, Menu, MenuItem, useTheme, Tooltip, Dialog, DialogTitle, DialogContent, DialogContentText, DialogActions } from '@mui/material';
import { PROVIDER_ICONS, PROVIDER_COLORS } from '../icons/ProviderIcons';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import MoreVertIcon from '@mui/icons-material/MoreVert';
import LanguageIcon from '@mui/icons-material/Language';
import { IAppFlatState, IAgentSkill } from '../../types';
import AddApiSkillDialog from './AddApiSkillDialog';
import BrowserSkill from './BrowserSkill';
import ApiIcon from '@mui/icons-material/Api';

import { alphaVantageTool } from './examples/skillAlphaVantageApi';
import { airQualityTool } from './examples/skillAirQualityApi';
import { exchangeRatesSkill } from './examples/skillExchangeRatesApi';

interface ISkill {
  id: string;
  icon?: React.ReactNode;
  name: string;
  description: string;
  type: string;
  skill: IAgentSkill;
}

const SKILL_TYPE_HTTP_API = 'HTTP API';
const SKILL_TYPE_BROWSER = 'Browser';

// Base static skills/plugins data
const BASE_SKILLS: ISkill[] = [
  {
    id: 'browser',
    icon: <LanguageIcon />,
    name: 'Browser',
    description: 'Enable the AI to browse websites and extract information from them. The AI can visit URLs and process their content.',
    type: SKILL_TYPE_BROWSER,
    skill: {
      name: 'Browser',
      description: 'Enable the AI to browse websites and extract information from them.',
      systemPrompt: '',
      apiSkill: {
        schema: '',
        url: '',
        requiredParameters: [],
      },
      configurable: true,
    },
  },
  {
    id: 'alpha-vantage',
    icon: alphaVantageTool.icon,
    name: alphaVantageTool.name,
    description: alphaVantageTool.description,
    type: SKILL_TYPE_HTTP_API,
    skill: alphaVantageTool,
  },
  {
    id: 'air-quality',
    icon: airQualityTool.icon,
    name: airQualityTool.name,
    description: airQualityTool.description,
    type: SKILL_TYPE_HTTP_API,
    skill: airQualityTool,
  },
  {
    id: 'exchange-rates',
    icon: exchangeRatesSkill.icon,
    name: exchangeRatesSkill.name,
    description: exchangeRatesSkill.description,
    type: SKILL_TYPE_HTTP_API,
    skill: exchangeRatesSkill,
  },
];

const CUSTOM_API_SKILL: ISkill = {
  id: 'new-custom-api',
  icon: <ApiIcon />,
  name: 'New API',
  description: 'Add your own custom API integration. Any HTTP endpoint can become a skill for your agent.',
  type: SKILL_TYPE_HTTP_API,
  skill: {
    name: 'Custom API',
    icon: <ApiIcon />,
    description: 'Add your own API integration.',
    systemPrompt: '',
    apiSkill: {
      schema: '',
      url: '',
      requiredParameters: [],
    },
    configurable: true,
  },
};

const getFirstLine = (text: string): string => {
  return text.split('\n')[0].trim();
};

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
  const [isDisableConfirmOpen, setIsDisableConfirmOpen] = useState(false);
  const [skillToDisable, setSkillToDisable] = useState<string | null>(null);

  // Convert custom APIs to skills
  const customApiSkills = useMemo(() => {
    if (!app.apiTools) return [];

    // Filter out any API tools that match predefined skills
    return app.apiTools
      .filter(api => !BASE_SKILLS.some(skill => skill.name === api.name))
      .map(api => ({
        id: `custom-api-${api.name}`,
        icon: <ApiIcon />,
        name: api.name,
        description: api.description,
        type: SKILL_TYPE_HTTP_API,
        skill: {
          name: api.name,
          icon: <ApiIcon />,
          description: api.description,
          systemPrompt: api.system_prompt || '',
          apiSkill: {
            schema: api.schema,
            url: api.url,
            requiredParameters: [],
            headers: api.headers || {},
            query: api.query || {},
            oauth_provider: api.oauth_provider || '',
            oauth_scopes: api.oauth_scopes || [],
          },
          configurable: true,
        },
      }));
  }, [app.apiTools]);

  // Combine base skills with custom API skills
  const allSkills = useMemo(() => {
    return [...BASE_SKILLS, ...customApiSkills, CUSTOM_API_SKILL];
  }, [customApiSkills]);

  const isSkillEnabled = (skillName: string): boolean => {
    if (skillName === 'Browser') {
      return app.browserTool?.enabled ?? false;
    }
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

  const handleDisableSkill = async () => {
    if (skillToDisable) {
      const skill = allSkills.find(s => s.name === skillToDisable);
      if (skill) {
        // Remove the tool from app.apiTools
        const updatedTools = app.apiTools?.filter(tool => tool.name !== skill.name) || [];
        
        // Update the app state
        await onUpdate({
          ...app,
          apiTools: updatedTools
        });
      }
    }
    handleMenuClose();
    setIsDisableConfirmOpen(false);
    setSkillToDisable(null);
  };

  const handleDisableClick = () => {
    setSkillToDisable(selectedSkillForMenu);
    setIsDisableConfirmOpen(true);
    handleMenuClose();
  };

  const handleOpenDialog = (skill: ISkill) => {
    // For custom API tile, don't pass the skill template
    if (skill.id === 'new-custom-api') {
      setSelectedSkill(null);
    } else {
      setSelectedSkill(skill.skill);
    }
    setIsDialogOpen(true);
  };

  const renderSkillDialog = () => {
    if (!selectedSkill) {
      return (
        <AddApiSkillDialog
          open={isDialogOpen}
          onClose={() => {
            setIsDialogOpen(false);
          }}
          onClosed={() => {
            setSelectedSkill(null);
          }}
          skill={undefined}
          app={app}
          onUpdate={onUpdate}
          isEnabled={false}
        />
      );
    }

    if (selectedSkill.name === 'Browser') {
      return (
        <BrowserSkill
          open={isDialogOpen}
          onClose={() => {
            setIsDialogOpen(false);
          }}
          onClosed={() => {
            setSelectedSkill(null);
          }}
          app={app}
          onUpdate={onUpdate}
          isEnabled={isSkillEnabled('Browser')}
        />
      );
    }

    return (
      <AddApiSkillDialog
        open={isDialogOpen}
        onClose={() => {
          setIsDialogOpen(false);
        }}
        onClosed={() => {
          setSelectedSkill(null);
        }}
        skill={selectedSkill}
        app={app}
        onUpdate={onUpdate}
        isEnabled={isSkillEnabled(selectedSkill.name)}
      />
    );
  };

  return (
    <Box sx={{ mt: 2, mr: 4 }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        💡 Skills
      </Typography>
      {/* Add a paragraph with info about skills */}
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Extend the capabilities of the AI with custom functions, APIs and workflows.
      </Typography>
      <Grid container spacing={2}>
        {allSkills.map((skill) => {
          const defaultIcon = PROVIDER_ICONS[skill.type] || PROVIDER_ICONS['custom'];
          const color = PROVIDER_COLORS[skill.type] || PROVIDER_COLORS['custom'];
          const isEnabled = isSkillEnabled(skill.name);
          const isCustomApiTile = skill.id === 'new-custom-api';
          
          return (
            <Grid item xs={12} sm={6} md={4} lg={3} key={skill.id}>
              <Tooltip
                title={
                  <Box sx={{ p: 1 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 1 }}>
                      Skill Type: {skill.type.toUpperCase()}
                    </Typography>
                    <Typography variant="body2">
                      {skill.description}
                    </Typography>
                  </Box>
                }
                arrow
                placement="bottom"
                componentsProps={{
                  tooltip: {
                    sx: {
                      bgcolor: 'background.paper',
                      color: 'text.primary',
                      border: '1px solid',
                      borderColor: 'divider',
                      boxShadow: 3,
                      maxWidth: 300,
                      '& .MuiTooltip-arrow': {
                        color: 'background.paper',
                        '&:before': {
                          border: '1px solid',
                          borderColor: 'divider',
                        },
                      },
                    },
                  },
                }}
              >
                <Card
                  sx={{
                    height: '100%',
                    display: 'flex',
                    flexDirection: 'column',
                    transition: 'all 0.2s',
                    boxShadow: 2,
                    opacity: isEnabled ? 1 : 0.7,
                    '&:hover': {
                      transform: isEnabled ? 'translateY(-4px)' : 'none',
                      boxShadow: isEnabled ? 4 : 2,
                    },
                    ...(isCustomApiTile && {
                      position: 'relative',
                      '&::before': {
                        content: '""',
                        position: 'absolute',
                        top: 0,
                        left: 0,
                        right: 0,
                        bottom: 0,
                        borderRadius: 'inherit',
                        padding: '2px',
                        background: 'linear-gradient(45deg, #ff6b6b, #4ecdc4, #45b7d1, #96c93d)',
                        WebkitMask: 'linear-gradient(#fff 0 0) content-box, linear-gradient(#fff 0 0)',
                        WebkitMaskComposite: 'xor',
                        maskComposite: 'exclude',
                        animation: 'shimmer 3s linear infinite',
                        backgroundSize: '300% 300%',
                      },
                      '@keyframes shimmer': {
                        '0%': {
                          backgroundPosition: '0% 50%',
                        },
                        '50%': {
                          backgroundPosition: '100% 50%',
                        },
                        '100%': {
                          backgroundPosition: '0% 50%',
                        },
                      },
                    }),
                  }}
                >
                  <CardHeader
                    avatar={
                      <Avatar sx={{ bgcolor: 'white', color: color, width: 40, height: 40 }}>
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
                      {getFirstLine(skill.description)}
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
                        {isCustomApiTile ? 'Add' : 'Enable'}
                      </Button>
                    )}
                  </CardActions>
                </Card>
              </Tooltip>
            </Grid>
          );
        })}
      </Grid>

      <Menu
        anchorEl={menuAnchorEl}
        open={Boolean(menuAnchorEl)}
        onClose={handleMenuClose}
      >
        <MenuItem onClick={handleDisableClick}>Disable</MenuItem>
      </Menu>

      <Dialog
        open={isDisableConfirmOpen}
        onClose={() => {
          setIsDisableConfirmOpen(false);
          setSkillToDisable(null);
        }}
      >
        <DialogTitle>Disable Skill</DialogTitle>
        <DialogContent>
          <DialogContentText>
            Are you sure you want to disable {skillToDisable ? `"${skillToDisable}"` : 'this skill'}? All configuration will be lost once the skill is disabled.
          </DialogContentText>
        </DialogContent>
        <DialogActions>
          <Button onClick={() => {
            setIsDisableConfirmOpen(false);
            setSkillToDisable(null);
          }}>Cancel</Button>
          <Button onClick={handleDisableSkill} color="error" variant="contained">
            Disable
          </Button>
        </DialogActions>
      </Dialog>

      {renderSkillDialog()}
    </Box>
  );
};

export default Skills;
