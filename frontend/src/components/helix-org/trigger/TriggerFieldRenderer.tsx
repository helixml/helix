import { FC } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import CopyButtonWithCheck from '../../session/CopyButtonWithCheck'
import CronScheduleFields from '../CronScheduleFields'
import GitHubRepoPicker from '../GitHubRepoPicker'
import { GitHubBranchesField, GitHubEventsField } from '../GitHubTopicConfigFields'
import GitLabEventsField from './fields/GitLabEventsField'
import GitLabRepoPicker from './fields/GitLabRepoPicker'
import SlackWorkspacePicker from './fields/SlackWorkspacePicker'
import { TransportDirection, TransportFieldType } from '../../../api/api'
import type { TriggerField } from '../../../services/triggerKindService'

const asText = (value: unknown): string =>
  Array.isArray(value) ? value.join(', ') : value === undefined || value === null ? '' : String(value)

const asList = (value: unknown): string[] => (Array.isArray(value) ? value.map(String) : [])

const ReadOnlyValue: FC<{ field: TriggerField; value: unknown }> = ({ field, value }) => {
  const text = asText(value)
  return (
    <Box>
      <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
        {field.label}
      </Typography>
      <Stack direction="row" spacing={0.5} alignItems="center" sx={{ minWidth: 0 }}>
        <Typography
          variant="body2"
          sx={{ fontFamily: 'monospace', overflowWrap: 'anywhere', color: text ? 'text.primary' : 'text.disabled' }}
        >
          {text || '—'}
        </Typography>
        {text && <CopyButtonWithCheck text={text} />}
      </Stack>
    </Box>
  )
}

export interface TriggerFieldRendererProps {
  field: TriggerField
  value: unknown
  onChange: (next: unknown) => void
  readOnly?: boolean
  // siblingValue/onSiblingChange carry the cron `message` field, which
  // CronScheduleFields owns together with `schedule` in one control.
  siblingValue?: unknown
  onSiblingChange?: (next: unknown) => void
}

const TriggerFieldRenderer: FC<TriggerFieldRendererProps> = ({
  field,
  value,
  onChange,
  readOnly,
  siblingValue,
  onSiblingChange,
}) => {
  if (readOnly || field.read_only) return <ReadOnlyValue field={field} value={value} />

  const help = field.direction === TransportDirection.Outbound ? undefined : field.help
  const outboundHelp = field.direction === TransportDirection.Outbound ? field.help : undefined

  const control = (() => {
    switch (field.type) {
      case TransportFieldType.FieldCron:
        return (
          <CronScheduleFields
            value={asText(value)}
            onChange={onChange}
            message={asText(siblingValue)}
            onMessageChange={onSiblingChange as ((next: string) => void) | undefined}
          />
        )
      case TransportFieldType.FieldGitHubRepo:
        return <GitHubRepoPicker value={asText(value)} onChange={onChange} />
      case TransportFieldType.FieldGitHubEvents:
        return <GitHubEventsField events={asList(value)} onChange={onChange} />
      case TransportFieldType.FieldGitLabRepo:
        return <GitLabRepoPicker value={asText(value)} onChange={onChange} label={field.label ?? ''} help={field.help} />
      case TransportFieldType.FieldGitLabEvents:
        return <GitLabEventsField events={asList(value)} onChange={onChange} label={field.label ?? ''} help={field.help} />
      case TransportFieldType.FieldSlackWorkspace:
        return <SlackWorkspacePicker value={asText(value)} onChange={onChange} label={field.label ?? ''} help={field.help} />
      case TransportFieldType.FieldStringList:
        return field.name === 'branches' ? (
          <GitHubBranchesField branches={asList(value)} onChange={onChange} />
        ) : (
          <Autocomplete
            multiple
            freeSolo
            options={[]}
            value={asList(value)}
            onChange={(_, next) => onChange(next.map(String))}
            renderInput={(params) => <TextField {...params} label={field.label} size="small" helperText={help} />}
          />
        )
      default:
        return (
          <TextField
            label={field.label}
            value={asText(value)}
            onChange={(event) => onChange(event.target.value)}
            placeholder={field.placeholder}
            required={field.required}
            helperText={help}
            size="small"
            fullWidth
            type={field.type === TransportFieldType.FieldURL ? 'url' : 'text'}
          />
        )
    }
  })()

  if (!outboundHelp) return <Box>{control}</Box>

  return (
    <Box>
      {control}
      <Stack direction="row" spacing={0.75} alignItems="flex-start" sx={{ mt: 0.5, ml: 1.75 }}>
        <Chip label="Outbound" size="small" color="warning" variant="outlined" sx={{ height: 18, fontSize: '0.65rem' }} />
        <Typography variant="caption" color="text.secondary">{outboundHelp}</Typography>
      </Stack>
    </Box>
  )
}

export default TriggerFieldRenderer
