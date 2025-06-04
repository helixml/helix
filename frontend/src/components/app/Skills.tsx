import React from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Typography, Switch, FormControlLabel } from '@mui/material';
import { PROVIDER_ICONS, PROVIDER_COLORS } from '../icons/ProviderIcons';
import ShowChartIcon from '@mui/icons-material/ShowChart';
import { IAppFlatState } from '../../types';
import { alphaVantageTool } from './examples/alphaVantageApi';

// Example static skills/plugins data
const SKILLS = [
  // TODO: build skills in the backend for browser use and calculator
  // {
  //   id: 'web-search',
  //   name: 'Web Search',
  //   description: 'Search for information from the internet in real-time using Google Search.',
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
    name: 'Market News',
    description: 'Get latest market news and sentiment data using Alpha Vantage API',
    type: 'custom',
    skill: alphaVantageTool,
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
  const isSkillEnabled = (skillName: string): boolean => {
    return app.apiTools?.some(tool => tool.name === skillName) ?? false;
  };

  const handleSkillToggle = async (skillName: string, enabled: boolean) => {
    const currentTools = app.apiTools || [];
    let updatedTools;

    if (enabled) {
      // Configuring skill
      
    } else {
      // Remove the skill from apiTools
      updatedTools = currentTools.filter(tool => tool.name !== skillName);
    }

    if (updatedTools) {
      await onUpdate({
        ...app,
        apiTools: updatedTools,
      });
    }
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
                />
                <CardContent sx={{ flexGrow: 1 }}>
                  <Typography variant="body2" color="text.secondary">
                    {skill.description}
                  </Typography>
                </CardContent>
                <CardActions sx={{ justifyContent: 'flex-end', px: 2, pb: 2 }}>
                  <FormControlLabel
                    control={
                      <Switch
                        checked={isEnabled}
                        onChange={(e) => handleSkillToggle(skill.name, e.target.checked)}
                        color="primary"
                      />
                    }
                    label={isEnabled ? "Enabled" : "Disabled"}
                    labelPlacement="start"
                  />
                </CardActions>
              </Card>
            </Grid>
          );
        })}
      </Grid>
    </Box>
  );
};

export default Skills;
