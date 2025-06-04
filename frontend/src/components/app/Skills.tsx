import React, { useState } from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Typography, Switch, FormControlLabel } from '@mui/material';
import { PROVIDER_ICONS, PROVIDER_COLORS } from '../icons/ProviderIcons';
import ShowChartIcon from '@mui/icons-material/ShowChart';
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
    name: 'Market News',
    description: 'Get latest market news and sentiment data using Alpha Vantage API',
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
  const [selectedSkill, setSelectedSkill] = useState<IAgentSkill | null>(null);
  const [isDialogOpen, setIsDialogOpen] = useState(false);

  const isSkillEnabled = (skillName: string): boolean => {
    return app.apiTools?.some(tool => tool.name === skillName) ?? false;
  };

  const handleSkillToggle = async (skillName: string, enabled: boolean) => {
    const currentTools = app.apiTools || [];
    let updatedTools;

    if (enabled) {
      // Find the skill in SKILLS array
      const skill = SKILLS.find(s => s.name === skillName);
      
      if (skill?.apiSkill) {
        // If skill has apiSkill, open the dialog for configuration
        setSelectedSkill({
          name: skill.name,
          description: skill.description,
          systemPrompt: '',
          apiSkill: skill.apiSkill,
          configurable: true,
        });
        setIsDialogOpen(true);
        return;
      }
      
      // If no apiSkill, just add the skill directly
      updatedTools = [...currentTools, {
        name: skillName,
        description: skill?.description || '',
        schema: '',
        url: '',
      }];
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

  const handleDialogSave = async (skill: IAgentSkill) => {
    const currentTools = app.apiTools || [];
    const updatedTools = [...currentTools, {
      name: skill.name,
      description: skill.description,
      schema: skill.apiSkill.schema,
      url: skill.apiSkill.url,
    }];

    await onUpdate({
      ...app,
      apiTools: updatedTools,
    });

    setIsDialogOpen(false);
    setSelectedSkill(null);
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

      <AddApiSkillDialog
        open={isDialogOpen}
        onClose={() => {
          setIsDialogOpen(false);
          setSelectedSkill(null);
        }}
        onSave={handleDialogSave}
        existingSkill={selectedSkill || undefined}
      />
    </Box>
  );
};

export default Skills;
