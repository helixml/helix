// DefaultAgentConfigPanel edits the org's Default Runtime. New orgs use
// the atomic agent.default object; legacy worker.* values remain readable.

import { FC, useEffect, useMemo, useState } from 'react'
import Button from '@mui/material/Button'
import Paper from '@mui/material/Paper'

import AgentConfigForm, { AgentConfigValue } from './BotRuntimeForm'
import LoadingSpinner from '../widgets/LoadingSpinner'
import useSnackbar from '../../hooks/useSnackbar'
import { useOrgCodeAgentHarnesses } from '../../services/codeAgentHarnessesService'
import {
  SettingsSpecDTO,
  useHelixOrgBase,
  useHelixOrgSettings,
  useSetHelixOrgSetting,
} from '../../services/helixOrgService'

const decodeStringValue = (v: string): string => {
  if (!v) return ''
  try {
    const parsed = JSON.parse(v)
    return typeof parsed === 'string' ? parsed : ''
  } catch {
    return v
  }
}

const decodeAgentConfig = (v: string): AgentConfigValue | undefined => {
  if (!v) return undefined
  try {
    const config = JSON.parse(v)
    return {
      runtime: config.code_agent_runtime ?? '',
      credentials: config.code_agent_credential_type ?? '',
      provider: config.provider ?? '',
      model: config.model ?? '',
      reasoning_effort: config.reasoning_effort ?? 'none',
    }
  } catch {
    return undefined
  }
}

const DefaultAgentConfigPanel: FC<{ disabled?: boolean }> = ({ disabled = false }) => {
  const { orgID } = useHelixOrgBase()
  const { data, isLoading } = useHelixOrgSettings()
  const { data: harnesses = [], isLoading: loadingHarnesses } = useOrgCodeAgentHarnesses(orgID, {
    enabled: !!orgID,
  })
  const setMut = useSetHelixOrgSetting()
  const snackbar = useSnackbar()

  const specByKey = useMemo(() => {
    const m = new Map<string, SettingsSpecDTO>()
    for (const s of data?.specs ?? []) m.set(s.key, s)
    return m
  }, [data])

  const initial: AgentConfigValue = decodeAgentConfig(specByKey.get('agent.default')?.value ?? '') ?? {
    runtime: decodeStringValue(specByKey.get('worker.runtime')?.value ?? '') || 'claude_code',
    credentials: decodeStringValue(specByKey.get('worker.credentials')?.value ?? '') || 'subscription',
    provider: decodeStringValue(specByKey.get('worker.provider')?.value ?? ''),
    model: decodeStringValue(specByKey.get('worker.model')?.value ?? ''),
    reasoning_effort: 'none',
  }

  const [value, setValue] = useState<AgentConfigValue>(initial)
  const [dirty, setDirty] = useState(false)

  // Re-seed local state when the loaded data lands or refreshes.
  useEffect(() => {
    if (!data) return
    setValue(initial)
    setDirty(false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const handlePatch = (patch: Partial<AgentConfigValue>) => {
    if (disabled) return
    setValue((current) => ({ ...current, ...patch }))
    setDirty(true)
  }

  const handleSave = () => {
    setMut
      .mutateAsync({ key: 'agent.default', value: JSON.stringify({
        code_agent_runtime: value.runtime,
        code_agent_credential_type: value.credentials,
        provider: value.provider,
        model: value.model,
        reasoning_effort: value.reasoning_effort || 'none',
      }) })
      .then(() => {
        setDirty(false)
        snackbar.success('Default runtime saved')
      })
      .catch((e: any) => snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed'))
  }

  const subscriptionAvailability = {
    claude: harnesses.some((harness) => harness.runtime === 'claude_code'
      && harness.enabled
      && harness.subscription_enabled === true
      && harness.viewer_has_subscription),
    codex: harnesses.some((harness) => harness.runtime === 'codex_cli'
      && harness.enabled
      && harness.subscription_enabled === true
      && harness.viewer_has_subscription),
  }

  return (
    <Paper variant="outlined" sx={{ p: 3 }}>
      {isLoading || loadingHarnesses
        ? <LoadingSpinner />
        : <>
            <AgentConfigForm
              value={value}
              onChange={handlePatch}
              showReasoningEffort
              disabled={disabled}
              subscriptionAvailability={subscriptionAvailability}
            />
            <Button
              variant="contained"
              onClick={handleSave}
              disabled={disabled || !dirty || setMut.isPending}
              sx={{ mt: 2 }}
            >
              {setMut.isPending ? 'Saving...' : 'Save Default Runtime'}
            </Button>
          </>}
    </Paper>
  )
}

export default DefaultAgentConfigPanel
