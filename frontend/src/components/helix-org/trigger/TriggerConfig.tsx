import { FC, useEffect, useMemo, useState } from 'react'
import Accordion from '@mui/material/Accordion'
import AccordionDetails from '@mui/material/AccordionDetails'
import AccordionSummary from '@mui/material/AccordionSummary'
import Alert from '@mui/material/Alert'
import Chip from '@mui/material/Chip'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import MenuItem from '@mui/material/MenuItem'
import Select from '@mui/material/Select'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import { ChevronDown } from 'lucide-react'

import { TransportFieldType } from '../../../api/api'
import { useTriggerKinds } from '../../../services/triggerKindService'
import type { TriggerKindDescriptor } from '../../../services/triggerKindService'
import type { TriggerDTO } from '../../../services/triggerService'
import TriggerActivationCard from './TriggerActivationCard'
import TriggerFieldRenderer from './TriggerFieldRenderer'
import TriggerSecretsNote from './TriggerSecretsNote'
import { draftToConfig, initialDraft, missingRequired } from './triggerConfigModel'

export interface TriggerConfigValue {
  name: string
  description: string
  kind: string
  config: Record<string, unknown>
}

interface Props {
  trigger?: TriggerDTO
  density?: 'compact' | 'full'
  mode: 'read' | 'edit' | 'create'
  orgID?: string
  onChange?: (value: TriggerConfigValue, valid: boolean) => void
}

const TriggerConfig: FC<Props> = ({ trigger, density = 'full', mode, orgID, onChange }) => {
  const { data: kinds = [], isLoading: kindsLoading } = useTriggerKinds()
  const [name, setName] = useState(trigger?.name ?? '')
  const [description, setDescription] = useState(trigger?.description ?? '')
  const [kind, setKind] = useState(trigger?.kind ?? 'local')
  const [draft, setDraft] = useState<Record<string, unknown>>({})
  const [rawConfig, setRawConfig] = useState('{}')
  const [rawError, setRawError] = useState('')

  const desc: TriggerKindDescriptor | undefined = useMemo(
    () => kinds.find((entry) => entry.kind === kind),
    [kinds, kind],
  )
  const savedConfig = (trigger?.config ?? {}) as Record<string, unknown>

  useEffect(() => {
    setName(trigger?.name ?? '')
    setDescription(trigger?.description ?? '')
    setKind(trigger?.kind ?? 'local')
  }, [trigger?.id, trigger?.name, trigger?.description, trigger?.kind, trigger?.revision])

  // rawConfig is seeded during render for the same reason as draft: it is
  // authoritative for unmodelled keys, so a copy that arrives one commit
  // late would strip them on the first emit before restoring them. Unlike
  // draft this does not wait for the descriptor — the raw blob is the
  // Trigger's own config, known immediately.
  const configKey = `${trigger?.id ?? ''}|${trigger?.revision ?? ''}`
  const [seededConfigKey, setSeededConfigKey] = useState<string | undefined>(undefined)
  if (seededConfigKey !== configKey) {
    setSeededConfigKey(configKey)
    setRawConfig(JSON.stringify(trigger?.config ?? {}, null, 2))
  }

  // Seed the draft DURING RENDER, not in an effect. Field components such
  // as CronScheduleFields read their prop only in useState initialisers, so
  // a draft populated one commit later leaves them latched on their own
  // defaults — editing any single control then silently rewrites the rest
  // of the schedule. Setting state during render makes React re-render
  // before committing children, so they never mount with a stale value.
  const draftKey = `${desc?.kind ?? ''}|${trigger?.id ?? ''}|${trigger?.revision ?? ''}`
  const [seededKey, setSeededKey] = useState<string | undefined>(undefined)
  if (desc && seededKey !== draftKey) {
    setSeededKey(draftKey)
    setDraft(initialDraft(desc, savedConfig))
  }

  const missing = missingRequired(desc, draft)

  useEffect(() => {
    if (!onChange) return
    const merged = draftToConfig(desc, draft, savedConfig)
    // The raw JSON box is AUTHORITATIVE for keys the form does not model, so
    // deleting a key there actually removes it. Without dropping the
    // carried-over keys first, draftToConfig's {...existing} base would
    // silently resurrect anything the user deleted. Modelled keys are never
    // touched here — the fields above own those, read-only ones included.
    try {
      const parsed = JSON.parse(rawConfig || '{}')
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        const modelled = new Set((desc?.fields ?? []).map((field) => field.name))
        for (const key of Object.keys(merged)) {
          if (!modelled.has(key)) delete merged[key]
        }
        for (const [key, value] of Object.entries(parsed)) {
          if (!modelled.has(key)) merged[key] = value
        }
      }
    } catch {
      // Surfaced through rawError below; the modelled fields still submit.
    }
    onChange({ name: name.trim(), description: description.trim(), kind, config: merged }, !!name.trim() && missing.length === 0)
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [name, description, kind, JSON.stringify(draft), rawConfig, desc?.kind])

  const setField = (fieldName: string, value: unknown) =>
    setDraft((prev) => ({ ...prev, [fieldName]: value }))

  const editable = mode !== 'read'
  const fields = desc?.fields ?? []
  const ordered = [...fields.filter((f) => !f.read_only), ...fields.filter((f) => f.read_only)]
  const missingLabels = missing.map((n) => fields.find((f) => f.name === n)?.label ?? n)

  return (
    <Stack spacing={2}>
      {mode === 'create' && (
        <FormControl fullWidth size="small">
          <InputLabel id="trigger-kind-label">What starts this Trigger?</InputLabel>
          <Select
            labelId="trigger-kind-label"
            label="What starts this Trigger?"
            value={kind}
            onChange={(event) => { setKind(event.target.value as string); setRawConfig('{}') }}
          >
            {kinds.filter((entry) => !entry.system_managed).map((entry) => (
              <MenuItem key={entry.kind} value={entry.kind}>{entry.label}</MenuItem>
            ))}
          </Select>
        </FormControl>
      )}

      {desc?.summary && <Typography variant="caption" color="text.secondary">{desc.summary}</Typography>}

      {mode === 'edit' && (
        <Stack direction="row" spacing={1} alignItems="center">
          <Chip size="small" label={desc?.label ?? kind} />
          <Typography variant="caption" color="text.secondary">
            A Trigger's source cannot be changed after it is created. Create a new Trigger to use a different source.
          </Typography>
        </Stack>
      )}

      {editable && (
        <>
          <TextField label="Name" value={name} onChange={(e) => setName(e.target.value)} required size="small" autoFocus={mode === 'create'} />
          <TextField label="Description (optional)" value={description} onChange={(e) => setDescription(e.target.value)} multiline minRows={2} size="small" />
        </>
      )}

      {!desc ? (
        <Typography variant="caption" color="text.secondary">
          {kindsLoading ? 'Loading settings…' : 'Unknown Trigger type; edit the raw JSON below.'}
        </Typography>
      ) : ordered.length === 0 ? (
        <Typography variant="caption" color="text.secondary">This Trigger type has no settings.</Typography>
      ) : (
        ordered
          .filter((field) => !(desc?.kind === 'cron' && field.name === 'message'))
          .map((field) => (
            <TriggerFieldRenderer
              key={field.name}
              field={field}
              value={draft[field.name ?? '']}
              onChange={(next) => setField(field.name ?? '', next)}
              readOnly={!editable}
              siblingValue={field.type === TransportFieldType.FieldCron ? draft.message : undefined}
              onSiblingChange={field.type === TransportFieldType.FieldCron ? (next) => setField('message', next) : undefined}
            />
          ))
      )}

      {editable && missingLabels.length > 0 && (
        <Alert severity="error">Required: {missingLabels.join(', ')}</Alert>
      )}

      <TriggerActivationCard activation={trigger?.activation} density={density} />

      {density === 'full' && <TriggerSecretsNote secrets={desc?.secrets} orgID={orgID} />}

      {editable && (
        <Accordion disableGutters elevation={0} sx={{ '&:before': { display: 'none' }, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
          <AccordionSummary expandIcon={<ChevronDown size={18} />}>
            <Typography variant="body2" color="text.secondary">Advanced (raw JSON)</Typography>
          </AccordionSummary>
          <AccordionDetails>
            <TextField
              value={rawConfig}
              onChange={(event) => {
                setRawConfig(event.target.value)
                try {
                  const parsed = JSON.parse(event.target.value || '{}')
                  setRawError(!parsed || Array.isArray(parsed) || typeof parsed !== 'object' ? 'Must be a JSON object.' : '')
                } catch {
                  setRawError('Must be a JSON object.')
                }
              }}
              multiline
              minRows={5}
              size="small"
              fullWidth
              error={!!rawError}
              helperText={rawError || 'Keys the form models are controlled by the fields above. Anything else here is saved as-is, and removing it here removes it.'}
              sx={{ '& textarea': { fontFamily: 'monospace', fontSize: '0.8rem' } }}
            />
          </AccordionDetails>
        </Accordion>
      )}
    </Stack>
  )
}

export default TriggerConfig
