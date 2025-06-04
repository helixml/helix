import React from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Button, Typography } from '@mui/material';
import { PROVIDER_ICONS, PROVIDER_COLORS } from '../icons/ProviderIcons';
import ShowChartIcon from '@mui/icons-material/ShowChart';
import { IAppFlatState } from '../../types';

// Example static skills/plugins data
const SKILLS = [
  {
    id: 'web-search',
    name: 'Web Search',
    description: 'Search for information from the internet in real-time using Google Search.',
    type: 'custom',
    enabled: true,
  },
  {
    id: 'simple-calculator',
    name: 'Simple Calculator',
    description: 'Calculate a math expression. For example, "2 + 2" or "2 * 2".',
    type: 'custom',
    enabled: true,
  },
  {
    id: 'alpha-vantage',
    icon: <ShowChartIcon />,
    name: 'Market News',
    description: 'Get latest market news and sentiment data using Alpha Vantage API',
    type: 'custom',
    enabled: true,
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
  return (
    <Box sx={{ mt: 2, mr: 4 }}>
      <Typography variant="h6" sx={{ mb: 2 }}>
        Skills
      </Typography>
      <Grid container spacing={2}>
        {SKILLS.map((skill) => {
          const defaultIcon = PROVIDER_ICONS[skill.type] || PROVIDER_ICONS['custom'];
          const color = PROVIDER_COLORS[skill.type] || PROVIDER_COLORS['custom'];
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
                <CardActions sx={{ justifyContent: 'flex-end' }}>
                  <Button
                    variant={skill.enabled ? 'contained' : 'outlined'}
                    color={skill.enabled ? 'primary' : 'inherit'}
                    disabled={skill.enabled}
                  >
                    {skill.enabled ? 'Enabled' : 'Enable'}
                  </Button>
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
