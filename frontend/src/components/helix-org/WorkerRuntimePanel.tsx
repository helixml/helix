// DefaultAgentConfigPanel edits the org's Default Runtime. New orgs use
// the atomic agent.default object; legacy worker.* values remain readable.

import { FC, useEffect, useMemo, useState } from 'react'
import Button from '@mui/material/Button'
import Paper from '@mui/material/Paper'
import Typography from '@mui/material/Typography'

import AgentConfigForm, { AgentConfigValue } from './BotRuntimeForm'
import BotSandboxForm, { BotSandboxValue } from './BotSandboxForm'
import { DEFAULT_SANDBOX_PRESET } from '../../constants/sandboxPresets'
import { TypesSandboxRuntime } from '../../api/api'
import LoadingSpinner from '../widgets/LoadingSpinner'
import useSnackbar from '../../hooks/useSnackbar'
import { extractErrorMessage } from '../../hooks/useErrorCallback'
import { useOrgCodeAgentHarnesses } from '../../services/codeAgentHarnessesService'
import {
  SettingsSpecDTO,
  useDeleteHelixOrgSetting,
  useHelixOrgBase,
  useHelixOrgSettings,
  useSetHelixOrgSetting,
} from '../../services/helixOrgService'

export const SANDBOX_RUNTIME_KEY = 'worker.sandbox_runtime'
export const SANDBOX_VCPUS_KEY = 'worker.sandbox_vcpus'

const decodeIntValue = (v: string): number => {
  if (!v) return 0
  try {
    const parsed = JSON.parse(v)
    return typeof parsed === 'number' && Number.isFinite(parsed) ? parsed : 0
  } catch {
    return 0
  }
}

// DefaultSandboxPanel edits the org-wide sandbox defaults every agent
// without its own setting inherits. Unset falls through to the Helix
// default (full desktop, standard preset), matching spec tasks.
export const DefaultSandboxPanel: FC<{ disabled?: boolean }> = ({ disabled = false }) => {
  const { data, isLoading } = useHelixOrgSettings()
  const setMut = useSetHelixOrgSetting()
  const deleteMut = useDeleteHelixOrgSetting()
  const snackbar = useSnackbar()

  const specByKey = useMemo(() => {
    const m = new Map<string, SettingsSpecDTO>()
    for (const s of data?.specs ?? []) m.set(s.key, s)
    return m
  }, [data])
  const stored: BotSandboxValue = {
    runtime: decodeStringValue(specByKey.get(SANDBOX_RUNTIME_KEY)?.value ?? ''),
    vcpus: decodeIntValue(specByKey.get(SANDBOX_VCPUS_KEY)?.value ?? ''),
  }
  const [value, setValue] = useState<BotSandboxValue>(stored)
  const [dirty, setDirty] = useState(false)

  useEffect(() => {
    if (!data) return
    setValue(stored)
    setDirty(false)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const saving = setMut.isPending || deleteMut.isPending
  const handleSave = async () => {
    try {
      if (value.runtime) {
        await setMut.mutateAsync({ key: SANDBOX_RUNTIME_KEY, value: JSON.stringify(value.runtime) })
      } else if (specByKey.get(SANDBOX_RUNTIME_KEY)?.configured) {
        await deleteMut.mutateAsync(SANDBOX_RUNTIME_KEY)
      }
      if (value.vcpus) {
        await setMut.mutateAsync({ key: SANDBOX_VCPUS_KEY, value: JSON.stringify(value.vcpus) })
      } else if (specByKey.get(SANDBOX_VCPUS_KEY)?.configured) {
        await deleteMut.mutateAsync(SANDBOX_VCPUS_KEY)
      }
      setDirty(false)
      snackbar.success('Default sandbox saved')
    } catch (e: any) {
      snackbar.error(extractErrorMessage(e) || 'save failed')
    }
  }

  return (
    <Paper variant="outlined" sx={{ p: 3 }}>
      <Typography variant="subtitle1" sx={{ mb: 0.5 }}>Default sandbox</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Environment and size for agents that don't set their own. Applies on each agent's next start.
      </Typography>
      {isLoading ? <LoadingSpinner /> : (
        <>
          <BotSandboxForm
            value={value}
            onChange={(patch) => { setValue((current) => ({ ...current, ...patch })); setDirty(true) }}
            disabled={disabled || saving}
            inheritLabel="Helix default"
            effective={{
              runtime: TypesSandboxRuntime.SandboxRuntimeUbuntuDesktop,
              vcpus: DEFAULT_SANDBOX_PRESET.vcpus,
              memory_mb: DEFAULT_SANDBOX_PRESET.memory_mb,
            }}
          />
          <Button
            variant="contained"
            onClick={handleSave}
            disabled={disabled || !dirty || saving}
            sx={{ mt: 2 }}
          >
            {saving ? 'Saving...' : 'Save Default Sandbox'}
          </Button>
        </>
      )}
    </Paper>
  )
}

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
  const [saveStatus, setSaveStatus] = useState<'idle' | 'saving' | 'saved' | 'error'>('idle')

  useEffect(() => {
    if (saveStatus !== 'saved') return
    const timeout = window.setTimeout(() => setSaveStatus('idle'), 3000)
    return () => window.clearTimeout(timeout)
  }, [saveStatus])

  // Re-seed local state when the loaded data lands or refreshes.
  useEffect(() => {
    if (!data) return
    setValue(initial)
    setDirty(false)
    setSaveStatus('idle')
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const handlePatch = (patch: Partial<AgentConfigValue>) => {
    if (disabled) return
    setValue((current) => ({ ...current, ...patch }))
    setDirty(true)
  }

  const handleSave = async () => {
    setSaveStatus('saving')
    try {
      await setMut.mutateAsync({ key: 'agent.default', value: JSON.stringify({
        code_agent_runtime: value.runtime,
        code_agent_credential_type: value.credentials,
        provider: value.provider,
        model: value.model,
        reasoning_effort: value.reasoning_effort || 'none',
      }) })
      setDirty(false)
      snackbar.success('Default runtime saved')
      setSaveStatus('saved')
    } catch (e: any) {
      const message = extractErrorMessage(e) || 'save failed'
      snackbar.error(message)
      setSaveStatus('error')
    }
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
              disabled={disabled || setMut.isPending || saveStatus === 'saving'}
              subscriptionAvailability={subscriptionAvailability}
            />
            <Button
              variant="contained"
              onClick={handleSave}
              disabled={disabled || !dirty || setMut.isPending || saveStatus === 'saving'}
              sx={{ mt: 2 }}
            >
              {setMut.isPending || saveStatus === 'saving' ? 'Saving...' : 'Save Default Runtime'}
            </Button>
            {saveStatus !== 'idle' && (
              <Typography
                role="status"
                variant="caption"
                color={saveStatus === 'error' ? 'error.main' : saveStatus === 'saved' ? 'success.main' : 'text.secondary'}
                sx={{ display: 'block', mt: 2 }}
              >
                {saveStatus === 'saving' ? 'Saving...' : saveStatus === 'saved' ? 'Saved' : 'Save failed'}
              </Typography>
            )}
          </>}
    </Paper>
  )
}

export default DefaultAgentConfigPanel
