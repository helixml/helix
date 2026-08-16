import { FC, ReactNode } from 'react'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import CircularProgress from '@mui/material/CircularProgress'
import FormControl from '@mui/material/FormControl'
import FormControlLabel from '@mui/material/FormControlLabel'
import MenuItem from '@mui/material/MenuItem'
import Radio from '@mui/material/Radio'
import RadioGroup from '@mui/material/RadioGroup'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import {
  TypesCodeAgentCredentialType,
  TypesOrgCodeAgentProviderStatus,
  TypesOrgCodeAgentProviderUpdate,
  TypesProviderEndpoint,
} from '../../api/api'
import { getAgentHarnessLabel } from '../agent/AgentHarness'
import CodeAgentProviderRow, { ProviderHealth } from './CodeAgentProviderRow'
import { providerRef } from '../create/AdvancedModelPicker'

const SUBSCRIPTION = TypesCodeAgentCredentialType.CodeAgentCredentialTypeSubscription
const API_KEY = TypesCodeAgentCredentialType.CodeAgentCredentialTypeAPIKey

/**
 * Health is about *this viewer*, matching the API's viewer-scoped `available`.
 * A provider the org enabled but the viewer cannot use is "attention", not
 * "ready" — telling them it is ready would just move the failure into the run.
 */
function healthOf(provider: TypesOrgCodeAgentProviderStatus): ProviderHealth {
  if (!provider.enabled) return 'unavailable'
  return provider.available ? 'ready' : 'attention'
}

function statusTextOf(provider: TypesOrgCodeAgentProviderStatus): string {
  if (!provider.enabled) return 'Disabled for this organization'
  if (!provider.available) return provider.unavailable_reason || 'Needs attention'
  if (provider.credential_type === SUBSCRIPTION) {
    return 'Ready — authenticating with your own subscription'
  }
  return 'Ready — routed through the organization API key'
}

/**
 * The organization's coding-agent allow list.
 *
 * Only the credential mode and (for API-key mode) the provider live here.
 * Subscriptions are deliberately absent: they belong to individual users, so
 * this page enables the *mode* and each member connects their own account.
 */
const CodeAgentProvidersSection: FC<{
  providers: TypesOrgCodeAgentProviderStatus[]
  endpoints: TypesProviderEndpoint[]
  loading?: boolean
  saving?: boolean
  readOnly?: boolean
  // Rendered inside a runtime's Credentials block when it is in subscription
  // mode. This is where each member connects their own account, so it belongs
  // next to the mode that needs it rather than in a separate card.
  subscriptionAction?: (runtime: string) => ReactNode
  onChange: (update: TypesOrgCodeAgentProviderUpdate) => void
}> = ({ providers, endpoints, loading = false, saving = false, readOnly = false, subscriptionAction, onChange }) => {
  const patch = (
    provider: TypesOrgCodeAgentProviderStatus,
    changes: Partial<TypesOrgCodeAgentProviderUpdate>,
  ) => {
    onChange({
      runtime: provider.runtime!,
      enabled: provider.enabled ?? false,
      credential_type: provider.credential_type,
      provider_endpoint_id: provider.provider_endpoint_id,
      default_model: provider.default_model,
      ...changes,
    })
  }

  if (loading) {
    return (
      <Stack direction="row" alignItems="center" spacing={1.5} sx={{ py: 3 }}>
        <CircularProgress size={16} />
        <Typography variant="body2" color="text.secondary">
          Loading coding agents…
        </Typography>
      </Stack>
    )
  }

  return (
    <Box>
      {providers.map((provider) => {
        const label = getAgentHarnessLabel(provider.runtime)
        // A runtime that cannot use a subscription is API-key only; treat an
        // unset credential_type as API key so the radio has a definite value.
        const credentialType = provider.credential_type === SUBSCRIPTION ? SUBSCRIPTION : API_KEY
        const subscriptionMode = credentialType === SUBSCRIPTION

        return (
          <CodeAgentProviderRow
            key={provider.runtime}
            runtime={provider.runtime || ''}
            health={healthOf(provider)}
            status={statusTextOf(provider)}
            enabled={provider.enabled ?? false}
            disabled={readOnly || saving}
            badge={provider.supports_subscription && subscriptionMode ? (
              <Chip label="Subscription" size="small" variant="outlined" sx={{ height: 18, fontSize: '0.65rem' }} />
            ) : undefined}
            onToggle={(enabled) => patch(provider, { enabled })}
          >
            <Stack spacing={2}>
              {provider.supports_subscription && (
                <Box>
                  <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                    Credentials
                  </Typography>
                  <FormControl>
                    <RadioGroup
                      value={credentialType}
                      onChange={(event) => patch(provider, {
                        credential_type: event.target.value as TypesCodeAgentCredentialType,
                        // Switching to a subscription drops the pinned endpoint:
                        // the agent talks to the vendor directly, not the proxy.
                        provider_endpoint_id: event.target.value === SUBSCRIPTION
                          ? undefined
                          : provider.provider_endpoint_id,
                      })}
                    >
                      <FormControlLabel
                        value={SUBSCRIPTION}
                        disabled={readOnly || saving}
                        control={<Radio size="small" />}
                        label={<Typography variant="body2">Members use their own subscription</Typography>}
                      />
                      <FormControlLabel
                        value={API_KEY}
                        disabled={readOnly || saving}
                        control={<Radio size="small" />}
                        label={<Typography variant="body2">Organization API key</Typography>}
                      />
                    </RadioGroup>
                  </FormControl>
                  {subscriptionMode && subscriptionAction && (
                    <Box sx={{ mt: 1 }}>{subscriptionAction(provider.runtime || '')}</Box>
                  )}
                  {subscriptionMode && !provider.viewer_has_subscription && (
                    <Alert severity="info" variant="outlined" sx={{ mt: 1, py: 0.25 }}>
                      Each member connects their own {label} account. You have not connected
                      yours yet, so {label} will not run for you until you do.
                    </Alert>
                  )}
                </Box>
              )}

              {!subscriptionMode && (
                <Box>
                  <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 1 }}>
                    Provider
                  </Typography>
                  <FormControl fullWidth size="small" sx={{ maxWidth: 380 }}>
                    <Select
                      displayEmpty
                      value={provider.provider_endpoint_id || ''}
                      disabled={readOnly || saving}
                      onChange={(event) => patch(provider, {
                        provider_endpoint_id: event.target.value || undefined,
                        // Endpoint changed, so a model pinned from the old one
                        // no longer necessarily exists.
                        default_model: undefined,
                      })}
                      renderValue={(value) => {
                        if (!value) {
                          return <Typography variant="body2" color="text.secondary">Select a provider</Typography>
                        }
                        const endpoint = endpoints.find((candidate) => candidate.id === value)
                        return <Typography variant="body2">{endpoint?.name || value}</Typography>
                      }}
                    >
                      {endpoints.map((endpoint) => (
                        <MenuItem key={endpoint.id} value={endpoint.id}>
                          <Typography variant="body2">{endpoint.name || providerRef(endpoint)}</Typography>
                        </MenuItem>
                      ))}
                    </Select>
                  </FormControl>
                  <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.75 }}>
                    Tasks using {label} route through this provider, and the task picker offers
                    only its models.
                  </Typography>
                </Box>
              )}
            </Stack>
          </CodeAgentProviderRow>
        )
      })}
    </Box>
  )
}

export default CodeAgentProvidersSection
