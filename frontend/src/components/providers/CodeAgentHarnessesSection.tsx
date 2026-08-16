import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import {
  TypesOrgCodeAgentHarnessStatus,
  TypesOrgCodeAgentHarnessUpdate,
  TypesProviderEndpoint,
} from '../../api/api'
import CodeAgentHarnessRow, { HarnessHealth } from './CodeAgentHarnessRow'

function endpointIsRunnable(endpoint: TypesProviderEndpoint): boolean {
  return (endpoint.available_models || []).some((model) => model.enabled
    && (!model.type || model.type === 'chat' || model.type === 'text'))
}

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
        const runnableEndpoints = endpoints.filter(endpointIsRunnable)
        const hasSubscription = harness.supports_subscription && harness.viewer_has_subscription
        const health: HarnessHealth = !harness.enabled
          ? 'unavailable'
          : runnableEndpoints.length > 0 || hasSubscription
            ? 'ready'
            : 'attention'
        const status = !harness.enabled
          ? 'Disabled for this organization'
          : health === 'ready'
            ? 'Ready to configure when creating a task'
            : 'Enabled, but no provider or subscription is available'

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
                <Box>
                  <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 0.75 }}>
                    Subscription
                  </Typography>
                  <Typography variant="body2" sx={{ mb: 1 }}>
                    {subscriptionIdentity?.(harness.runtime || '')
                      || (harness.viewer_has_subscription
                        ? 'Connected for your account'
                        : 'No subscription connected for your account')}
                  </Typography>
                  {subscriptionAction?.(harness.runtime || '')}
                </Box>
              )}

              <Box>
                <Typography variant="subtitle2" color="text.secondary" sx={{ mb: 0.75 }}>
                  API providers available to tasks
                </Typography>
                {runnableEndpoints.length > 0 ? (
                  <Stack spacing={0.5}>
                    {runnableEndpoints.map((endpoint) => (
                      <Stack key={endpoint.id || endpoint.name} direction="row" alignItems="center" spacing={1}>
                        <Box
                          aria-hidden="true"
                          sx={{ width: 7, height: 7, borderRadius: '50%', bgcolor: 'success.main' }}
                        />
                        <Typography variant="body2">{endpoint.name || 'Unnamed provider'}</Typography>
                      </Stack>
                    ))}
                  </Stack>
                ) : (
                  <Typography variant="body2" color="text.secondary">
                    No API providers with available models are connected.
                  </Typography>
                )}
                <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 1 }}>
                  The provider and model are selected in the task chat.
                </Typography>
              </Box>
            </Stack>
          </CodeAgentHarnessRow>
        )
      })}
    </Box>
  )
}

export default CodeAgentHarnessesSection
