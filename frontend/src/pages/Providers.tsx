import React, { useState, useMemo } from 'react';
import { Box, Grid, Card, CardHeader, CardContent, CardActions, Avatar, Typography, Button, Tooltip, Divider, Alert } from '@mui/material';
import Container from '@mui/material/Container';
import Page from '../components/system/Page';
import AddCircleOutlineIcon from '@mui/icons-material/AddCircleOutline';
import CheckCircleIcon from '@mui/icons-material/CheckCircle';
import AddProviderDialog from '../components/providers/AddProviderDialog';

import { useListProviders, useDetectLocalProviders, useCreateProviderEndpoint } from '../services/providersService';
import { TypesProviderEndpointType } from '../api/api';
import CircularProgress from '@mui/material/CircularProgress';
import { Server } from 'lucide-react';
import { useGetOrgByName } from '../services/orgService';

import { PROVIDERS, Provider } from '../components/providers/types';
import CustomLogo from '../components/providers/logos/custom';
import useRouter from '../hooks/useRouter';
import useAccount from '../hooks/useAccount';
import AnthropicLogo from '../components/providers/logos/anthropic';
import OpenAILogo from '../components/providers/logos/openai';
import ClaudeSubscriptionConnect, { useClaudeSubscriptions } from '../components/account/ClaudeSubscriptionConnect';
import CodexSubscriptionConnect from '../components/account/CodexSubscriptionConnect';
import { useCodexSubscriptions } from '../services/codexSubscriptionsService';
import { getTokenExpiryStatus } from '../components/account/claudeSubscriptionUtils';
import LMStudioModels from '../components/providers/LMStudioModels';

const Providers: React.FC = () => {
  const router = useRouter()
  const account = useAccount()
  const [selectedProvider, setSelectedProvider] = useState<Provider | null>(null);
  const [dialogOpen, setDialogOpen] = useState(false);
  const [localModelsEndpointId, setLocalModelsEndpointId] = useState<string | null>(null);
  const [connectingDetected, setConnectingDetected] = useState<string>("");
  const isMacDesktop = account.serverConfig?.edition === "mac-desktop";
  const { data: detectedProviders } = useDetectLocalProviders(true);
  const createProvider = useCreateProviderEndpoint();

  const orgName = router.params.org_id

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
  const { data: codexSubscriptions } = useCodexSubscriptions()
  const hasCodexSubscription = (codexSubscriptions?.length ?? 0) > 0

  const pageProps = {
    breadcrumbTitle: 'AI providers',
    breadcrumbParent: {
      title: 'Organizations',
      routeName: 'orgs',
      useOrgRouter: false,
    },
    breadcrumbShowHome: true,
    orgBreadcrumbs: true,
    topbarContent: null,
  }

  // If providers management is disabled and user is not admin, show message
  if (!providersManagementEnabled && !account.admin) {
    return (
      <Page {...pageProps}>
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

  const handleOpenDialog = (provider: Provider, existingEndpoint?: any) => {
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

  // All endpoints (user + global) — used for checking if a provider is configured
  const allEndpoints = providerEndpoints;

  // Local inference servers (LM Studio, Ollama) — check all endpoints
  const localEndpoints = allEndpoints.filter(e =>
    e.name === 'lmstudio' || e.name === 'ollama' || e.name?.includes('lmstudio') || e.name?.includes('ollama')
  );

  // User-created custom endpoints: anything whose name doesn't match a predefined PROVIDERS id.
  const knownProviderIds = new Set(PROVIDERS.map(p => p.id));
  const customEndpoints = userEndpoints.filter(e => e.name && !knownProviderIds.has(e.name));

  // Detected but not yet connected local providers
  const unconnectedDetected = (detectedProviders || []).filter(
    dp => !allEndpoints.some(e => e.name === dp.server_type)
  );

  const handleConnectDetected = async (dp: { server_type: string; base_url: string; name: string }) => {
    setConnectingDetected(dp.server_type);
    try {
      await createProvider.mutateAsync({
        name: dp.server_type,
        base_url: dp.base_url,
        api_key: "",
        endpoint_type: TypesProviderEndpointType.ProviderEndpointTypeGlobal,
        owner: "system",
        owner_type: "system" as any,
      });
    } finally {
      setConnectingDetected("");
    }
  };

  return (
    <Page {...pageProps}>
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

        {/* Detected local servers banner */}
        {unconnectedDetected.length > 0 && (
          <Box sx={{ mb: 4 }}>
            {unconnectedDetected.map((dp) => (
              <Alert
                key={dp.server_type}
                severity="success"
                icon={<Server size={20} />}
                sx={{
                  mb: 1,
                  border: '1px solid rgba(0,232,145,0.3)',
                  bgcolor: 'rgba(0,232,145,0.05)',
                  '& .MuiAlert-icon': { color: '#00e891' },
                }}
                action={
                  <Button
                    size="small"
                    variant="contained"
                    disabled={connectingDetected === dp.server_type}
                    onClick={() => handleConnectDetected(dp)}
                    sx={{
                      bgcolor: '#00e891', color: '#000',
                      '&:hover': { bgcolor: '#00cc7a' },
                      textTransform: 'none',
                    }}
                  >
                    {connectingDetected === dp.server_type ? <CircularProgress size={16} sx={{ color: '#000' }} /> : 'Connect'}
                  </Button>
                }
              >
                <strong>{dp.name}</strong> detected on this machine with {dp.models.length} model{dp.models.length !== 1 ? 's' : ''} available
              </Alert>
            ))}
          </Box>
        )}

        <Typography variant="h6" sx={{ mb: 1.5 }}>
          Subscriptions
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Connect an Anthropic or ChatGPT subscription for coding agents in desktop sessions.
        </Typography>
        <Grid container spacing={3} justifyContent="left" sx={{ mb: 4 }}>
          <Grid item xs={12} sm={6} display="flex" justifyContent="center">
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
                title="Anthropic"
                titleTypographyProps={{ variant: 'h6', align: 'center' }}
              />
              <CardContent sx={{ flexGrow: 1, textAlign: 'center' }}>
                <Typography variant="body2" color="text.secondary">
                  Use your Claude account with Claude Code inside desktop agents. Not an API key provider.
                </Typography>
              </CardContent>
              <CardActions sx={{ justifyContent: 'center', pb: 2, minHeight: 52, '& .MuiButton-root': { minWidth: 104, height: 36 } }}>
                <ClaudeSubscriptionConnect variant="button" />
              </CardActions>
            </Card>
          </Grid>
          <Grid item xs={12} sm={6} display="flex" justifyContent="center">
            <Card sx={{ width: 320, height: '100%', display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', boxShadow: 2, borderStyle: 'dashed', borderWidth: 1, borderColor: hasCodexSubscription ? 'success.main' : 'divider' }}>
              <CardHeader
                avatar={<Avatar sx={{ bgcolor: 'white', width: 56, height: 56 }}><OpenAILogo style={{ width: 40, height: 40 }} /></Avatar>}
                title="ChatGPT"
                titleTypographyProps={{ variant: 'h6', align: 'center' }}
              />
              <CardContent sx={{ flexGrow: 1, textAlign: 'center' }}>
                <Typography variant="body2" color="text.secondary">
                  Use your ChatGPT account with Codex CLI inside desktop agents. Not an API key provider.
                </Typography>
              </CardContent>
              <CardActions sx={{ justifyContent: 'center', pb: 2, minHeight: 52, '& .MuiButton-root': { minWidth: 104, height: 36 } }}>
                <CodexSubscriptionConnect orgId={org?.id} />
              </CardActions>
            </Card>
          </Grid>
        </Grid>

        {localEndpoints.length > 0 && (
          <>
            <Divider sx={{ mb: 3 }} />
            <Typography variant="h6" sx={{ mb: 1.5 }}>
              Local AI Servers
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
              Models running on this machine via LM Studio or Ollama.
            </Typography>
            {localEndpoints.map((ep) => (
              <Box key={ep.id} sx={{ mb: 3 }}>
                <Box sx={{ display: "flex", alignItems: "center", gap: 1, mb: 1.5 }}>
                  <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                    {ep.name === 'lmstudio' ? 'LM Studio' : ep.name === 'ollama' ? 'Ollama' : ep.name}
                  </Typography>
                  {ep.status === 'error' ? (
                    <Alert severity="error" sx={{ py: 0, px: 1, fontSize: '0.75rem' }}>{ep.error || 'Connection error'}</Alert>
                  ) : (
                    <Typography variant="caption" sx={{ color: '#00e891' }}>Connected</Typography>
                  )}
                  <Typography variant="caption" sx={{ color: 'text.disabled', fontFamily: 'monospace' }}>{ep.base_url}</Typography>
                </Box>
                {ep.id && <LMStudioModels endpointId={ep.id} />}
              </Box>
            ))}
          </>
        )}

        <Divider sx={{ mb: 3 }} />

        <Typography variant="h6" sx={{ mb: 1.5 }}>
          AI Providers
        </Typography>
        <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
          Add API keys for chat, Zed Agent, Qwen Code, and other AI features.
        </Typography>
        <Grid container spacing={3} justifyContent="left">
          {PROVIDERS.map((provider) => {
            // The custom provider tile always opens a fresh "Add" dialog — many custom
            // providers can coexist, and each existing one is shown as its own card below.
            // This section manages keys owned by the current user or organization.
            // Synthetic global providers have id "-" and are configured by the server
            // environment, so they cannot be disconnected from this dialog.
            const existingProvider = provider.is_custom ? undefined : userEndpoints.find(
              endpoint => endpoint.name === provider.id || endpoint.name === provider.id.replace('user/', '')
            );
            const isConfigured = !!existingProvider;
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
        {selectedProvider && editAllowed && (
          <AddProviderDialog
            orgId={org?.id || ''}
            open={dialogOpen}
            onClose={handleCloseDialog}
            provider={selectedProvider}
            existingProvider={userEndpoints.find(endpoint =>
              endpoint.name === selectedProvider.id ||
              endpoint.name === selectedProvider.id.replace('user/', '')
            )}
          />
        )}
      </Container>
    </Page>
  );
};

export default Providers;
