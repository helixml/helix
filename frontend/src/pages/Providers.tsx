import React, { useState } from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Typography, Button, Dialog, DialogTitle, DialogContent, DialogActions, TextField, Tooltip } from '@mui/material';

// Import SVGs as components
import OpenAILogo from '../components/providers/logos/openai';
import AnthropicLogo from '../components/providers/logos/anthropic';
import GroqLogo from '../components/providers/logos/groq';
import CerebrasLogo from '../components/providers/logos/cerebras';

import googleLogo from '../../assets/img/providers/google.svg';


interface Provider {
  id: string;
  name: string;
  description: string;
  logo: string | React.ComponentType<React.SVGProps<SVGSVGElement>> | React.ComponentType<any>;
}

const PROVIDERS: Provider[] = [
  {
    id: 'openai',
    name: 'OpenAI',
    description: 'Connect to OpenAI for GPT models, image generation, and more.',
    logo: OpenAILogo,
  },
  {
    id: 'google',
    name: 'Google',
    description: 'Use Google AI models and services.',
    logo: googleLogo,
  },
  {
    id: 'anthropic',
    name: 'Anthropic',
    description: 'Access Anthropic Claude models for advanced language tasks.',
    logo: AnthropicLogo,
  },
  {
    id: 'groq',
    name: 'Groq',
    description: 'Integrate with Groq for ultra-fast LLM inference.',
    logo: GroqLogo,
  },
  {
    id: 'cerebras',
    name: 'Cerebras',
    description: 'Integrate with Cerebras for ultra-fast LLM inference.',
    logo: CerebrasLogo,
  },
];

interface ProviderConfig {
  apiKey: string;
}

type ProviderConfigs = Record<string, ProviderConfig>;

const Providers: React.FC = () => {
  const [configs, setConfigs] = useState<ProviderConfigs>({});
  const [dialogOpen, setDialogOpen] = useState(false);
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  const [apiKey, setApiKey] = useState('');

  const handleOpenDialog = (provider: Provider) => {
    setSelectedProvider(provider);
    setApiKey(configs[provider.id]?.apiKey || '');
    setDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
    setSelectedProvider(null);
    setApiKey('');
  };

  const handleSave = () => {
    if (selectedProvider) {
      setConfigs({
        ...configs,
        [selectedProvider.id]: { apiKey },
      });
    }
    handleCloseDialog();
  };

  const handleDelete = () => {
    if (selectedProvider) {
      const newConfigs = { ...configs };
      delete newConfigs[selectedProvider.id];
      setConfigs(newConfigs);
    }
    handleCloseDialog();
  };

  return (
    <Box sx={{ mt: 4, mx: 4 }}>
      <Typography variant="h4" sx={{ mb: 2 }}>
        AI Providers
      </Typography>
      <Typography variant="body1" color="text.secondary" sx={{ mb: 4 }}>
        Add your own API keys to use with your Helix agents.
      </Typography>
      <Grid container spacing={3}>
        {PROVIDERS.map((provider) => {
          const isConfigured = !!configs[provider.id];
          return (
            <Grid item xs={12} sm={6} md={4} lg={3} key={provider.id}>
              <Tooltip
                title={
                  <Box sx={{ p: 1 }}>
                    <Typography variant="subtitle2" sx={{ fontWeight: 'bold', mb: 1 }}>
                      {provider.name}
                    </Typography>
                    <Typography variant="body2">{provider.description}</Typography>
                  </Box>
                }
                arrow
                placement="bottom"
              >
                <Card
                  sx={{
                    height: '100%',
                    display: 'flex',
                    flexDirection: 'column',
                    alignItems: 'center',
                    justifyContent: 'center',
                    boxShadow: 2,
                    borderStyle: 'dashed',
                    borderWidth: 1,
                    borderColor: 'divider',
                    opacity: isConfigured ? 1 : 0.85,
                    transition: 'all 0.2s',
                    '&:hover': {
                      boxShadow: 4,
                      transform: 'translateY(-4px)',
                    },
                  }}
                >
                  <CardHeader
                    avatar={
                      <Avatar sx={{ bgcolor: 'white', width: 56, height: 56 }}>
                        {typeof provider.logo === 'string' ? (
                          <img src={provider.logo} alt={provider.name} style={{ width: 40, height: 40 }} />
                        ) : (
                          <provider.logo style={{ width: 40, height: 40 }} />
                        )}
                      </Avatar>
                    }
                    title={provider.name}
                    titleTypographyProps={{ variant: 'h6', align: 'center' }}
                  />
                  <CardContent sx={{ flexGrow: 1, textAlign: 'center' }}>
                    <Typography variant="body2" color="text.secondary">
                      {provider.description}
                    </Typography>
                  </CardContent>
                  <CardActions sx={{ justifyContent: 'center', pb: 2 }}>
                    <Button
                      variant={isConfigured ? 'outlined' : 'contained'}
                      color={isConfigured ? 'success' : 'primary'}
                      onClick={() => handleOpenDialog(provider)}
                    >
                      {isConfigured ? 'Edit' : 'Add'}
                    </Button>
                  </CardActions>
                </Card>
              </Tooltip>
            </Grid>
          );
        })}
      </Grid>

      <Dialog open={dialogOpen} onClose={handleCloseDialog} maxWidth="xs" fullWidth>
        <DialogTitle>
          {selectedProvider ? `${configs[selectedProvider.id] ? 'Edit' : 'Add'} ${selectedProvider.name} Provider` : ''}
        </DialogTitle>
        <DialogContent>
          <TextField
            label="API Key"
            fullWidth
            value={apiKey}
            onChange={(e) => setApiKey(e.target.value)}
            margin="normal"
            autoFocus
            type="password"
          />
        </DialogContent>
        <DialogActions>
          {selectedProvider && configs[selectedProvider.id] && (
            <Button onClick={handleDelete} color="error">
              Delete
            </Button>
          )}
          <Button onClick={handleCloseDialog}>Cancel</Button>
          <Button onClick={handleSave} variant="contained" color="primary" disabled={!apiKey}>
            Save
          </Button>
        </DialogActions>
      </Dialog>
    </Box>
  );
};

export default Providers; 