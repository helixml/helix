import React, { useState, useMemo } from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Typography, Button, Tooltip, Divider, Alert } from '@mui/material';
import Container from '@mui/material/Container';
import Page from '../components/system/Page';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import AddProviderDialog from '../components/providers/AddProviderDialog';

import { useListProviders } from '../services/providersService';
import { useGetOrgByName } from '../services/orgService';

import { PROVIDERS, Provider } from '../components/providers/types';
import CustomLogo from '../components/providers/logos/custom';
import useRouter from '../hooks/useRouter';
import useAccount from '../hooks/useAccount';
import AnthropicLogo from '../components/providers/logos/anthropic';
import ClaudeSubscriptionConnect, { useClaudeSubscriptions } from '../components/account/ClaudeSubscriptionConnect';
import { getTokenExpiryStatus } from '../components/account/claudeSubscriptionUtils';
import WorkerRuntimePanel from '../components/helix-org/WorkerRuntimePanel';

const Providers: React.FC = () => {
  const router = useRouter()
  const account = useAccount()
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);

  const orgName = router.params.org_id

  // helix-org alpha: only then is the worker-runtime config (and its
  // settings endpoint) available for this org.
  const helixOrgEnabled = account.user?.alpha_features?.includes('helix-org') ?? false

  // Check if providers management is enabled
  const providersManagementEnabled = account.serverConfig.providers_management_enabled

  // Get org if orgName is set (hooks must be called before any early returns)
  const { data: org, isLoading: isLoadingOrg } = useGetOrgByName(orgName, orgName !== undefined)

  // Get provider endpoints. Load models so the API populates `status` and
  // `error` for each endpoint — that's how a misconfigured provider (e.g.
  // wrong API key on NVIDIA NIM) surfaces a human-readable failure on this
  // page instead of looking "Connected" while silently broken.
  const { data: providerEndpoints = [], isLoading: isLoadingProviders, refetch: loadData } = useListProviders({
    loadModels: true,
    orgId: org?.id,
    enabled: !isLoadingOrg,
  });

  // Claude subscription state (must be called before any early returns)
  const { data: claudeSubscriptions } = useClaudeSubscriptions()
  const hasClaudeSubscription = (claudeSubscriptions?.length ?? 0) > 0
  const claudeIsSetupToken = hasClaudeSubscription && claudeSubscriptions![0].credential_type === 'setup_token'
  const claudeExpiry = hasClaudeSubscription && !claudeIsSetupToken
    ? getTokenExpiryStatus(claudeSubscriptions![0].access_token_expires_at)
    : null
  const claudeIsExpired = claudeExpiry?.isExpired ?? false

  // If providers management is disabled and user is not admin, show message
  if (!providersManagementEnabled && !account.admin) {
    return (
      <Page breadcrumbTitle="Providers" topbarContent={null}>
        <Container maxWidth="md" sx={{ mt: 10, mb: 6, display: 'flex', flexDirection: 'column', alignItems: 'center' }}>
          <Typography variant="h4" sx={{ mb: 2, fontWeight: 600 }}>
            AI Providers
          </Typography>
          <Typography variant="body1" color="text.secondary" sx={{ mb: 4, textAlign: 'center' }}>
            Provider management is not enabled on this installation.
          </Typography>
        </Container>
      </Page>
    )
  }


  let editAllowed = false

  // If we are not in an org context - we can perform all actions
  if (orgName === undefined) {
    editAllowed = true
  } else {
    // Otherwise we need to check if we are an org admin
    editAllowed = account.isOrgAdmin
  }

  const checkLoginStatus = (): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    return true
  }

  const handleOpenDialog = (provider: Provider) => {
    if(!checkLoginStatus()) return
    
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

  // User-created custom endpoints: anything whose name doesn't match a predefined PROVIDERS id.
  const knownProviderIds = new Set(PROVIDERS.map(p => p.id));
  const customEndpoints = userEndpoints.filter(e => e.name && !knownProviderIds.has(e.name));

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

        {/* Claude Subscription Section */}
        <Typography variant="h6" sx={{ mb: 1.5 }}>
          Claude Subscription
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Sign in with your Claude account to use Claude Code as the coding agent in desktop sessions.
        </Typography>
        <Grid container spacing={3} justifyContent="left" sx={{ mb: 4 }}>
          <Grid item xs={12} sm={6} md={4} display="flex" justifyContent="center">
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
                borderColor: hasClaudeSubscription
                  ? (claudeIsExpired ? 'error.main' : claudeExpiry?.isExpiringSoon ? 'warning.main' : 'success.main')
                  : 'divider',
                opacity: hasClaudeSubscription ? 1 : 0.85,
                transition: 'all 0.2s',
                '&:hover': {
                  boxShadow: editAllowed ? 4 : 2,
                  transform: editAllowed ? 'translateY(-4px)' : 'none',
                  borderColor: editAllowed ? 'primary.main' : 'divider',
                },
              }}
            >
              <CardHeader
                avatar={
                  <Avatar sx={{ bgcolor: 'white', width: 56, height: 56 }}>
                    <AnthropicLogo style={{ width: 40, height: 40 }} />
                  </Avatar>
                }
                title="Claude Subscription"
                titleTypographyProps={{ variant: 'h6', align: 'center' }}
              />
              <CardContent sx={{ flexGrow: 1, textAlign: 'center' }}>
                <Typography variant="body2" color="text.secondary">
                  Use your Claude account with Claude Code inside desktop agents. Not an API key provider.
                </Typography>
              </CardContent>
              <CardActions sx={{ justifyContent: 'center', pb: 2 }}>
                <ClaudeSubscriptionConnect variant="button" />
              </CardActions>
            </Card>
          </Grid>
        </Grid>

        <Divider sx={{ mb: 3 }} />

        <Typography variant="h6" sx={{ mb: 1.5 }}>
          API Key Providers
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Add API keys for chat, Zed Agent, Qwen Code, and other AI features.
        </Typography>
        <Grid container spacing={3} justifyContent="left">
          {PROVIDERS.map((provider) => {
            // The custom provider tile always opens a fresh "Add" dialog — many custom
            // providers can coexist, and each existing one is shown as its own card below.
            const isConfigured = !provider.is_custom && userEndpoints.some(endpoint => endpoint.name === provider.id);
            const existingProvider = provider.is_custom ? undefined : userEndpoints.find(endpoint => endpoint.name === provider.id);
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
                    {existingProvider?.status === 'error' && existingProvider.error && (
                      <Alert
                        severity="error"
                        variant="outlined"
                        sx={{ mt: 2, textAlign: 'left', wordBreak: 'break-word' }}
                      >
                        {existingProvider.error}
                      </Alert>
                    )}
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
                          color={isConfigured ? (existingProvider?.status === 'error' ? 'error' : 'success') : 'secondary'}
                          onClick={() => handleOpenDialog(provider)}
                          startIcon={isConfigured ? <CheckCircleIcon /> : <AddCircleOutlineIcon />}
                          disabled={!editAllowed && !isConfigured}
                        >
                          {isConfigured ? (existingProvider?.status === 'error' ? 'Fix Connection' : 'Connected') : 'Connect'}
                        </Button>
                      </span>
                    </Tooltip>
                  </CardActions>
                </Card>
              </Grid>
            );
          })}
          {customEndpoints.map((endpoint) => {
            const customCardProvider: Provider = {
              id: endpoint.name || '',
              alias: [],
              name: endpoint.name || 'Custom Provider',
              description: endpoint.description || 'Custom OpenAI-compatible provider.',
              logo: CustomLogo,
              base_url: endpoint.base_url || '',
              configurable_base_url: true,
              optional_api_key: true,
              is_custom: true,
              setup_instructions: 'Update the base URL or API key for this custom provider.',
            };
            return (
              <Grid item xs={12} sm={6} md={4} key={`custom-${endpoint.id}`} display="flex" justifyContent="center">
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
                    opacity: isLoadingProviders ? 0.5 : 1,
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
                        <CustomLogo style={{ width: 40, height: 40 }} />
                      </Avatar>
                    }
                    title={endpoint.name}
                    titleTypographyProps={{ variant: 'h6', align: 'center' }}
                  />
                  <CardContent sx={{ flexGrow: 1, textAlign: 'center' }}>
                    <Typography variant="body2" color="text.secondary" sx={{ wordBreak: 'break-all' }}>
                      {endpoint.base_url}
                    </Typography>
                    {endpoint.status === 'error' && endpoint.error && (
                      <Alert
                        severity="error"
                        variant="outlined"
                        sx={{ mt: 2, textAlign: 'left', wordBreak: 'break-word' }}
                      >
                        {endpoint.error}
                      </Alert>
                    )}
                  </CardContent>
                  <CardActions sx={{ justifyContent: 'center', pb: 2 }}>
                    <Button
                      size="small"
                      variant="outlined"
                      color={endpoint.status === 'error' ? 'error' : 'success'}
                      onClick={() => handleOpenDialog(customCardProvider)}
                      startIcon={<CheckCircleIcon />}
                      disabled={!editAllowed}
                    >
                      {endpoint.status === 'error' ? 'Fix Connection' : 'Connected'}
                    </Button>
                  </CardActions>
                </Card>
              </Grid>
            );
          })}
        </Grid>

        {orgName && helixOrgEnabled && (
          <>
            <Divider sx={{ my: 4 }} />
            <Typography variant="h6" sx={{ mb: 1, fontWeight: 600 }}>
              Default Bot Runtime
            </Typography>
            <Typography variant="body1" color="text.secondary" sx={{ mb: 3 }}>
              How this org's Bots run by default — which runtime, and which provider/model they route through.
            </Typography>
            <WorkerRuntimePanel />
          </>
        )}

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