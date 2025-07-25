import React, { useState,useMemo } from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Typography, Button, Tooltip } from '@mui/material';
import Container from '@mui/material/Container';
import Page from '../components/system/Page';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import AddProviderDialog from '../components/providers/AddProviderDialog';

import { useListProviders } from '../services/providersService';
import { useGetOrgByName } from '../services/orgService';

import { PROVIDERS, Provider } from '../components/providers/types';
import useRouter from '../hooks/useRouter';
import useAccount from '../hooks/useAccount';


interface ProviderConfig {
  apiKey: string;
}

const Providers: React.FC = () => {
  const router = useRouter()
  const account = useAccount()
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  const orgName = router.params.org_id

  // Get org if orgName is set  
  const { data: org, isLoading: isLoadingOrg } = useGetOrgByName(orgName, orgName !== undefined)

  // Get provider endpoints
  const { data: providerEndpoints = [], isLoading: isLoadingProviders, refetch: loadData } = useListProviders(false, org?.id, !isLoadingOrg);


  let editAllowed = false

  // If we are not in an org context - we can perform all actions
  if (orgName === undefined) {
    editAllowed = true
  } else {
    // Otherwise we need to check if we are an org admin
    editAllowed = account.isOrgAdmin
  }


  const handleOpenDialog = (provider: Provider) => {
    if (!editAllowed) return;
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
          {editAllowed 
            ? "Add your own API keys to use with your Helix agents."
            : "View the AI providers configured for your organization. Contact your organization owner to add new providers."
          }
        </Typography>
        <Grid container spacing={3} justifyContent="left">
          {PROVIDERS.map((provider) => {
            const isConfigured = userEndpoints.some(endpoint => endpoint.name === provider.id);
            const existingProvider = userEndpoints.find(endpoint => endpoint.name === provider.id);
            return (
              <Grid item xs={12} sm={6} md={4} key={provider.id} display="flex" justifyContent="center">          
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
                      boxShadow: isLoadingProviders ? 2 : (editAllowed ? 4 : 2),
                      transform: isLoadingProviders ? 'none' : (editAllowed ? 'translateY(-4px)' : 'none'),
                      borderColor: isLoadingProviders ? 'divider' : (editAllowed ? 'primary.main' : 'divider'),
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
                    <Tooltip
                      title={
                        !editAllowed && !isConfigured
                          ? "Contact your organization owner to enable this provider"
                          : ""
                      }
                      arrow
                      placement="top"
                    >
                      <span>
                        <Button
                          size="small"
                          variant={isConfigured ? 'outlined' : 'text'}
                          color={isConfigured ? 'success' : 'secondary'}
                          onClick={() => handleOpenDialog(provider)}
                          startIcon={isConfigured ? <CheckCircleIcon /> : <AddCircleOutlineIcon />}
                          disabled={!editAllowed && !isConfigured}
                        >
                          {isConfigured ? 'Connected' : 'Connect'}
                        </Button>
                      </span>
                    </Tooltip>
                  </CardActions>
                </Card>                
              </Grid>
            );
          })}
        </Grid>
        {selectedProvider && editAllowed && (
          <AddProviderDialog
            orgId={org?.id || ''}
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