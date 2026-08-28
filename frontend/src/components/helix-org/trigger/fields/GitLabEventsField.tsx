import { FC } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

// The four values api/pkg/org/domain/transport/gitlab.go accepts. Any
// other value is rejected at create time.
const GITLAB_EVENT_OPTIONS = [
  { value: 'Merge Request Hook', help: 'Merge requests opened, updated, merged or closed.' },
  { value: 'Note Hook', help: 'Comments on commits, merge requests, issues and snippets.' },
  { value: 'Pipeline Hook', help: 'CI pipeline status changes.' },
  { value: 'Push Hook', help: 'Commits pushed to a branch.' },
]

const GitLabEventsField: FC<{
  events: string[]
  onChange: (next: string[]) => void
  label: string
  help?: string
}> = ({ events, onChange, label, help }) => (
  <Autocomplete
    multiple
    disableCloseOnSelect
    disablePortal
    options={GITLAB_EVENT_OPTIONS.map((option) => option.value)}
    value={events}
    onChange={(_, next) => onChange(Array.from(new Set(next)))}
    renderOption={(props, option, { selected }) => {
      const { key: _key, ...liProps } = props as React.HTMLAttributes<HTMLLIElement> & { key?: React.Key }
      const meta = GITLAB_EVENT_OPTIONS.find((item) => item.value === option)
      return (
        <li {...liProps} key={option}>
          <Checkbox checked={selected} sx={{ mr: 1 }} />
          <div>
            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{option}</Typography>
            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', whiteSpace: 'normal' }}>
              {meta?.help}
            </Typography>
          </div>
        </li>
      )
    }}
    renderTags={(value, getTagProps) =>
      value.map((option, index) => {
        const { key: _key, ...tagProps } = getTagProps({ index })
        return <Chip {...tagProps} key={option} label={option} size="small" sx={{ fontFamily: 'monospace' }} />
      })
    }
    renderInput={(params) => <TextField {...params} label={label} size="small" helperText={help} />}
  />
)

export default GitLabEventsField
