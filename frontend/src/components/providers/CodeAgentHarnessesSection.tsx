import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Stack from '@mui/material/Stack'
import Switch from '@mui/material/Switch'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { Info } from 'lucide-react'

import {
  TypesOrgCodeAgentHarnessStatus,
  TypesOrgCodeAgentHarnessUpdate,
  TypesProviderEndpoint,
} from '../../api/api'
import CodeAgentHarnessRow, { HARNESS_SWITCH_SLOT_SX, HarnessHealth } from './CodeAgentHarnessRow'
import { providerRef } from '../create/AdvancedModelPicker'
import { getAgentHarnessLabel } from '../agent/AgentHarness'
import {
  providerEndpointIsConnected,
  providersForCodeAgentRuntime,
  requiredProviderNameForRuntime,
} from '../../utils/codeAgentProviders'

const PROVIDERS_HELP =
  'Enabled API providers expose their models in the task chat, where a model is selected.'

const CodeAgentHarnessesSection: FC<{
  harnesses: TypesOrgCodeAgentHarnessStatus[]
  endpoints: TypesProviderEndpoint[]
  loading?: boolean
  readOnly?: boolean
  subscriptionAction?: (runtime: string) => ReactNode
  subscriptionIdentity?: (runtime: string) => string | undefined
  onChange: (update: TypesOrgCodeAgentHarnessUpdate) => void
}> = ({
  harnesses,
  endpoints,
  loading = false,
  readOnly = false,
  subscriptionAction,
  subscriptionIdentity,
  onChange,
}) => {
  if (loading) {
    return (
      <Stack direction="row" alignItems="center" spacing={1.5} sx={{ py: 3 }}>
        <CircularProgress size={16} />
        <Typography variant="body2" color="text.secondary">
          Loading coding harnesses…
        </Typography>
      </Stack>
    )
  }

  return (
    <Box sx={{ borderTop: '1px solid', borderColor: 'divider' }}>
      {harnesses.map((harness) => {
        const compatibleEndpoints = providersForCodeAgentRuntime(endpoints, harness.runtime)
        const connectedEndpoints = compatibleEndpoints.filter(providerEndpointIsConnected)
        const allowedProviderRefs = harness.provider_refs == null
          ? null
          : new Set(harness.provider_refs)
        const allowedEndpoints = allowedProviderRefs == null
          ? connectedEndpoints
          : connectedEndpoints.filter((endpoint) => allowedProviderRefs.has(providerRef(endpoint)))
        const subscriptionEnabled = harness.subscription_enabled !== false
        const viewerHasSubscription = !!harness.viewer_has_subscription
        const hasSubscription = harness.supports_subscription
          && subscriptionEnabled
          && viewerHasSubscription
        const harnessLabel = getAgentHarnessLabel(harness.runtime || '')
        const health: HarnessHealth = !harness.enabled
          ? 'unavailable'
          : allowedEndpoints.length > 0 || hasSubscription
            ? 'ready'
            : 'attention'
        const status = !harness.enabled
          ? 'Disabled'
          : health === 'ready'
            ? 'Ready'
            : 'No providers available'
        const subscriptionActionNode = harness.supports_subscription
          ? subscriptionAction?.(harness.runtime || '')
          : null
        const requiredProviderName = requiredProviderNameForRuntime(harness.runtime)

        return (
          <CodeAgentHarnessRow
            key={harness.runtime}
            runtime={harness.runtime || ''}
            health={health}
            status={status}
            enabled={harness.enabled ?? false}
            disabled={readOnly}
            onToggle={(enabled) => onChange({ runtime: harness.runtime!, enabled })}
          >
            <Stack spacing={2}>
              {harness.supports_subscription && (
                <Stack
                  direction="row"
                  alignItems="center"
                  justifyContent="space-between"
                  spacing={1}
                >
                  <Box sx={{ minWidth: 0, flex: 1 }}>
                    <Typography variant="subtitle2" color="text.secondary">
                      {harness.runtime === 'claude_code' ? 'Claude subscription' : 'ChatGPT subscription'}
                    </Typography>
                    <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.25 }}>
                      {subscriptionIdentity?.(harness.runtime || '')
                        || (viewerHasSubscription ? 'Connected' : 'Not connected')}
                    </Typography>
                  </Box>
                  <Stack
                    direction="row"
                    alignItems="center"
                    spacing={1}
                    sx={{
                      flexShrink: 0,
                      '& .MuiButton-root': { textTransform: 'none', whiteSpace: 'nowrap' },
                    }}
                  >
                    {subscriptionActionNode}
                    <Tooltip
                      title="Connect a subscription first"
                      disableHoverListener={viewerHasSubscription}
                      disableFocusListener={viewerHasSubscription}
                    >
                      <Box sx={HARNESS_SWITCH_SLOT_SX} component="span">
                        <Switch
                          color="secondary"
                          checked={subscriptionEnabled}
                          disabled={readOnly || !viewerHasSubscription}
                          inputProps={{
                            'aria-label': `${subscriptionEnabled ? 'Disable' : 'Enable'} subscription for ${harnessLabel}`,
                          }}
                          onChange={(_, enabled) => onChange({
                            runtime: harness.runtime!,
                            enabled: harness.enabled ?? false,
                            subscription_enabled: enabled,
                          })}
                        />
                      </Box>
                    </Tooltip>
                  </Stack>
                </Stack>
              )}

              <Box>
                <Stack direction="row" alignItems="center" spacing={0.25} sx={{ mb: 0.5 }}>
                  <Typography variant="subtitle2" color="text.secondary">
                    API providers
                  </Typography>
                  <Tooltip title={PROVIDERS_HELP}>
                    <IconButton
                      size="small"
                      aria-label={PROVIDERS_HELP}
                      sx={{
                        width: 24,
                        height: 24,
                        color: 'text.secondary',
                        '&:hover': { color: 'text.primary' },
                      }}
                    >
                      <Info size={14} />
                    </IconButton>
                  </Tooltip>
                </Stack>
                {compatibleEndpoints.length > 0 ? (
                  <Stack
                    sx={{
                      '& > *': {
                        py: 1.25,
                        minHeight: 44,
                        borderBottom: '1px solid',
                        borderColor: 'divider',
                      },
                      '& > *:last-child': { borderBottom: 'none' },
                    }}
                  >
                    {compatibleEndpoints.map((endpoint) => {
                      const ref = providerRef(endpoint)
                      const connected = providerEndpointIsConnected(endpoint)
                      const checked = connected
                        && (allowedProviderRefs == null || allowedProviderRefs.has(ref))
                      return (
                        <Stack
                          key={ref}
                          direction="row"
                          alignItems="center"
                          justifyContent="space-between"
                          spacing={1}
                        >
                          <Box sx={{ minWidth: 0 }}>
                            <Typography variant="body2">{endpoint.name || 'Unnamed provider'}</Typography>
                            {!connected && (
                              <Typography variant="caption" color="text.secondary">
                                Not connected
                              </Typography>
                            )}
                          </Box>
                          <Tooltip
                            title="Connect this provider with a valid API key first"
                            disableHoverListener={connected}
                            disableFocusListener={connected}
                          >
                            <Box sx={HARNESS_SWITCH_SLOT_SX} component="span">
                              <Switch
                                color="secondary"
                                checked={checked}
                                disabled={readOnly || !connected}
                                inputProps={{
                                  'aria-label': `${checked ? 'Disable' : 'Enable'} ${endpoint.name || 'unnamed provider'} for ${harnessLabel}`,
                                }}
                                onChange={(_, enabled) => {
                                  const current = harness.provider_refs == null
                                    // Preserve temporarily unavailable endpoints
                                    // when materializing the legacy all-providers policy.
                                    ? [...new Set(compatibleEndpoints.map(providerRef))]
                                    : harness.provider_refs.filter((candidate) =>
                                      compatibleEndpoints.some((endpoint) => providerRef(endpoint) === candidate))
                                  const next = enabled
                                    ? [...new Set([...current, ref])]
                                    : current.filter((candidate) => candidate !== ref)
                                  onChange({
                                    runtime: harness.runtime!,
                                    enabled: harness.enabled ?? false,
                                    provider_refs: next,
                                  })
                                }}
                              />
                            </Box>
                          </Tooltip>
                        </Stack>
                      )
                    })}
                  </Stack>
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    {requiredProviderName
                      ? `Add the ${requiredProviderName} provider with an API key to use API mode.`
                      : 'No API providers with available models are connected.'}
                  </Typography>
                )}
              </Box>
            </Stack>
          </CodeAgentHarnessRow>
        )
      })}
    </Box>
  )
}

export default CodeAgentHarnessesSection
