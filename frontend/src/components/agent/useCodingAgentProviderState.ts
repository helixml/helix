import { useEffect, useMemo, useRef } from 'react'
import { useClaudeSubscriptions } from '../account/ClaudeSubscriptionConnect'
import { useListProviders } from '../../services/providersService'
import { CodingAgentFormValue } from './CodingAgentForm'

/**
 * Fetches provider and subscription state for CodingAgentForm, and auto-selects
 * the correct runtime/mode once when data first loads.
 *
 * Called internally by CodingAgentForm — parents no longer need to compute or
 * pass hasAnthropicProvider / hasClaudeSubscription.
 */
export function useCodingAgentProviderState(
  value: CodingAgentFormValue,
  onChange: (value: CodingAgentFormValue) => void,
) {
  const { data: providerEndpoints } = useListProviders({ loadModels: false })
  const { data: claudeSubscriptions } = useClaudeSubscriptions()

  const hasAnthropicProvider = useMemo(() => {
    if (!providerEndpoints) return false
    return providerEndpoints.some(p => p.name === 'anthropic')
  }, [providerEndpoints])

  const hasClaudeSubscription = (claudeSubscriptions?.length ?? 0) > 0

  // Use a primitive so we stay within the "only primitives in deps" rule.
  const providerEndpointsLoaded = providerEndpoints !== undefined

  // Auto-select the correct runtime/mode once, when provider data first resolves.
  // The ref guard ensures this never overrides an explicit user selection.
  const hasAutoSelected = useRef(false)
  useEffect(() => {
    if (hasAutoSelected.current) return
    if (!providerEndpointsLoaded) return
    hasAutoSelected.current = true
    if (hasAnthropicProvider) {
      onChange({ ...value, codeAgentRuntime: 'claude_code', claudeCodeMode: 'api_key' })
    } else if (hasClaudeSubscription) {
      onChange({ ...value, codeAgentRuntime: 'claude_code', claudeCodeMode: 'subscription' })
    }
  }, [hasAnthropicProvider, hasClaudeSubscription, providerEndpointsLoaded])

  return { hasAnthropicProvider, hasClaudeSubscription }
}
