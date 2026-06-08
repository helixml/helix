// GitHubRepoPicker is the shared `owner/name` repository selector for
// helix-org. It lists the repos the Helix GitHub App can see (via
// useListGitHubRepos) in an Autocomplete, falls back to free-text entry
// for repos that aren't listed yet, and exposes a refresh button so the
// operator can re-pull after granting the App access to a new org.
//
// It is intentionally the single source of truth for "pick a GitHub
// repo" across helix-org — the New Stream dialog and the per-stream
// Edit form both render it, so the picker behaves identically wherever a
// repository is configured. The parent owns the value + onChange; this
// component owns the fetching, loading state, and validation hint.

import { FC } from 'react'
import Autocomplete from '@mui/material/Autocomplete'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import RefreshIcon from '@mui/icons-material/Refresh'

import { useListGitHubRepos } from '../../services/helixOrgService'
import { GITHUB_REPO_PATTERN } from './githubStreamConstants'

export interface GitHubRepoPickerProps {
  value: string
  onChange: (next: string) => void
  // enabled gates the underlying repo fetch — pass the dialog's `open`
  // so closed dialogs don't poll GitHub. Defaults to true for always-on
  // surfaces (e.g. the inline stream-edit form).
  enabled?: boolean
  label?: string
  // helperOverride replaces the default count/hint helper text when set.
  helperOverride?: string
}

const GitHubRepoPicker: FC<GitHubRepoPickerProps> = ({
  value,
  onChange,
  enabled = true,
  label = 'Repository',
  helperOverride,
}) => {
  const reposQuery = useListGitHubRepos({ enabled })
  const options = (reposQuery.data?.repos ?? []).map((r) => r.full_name)
  const loading = reposQuery.isLoading || reposQuery.isRefetching

  const trimmed = value.trim()
  const repoError = trimmed === ''
    ? ''
    : (GITHUB_REPO_PATTERN.test(trimmed) ? '' : 'Must be `owner/name` (one slash, both halves non-empty).')

  const helper = repoError
    ? repoError
    : (helperOverride ?? (
        options.length >= 1000
          ? `Showing the ${options.length} most-recently-pushed repos. Type the full owner/name if the one you want isn't listed.`
          : `Pick a repo, or type owner/name directly. Helix registers the webhook on GitHub for you — no manual setup. ${options.length} repos loaded.`
      ))

  return (
    <Stack direction="row" spacing={1} alignItems="flex-start">
      <Autocomplete
        freeSolo
        autoSelect
        selectOnFocus
        fullWidth
        options={options}
        value={value || null}
        onChange={(_, v) => onChange((v ?? '').trim())}
        onInputChange={(_, v, reason) => {
          // Sync typed text into the value so callers see what the
          // operator typed even if they never picked a dropdown row.
          if (reason === 'input') onChange(v)
        }}
        loading={loading}
        disablePortal
        noOptionsText={
          reposQuery.isError
            ? 'Could not load repos — connect a GitHub account on Connected Services first.'
            : loading
              ? 'Loading…'
              : 'No matches — type the full owner/name and press Enter.'
        }
        renderInput={(params) => (
          <TextField
            {...params}
            label={label}
            placeholder="search or type owner/name…"
            size="small"
            error={!!repoError}
            InputProps={{
              ...params.InputProps,
              endAdornment: (
                <>
                  {loading && <CircularProgress size={16} sx={{ mr: 1 }} />}
                  {params.InputProps.endAdornment}
                </>
              ),
            }}
            helperText={helper}
          />
        )}
      />
      <IconButton
        size="small"
        onClick={() => reposQuery.refetch()}
        disabled={reposQuery.isFetching}
        title="Re-fetch repos from GitHub (use this after granting the Helix App access to a new org)"
        sx={{ mt: 0.5 }}
      >
        <RefreshIcon fontSize="small" />
      </IconButton>
    </Stack>
  )
}

export default GitHubRepoPicker
