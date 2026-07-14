// DefaultAgentConfigPanel edits the org's default agent configuration. New orgs use
// the atomic agent.default object; legacy worker.* values remain readable.

import { FC, useEffect, useMemo, useState } from 'react'
import Paper from '@mui/material/Paper'

import AgentConfigForm, { AgentConfigValue } from './BotRuntimeForm'
import LoadingSpinner from '../widgets/LoadingSpinner'
import useSnackbar from '../../hooks/useSnackbar'
import {
  SettingsSpecDTO,
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
    return JSON.parse(v) as AgentConfigValue
  } catch {
    return undefined
  }
}

const DefaultAgentConfigPanel: FC = () => {
  const { data, isLoading } = useHelixOrgSettings()
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
  }

  const [value, setValue] = useState<AgentConfigValue>(initial)

  // Re-seed local state when the loaded data lands or refreshes.
  useEffect(() => {
    if (!data) return
    setValue(initial)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const handlePatch = (patch: Partial<AgentConfigValue>) => {
    const next = { ...value, ...patch }
    setValue(next)
    setMut
      .mutateAsync({ key: 'agent.default', value: JSON.stringify(next) })
      .then(() => snackbar.success('Default agent configuration saved'))
      .catch((e: any) => snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed'))
  }

  return (
    <Paper variant="outlined" sx={{ p: 3 }}>
      {isLoading ? <LoadingSpinner /> : <AgentConfigForm value={value} onChange={handlePatch} />}
    </Paper>
  )
}

export default DefaultAgentConfigPanel
