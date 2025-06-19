import React, { useState } from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Typography, Button, Tooltip } from '@mui/material';
import Container from '@mui/material/Container';
import Page from '../components/system/Page';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import AddProviderDialog from '../components/providers/AddProviderDialog';

import { useListProviders } from '../services/providersService';

import { PROVIDERS, Provider } from '../components/providers/types';

interface ProviderConfig {
  apiKey: string;
}

type ProviderConfigs = Record<string, ProviderConfig>;

const Providers: React.FC = () => {
  const [configs, setConfigs] = useState<ProviderConfigs>({});
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  const { data: providerEndpoints = [], isLoading: isLoadingProviders, refetch: loadData } = useListProviders(false);

  const handleOpenDialog = (provider: Provider) => {
    setSelectedProvider(provider);
    setDialogOpen(true);
  };

  const handleCloseDialog = () => {
    setDialogOpen(false);
    setSelectedProvider(null);
  };

  // Filter for user endpoints only
  const userEndpoints = providerEndpoints.filter(endpoint => endpoint.endpoint_type === 'user');

  return (
    <Page breadcrumbTitle="Providers" topbarContent={null}>      
      <Container maxWidth="md" sx={{ mt: 10, mb: 6, display: 'flex', flexDirection: 'column', alignItems: 'left' }}>
        <Typography variant="h4" sx={{ mb: 2, fontWeight: 600 }}>
          AI Providers
        </Typography>
        <Typography variant="body1" color="text.secondary" sx={{ mb: 4 }}>
          Add your own API keys to use with your Helix agents.
        </Typography>
        <Grid container spacing={3} justifyContent="left">
          {PROVIDERS.map((provider) => {
            const isConfigured = userEndpoints.some(endpoint => endpoint.name === provider.id);
            const existingProvider = userEndpoints.find(endpoint => endpoint.name === provider.id);
            return (
              <Grid item xs={12} sm={6} md={4} key={provider.id} display="flex" justifyContent="center">
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
                      width: 320,
                      height: '100%',
                      display: 'flex',
                      flexDirection: 'column',
                      alignItems: 'center',
                      justifyContent: 'center',
                      boxShadow: 2,
                      borderStyle: 'dashed',
                      borderWidth: 1,
                      borderColor: 'divider',
                      opacity: isLoadingProviders ? 0.5 : (isConfigured ? 1 : 0.85),
                      transition: 'all 0.2s',
                      '&:hover': {
                        boxShadow: isLoadingProviders ? 2 : 4,
                        transform: isLoadingProviders ? 'none' : 'translateY(-4px)',
                        borderColor: isLoadingProviders ? 'divider' : 'primary.main',
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
                        size="small"
                        variant={isConfigured ? 'outlined' : 'text'}
                        color={isConfigured ? 'success' : 'secondary'}
                        onClick={() => handleOpenDialog(provider)}
                        startIcon={isConfigured ? <CheckCircleIcon /> : <AddCircleOutlineIcon />}
                      >
                        {isConfigured ? 'Connected' : 'Connect'}
                      </Button>
                    </CardActions>
                  </Card>
                </Tooltip>
              </Grid>
            );
          })}
        </Grid>
        {selectedProvider && (
          <AddProviderDialog
            open={dialogOpen}
            onClose={handleCloseDialog}
            provider={selectedProvider}
            existingProvider={userEndpoints.find(endpoint => endpoint.name === selectedProvider.id)}
          />
        )}
      </Container>
    </Page>
  );
};

export default Providers; 