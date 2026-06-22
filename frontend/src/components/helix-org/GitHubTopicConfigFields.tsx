// GitHub topic-config field building blocks. The Events and Branches
// editors are headless (the parent owns values + change handlers) and
// are composed alongside the shared GitHubRepoPicker by both the New
// Topic dialog and the per-topic Edit form. Repository selection lives
// in GitHubRepoPicker; event-type selection only appears at creation
// (afterwards events are managed on GitHub's own webhook UI).

import { FC } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import Box from '@mui/material/Box'
import Checkbox from '@mui/material/Checkbox'
import Chip from '@mui/material/Chip'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import {
  EventOption,
  GITHUB_EVENT_OPTIONS,
  eventValue,
  isValidGitHubEvent,
} from './githubTopicConstants'

// GitHubEventsField is the curated-but-free-text event whitelist editor.
export const GitHubEventsField: FC<{ events: string[]; onChange: (next: string[]) => void }> = ({
  events,
  onChange,
}) => {
  const badEvents = events.filter((e) => !isValidGitHubEvent(e))
  return (
    <Autocomplete<EventOption, true, false, true>
      multiple
      freeSolo
      forcePopupIcon
      openOnFocus
      disableCloseOnSelect
      disablePortal
      options={GITHUB_EVENT_OPTIONS}
      value={events}
      onChange={(_, next) => {
        const cleaned = next
          .map(eventValue)
          .map((s) => s.trim())
          .filter((s) => s.length > 0)
        onChange(Array.from(new Set(cleaned)))
      }}
      getOptionLabel={(o) => eventValue(o)}
      isOptionEqualToValue={(opt, val) => opt.value === eventValue(val)}
      filterSelectedOptions
      renderOption={(props, option, { selected }) => {
        const { key: _optKey, ...liProps } = props as React.HTMLAttributes<HTMLLIElement> & { key?: React.Key }
        return (
          <li {...liProps} key={option.value}>
            <Checkbox checked={selected} sx={{ mr: 1 }} />
            <Box>
              <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{option.value}</Typography>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', whiteSpace: 'normal' }}>
                {option.help}
              </Typography>
            </Box>
          </li>
        )
      }}
      renderTags={(value, getTagProps) =>
        value.map((option, index) => {
          const v = eventValue(option)
          const known = GITHUB_EVENT_OPTIONS.some((o) => o.value === v)
          const valid = isValidGitHubEvent(v)
          const { key: _tagKey, ...tagProps } = getTagProps({ index })
          return (
            <Chip
              {...tagProps}
              key={v}
              label={v}
              size="small"
              color={valid ? (known ? 'default' : 'info') : 'error'}
              variant={known ? 'filled' : 'outlined'}
              title={
                !valid
                  ? 'Invalid event name — must be lowercase letters, digits, and underscores (a-z, 0-9, _).'
                  : known
                  ? undefined
                  : 'Custom event name — accepted by the server, but the envelope mapping is best-effort.'
              }
              sx={{ fontFamily: 'monospace' }}
            />
          )
        })
      }
      renderInput={(params) => (
        <TextField
          {...params}
          label="Events"
          placeholder={events.length === 0 ? 'Pick events, or type your own and press Enter…' : ''}
          size="small"
          error={badEvents.length > 0}
          helperText={
            badEvents.length > 0
              ? `Invalid event name(s): ${badEvents.join(', ')} — must match /^[a-z][a-z0-9_]+$/`
              : 'Anything not listed is dropped at the transport. `*` = all events. Type a custom name and press Enter to add it.'
          }
        />
      )}
    />
  )
}

// GitHubBranchesField is the optional per-branch filter for push/create/delete.
export const GitHubBranchesField: FC<{ branches: string[]; onChange: (next: string[]) => void }> = ({
  branches,
  onChange,
}) => (
  <Autocomplete<string, true, false, true>
    multiple
    freeSolo
    openOnFocus
    disablePortal
    options={['*']}
    value={branches}
    onChange={(_, next) => {
      const cleaned = next.map((s) => s.trim()).filter((s) => s.length > 0)
      onChange(Array.from(new Set(cleaned)))
    }}
    renderTags={(value, getTagProps) =>
      value.map((b, index) => {
        const { key: _k, ...tagProps } = getTagProps({ index })
        return <Chip {...tagProps} key={b} label={b} size="small" sx={{ fontFamily: 'monospace' }} />
      })
    }
    renderInput={(params) => (
      <TextField
        {...params}
        label="Branches"
        placeholder={branches.length === 0 ? 'e.g. *, main, release/*' : ''}
        size="small"
        helperText="Applies to push / create / delete events. `*` = all branches; narrow with an exact name (main) or a prefix glob (release/*). Other event types (issues, pull_request, …) are unaffected."
      />
    )}
  />
)
