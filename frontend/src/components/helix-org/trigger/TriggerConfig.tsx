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
  const { data: kinds = [] } = useTriggerKinds()
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
    setRawConfig(JSON.stringify(trigger?.config ?? {}, null, 2))
  }, [trigger?.id, trigger?.name, trigger?.description, trigger?.kind, trigger?.revision])

  useEffect(() => {
    if (desc) setDraft(initialDraft(desc, savedConfig))
    // savedConfig is derived from trigger.revision, which is in the deps.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [desc?.kind, trigger?.id, trigger?.revision])

  const missing = missingRequired(desc, draft)

  useEffect(() => {
    if (!onChange) return
    let merged = draftToConfig(desc, draft, savedConfig)
    if (rawConfig.trim() && rawConfig.trim() !== '{}') {
      try {
        const parsed = JSON.parse(rawConfig)
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
          const modelled = new Set((desc?.fields ?? []).map((field) => field.name))
          for (const [key, value] of Object.entries(parsed)) {
            if (!modelled.has(key)) merged[key] = value
          }
        }
      } catch {
        // Surfaced through rawError below; the modelled fields still submit.
      }
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

      {ordered.length === 0 ? (
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
              helperText={rawError || 'Edit keys the form above does not cover. Changes here are merged with the fields above.'}
              sx={{ '& textarea': { fontFamily: 'monospace', fontSize: '0.8rem' } }}
            />
          </AccordionDetails>
        </Accordion>
      )}
    </Stack>
  )
}

export default TriggerConfig
