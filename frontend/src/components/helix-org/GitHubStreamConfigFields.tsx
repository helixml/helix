// GitHubStreamConfigFields is the Repository + Events form block
// shared by the New Stream dialog and the per-stream Edit dialog.
// It's intentionally headless: the parent owns the values + change
// handlers so the dialog around it stays in control of submit
// validation, snackbar wiring, etc.

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
  GITHUB_EVENT_PATTERN,
  GITHUB_REPO_PATTERN,
  eventValue,
} from './githubStreamConstants'

export interface GitHubStreamConfigFieldsProps {
  repo: string
  events: string[]
  branches?: string[]
  onRepoChange: (next: string) => void
  onEventsChange: (next: string[]) => void
  onBranchesChange?: (next: string[]) => void
}

export const GitHubStreamConfigFields: FC<GitHubStreamConfigFieldsProps> = ({
  repo,
  events,
  branches = [],
  onRepoChange,
  onEventsChange,
  onBranchesChange,
}) => {
  const repoError = repo.trim() === ''
    ? ''
    : (GITHUB_REPO_PATTERN.test(repo.trim()) ? '' : 'Must be `owner/name` (one slash, both halves non-empty).')
  const badEvents = events.filter((e) => !GITHUB_EVENT_PATTERN.test(e))

  return (
    <>
      {/* Scoping fields — backed by the Go validator's
          GitHubConfig{Repo, Events}. We surface the validator's
          rules inline so the user catches the mistake before
          submit. */}
      <TextField
        label="Repository"
        placeholder="helixml/helix"
        value={repo}
        onChange={(e) => onRepoChange(e.target.value)}
        error={!!repoError}
        helperText={repoError || 'Exactly one repo per stream — GitHub `owner/name` (e.g. helixml/helix). Matched case-insensitively against repository.full_name in the payload.'}
        fullWidth
        size="small"
      />
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
          onEventsChange(Array.from(new Set(cleaned)))
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
            const valid = GITHUB_EVENT_PATTERN.test(v)
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
                : "Anything not listed is dropped at the transport, so subscribed Workers don't activate for events they'd ignore. Type a custom name and press Enter to add it (e.g. push, release, workflow_run)."
            }
          />
        )}
      />
      {onBranchesChange && (
        <Autocomplete<string, true, false, true>
          multiple
          freeSolo
          openOnFocus
          disablePortal
          options={[]}
          value={branches}
          onChange={(_, next) => {
            const cleaned = next.map((s) => s.trim()).filter((s) => s.length > 0)
            onBranchesChange(Array.from(new Set(cleaned)))
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
              label="Branches (optional)"
              placeholder={branches.length === 0 ? 'e.g. main, release/* — leave empty for all branches' : ''}
              size="small"
              helperText="Narrows push / create / delete events to these branches — exact name (main) or prefix glob (release/*). Empty = all branches. Other event types (issues, pull_request, …) are unaffected."
            />
          )}
        />
      )}
    </>
  )
}

export default GitHubStreamConfigFields
