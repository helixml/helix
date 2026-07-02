// WorkerRuntimePanel is the connected "Default Bot Runtime" config on the org
// General settings page. It seeds BotRuntimeForm from the org's worker.* config
// registry keys and auto-saves each change back. Org context is resolved from
// router.params.org_id by the underlying hooks.

import { FC, useEffect, useMemo, useState } from 'react'
import Paper from '@mui/material/Paper'

import BotRuntimeForm, { BotRuntimeValue } from './BotRuntimeForm'
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

const LABELS: Record<string, string> = {
  runtime: 'Runtime',
  credentials: 'Credentials',
  provider: 'Provider',
  model: 'Model',
}

const WorkerRuntimePanel: FC = () => {
  const { data, isLoading } = useHelixOrgSettings()
  const setMut = useSetHelixOrgSetting()
  const snackbar = useSnackbar()

  const specByKey = useMemo(() => {
    const m = new Map<string, SettingsSpecDTO>()
    for (const s of data?.specs ?? []) m.set(s.key, s)
    return m
  }, [data])

  const initial: BotRuntimeValue = {
    runtime: decodeStringValue(specByKey.get('worker.runtime')?.value ?? '') || 'claude_code',
    credentials: decodeStringValue(specByKey.get('worker.credentials')?.value ?? '') || 'subscription',
    provider: decodeStringValue(specByKey.get('worker.provider')?.value ?? ''),
    model: decodeStringValue(specByKey.get('worker.model')?.value ?? ''),
  }

  const [value, setValue] = useState<BotRuntimeValue>(initial)

  // Re-seed local state when the loaded data lands or refreshes.
  useEffect(() => {
    if (!data) return
    setValue(initial)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data])

  const handlePatch = (patch: Partial<BotRuntimeValue>) => {
    setValue((v) => ({ ...v, ...patch }))
    for (const [k, val] of Object.entries(patch)) {
      setMut
        .mutateAsync({ key: `worker.${k}`, value: JSON.stringify(val) })
        .then(() => snackbar.success(`${LABELS[k] ?? k} saved`))
        .catch((e: any) => snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed'))
    }
  }

  return (
    <Paper variant="outlined" sx={{ p: 3 }}>
      {isLoading ? <LoadingSpinner /> : <BotRuntimeForm value={value} onChange={handlePatch} />}
    </Paper>
  )
}

export default WorkerRuntimePanel
