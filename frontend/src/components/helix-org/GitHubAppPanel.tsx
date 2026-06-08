// GitHubAppConnect is the single create / install / manage surface for the
// org's Helix GitHub App, shared by the Settings page (mode="panel") and the
// New Stream dialog (mode="gate"). It owns the install-status query and the
// create/install/manage popup flows (useGitHubAppActions) so both call sites
// stay identical.
//
// - mode="panel": wrapped in a Paper with a header; when installed shows a
//   "connected" status + Install-on-more / Manage links.
// - mode="gate": inline (action.hover box); renders only while NOT installed
//   (the dialog shows its repo picker once installed).

import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import { useGitHubAppInstallation } from '../../services/helixOrgService'
import { useGitHubAppActions } from './useGitHubAppActions'

export const GitHubAppConnect: FC<{ mode: 'panel' | 'gate'; onChange?: () => void }> = ({ mode, onChange }) => {
  const q = useGitHubAppInstallation({ pollWhileNotInstalled: true })
  const status = q.data
  const appExists = status?.app_exists === true
  const installed = status?.installed === true
  const installUrl = status?.install_url ?? ''
  const manageUrl = status?.manage_url ?? ''
  const { createApp, installApp, openManage, creating } = useGitHubAppActions(() => { q.refetch(); onChange?.() })
  const [githubOrg, setGithubOrg] = useState('')

  // In gate mode the dialog renders the repo picker once installed, so the
  // gate only appears while there's still something to set up.
  if (mode === 'gate' && (q.isLoading || installed)) return null

  const manageButton = manageUrl ? (
    <Button size="small" variant={mode === 'gate' ? 'text' : 'outlined'} onClick={() => openManage(manageUrl)} data-testid="github-app-manage-button">
      Manage app on GitHub →
    </Button>
  ) : null

  let body: React.ReactNode
  if (q.isLoading) {
    body = <CircularProgress size={20} />
  } else if (!appExists) {
    body = (
      <Stack spacing={1.5}>
        <Typography variant="body2">
          <strong>Create the Helix GitHub App for your org.</strong> Helix creates an app owned by your GitHub organization (one click — GitHub pre-fills the permissions). Afterwards Helix acts as the <code>helix</code> bot, not your personal account.
        </Typography>
        <Stack direction="row" spacing={1} alignItems="center">
          <TextField
            size="small"
            label="GitHub organization"
            placeholder="e.g. helixml"
            value={githubOrg}
            onChange={(e) => setGithubOrg(e.target.value)}
            sx={{ flex: 1, maxWidth: 360 }}
            inputProps={{ 'data-testid': 'github-app-create-org' }}
          />
          <Button
            variant="contained"
            size={mode === 'gate' ? 'small' : 'medium'}
            onClick={() => createApp(githubOrg)}
            disabled={!githubOrg.trim() || creating}
            data-testid="github-app-create-button"
          >
            {creating ? 'Starting…' : 'Create Helix app'}
          </Button>
        </Stack>
        <Typography variant="caption" color="text.secondary">
          You must be an owner of that GitHub org. You'll review the app on GitHub, click Create, then choose which repos to install it on.
        </Typography>
      </Stack>
    )
  } else if (!installed) {
    body = (
      <Stack spacing={1.5}>
        <Typography variant="body2">
          <strong>The Helix app is created but not installed on any repo yet.</strong> Install it on the repos you want Helix to work with.
        </Typography>
        <Stack direction="row" spacing={1} alignItems="center">
          <Button variant="contained" size={mode === 'gate' ? 'small' : 'medium'} onClick={() => installApp(installUrl)} disabled={!installUrl} data-testid="github-app-install-button">
            Install Helix
          </Button>
          {manageButton}
        </Stack>
      </Stack>
    )
  } else {
    body = (
      <Stack spacing={1.5}>
        <Typography variant="body2">
          <strong>✓ Helix App connected.</strong> Workers act as the bot; GitHub streams receive events via the app's webhook.
        </Typography>
        <Stack direction="row" spacing={1} alignItems="center">
          <Button size="small" variant="outlined" onClick={() => installApp(installUrl)} disabled={!installUrl}>
            Install on more repos
          </Button>
          {manageButton}
        </Stack>
        <Typography variant="caption" color="text.secondary">
          Manage = edit permissions, add/remove repositories, or delete the app — opens the app's developer settings on GitHub.
        </Typography>
      </Stack>
    )
  }

  if (mode === 'gate') {
    return (
      <Box sx={{ p: 1.5, borderRadius: 1, backgroundColor: 'action.hover' }} data-testid="github-app-gate">
        {body}
      </Box>
    )
  }
  return (
    <Paper variant="outlined" sx={{ p: 3 }}>
      <Typography variant="h6" sx={{ mb: 0.5 }}>GitHub App</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Helix acts on GitHub as a bot via an org-owned GitHub App: Workers clone / push / open PRs as the bot, and GitHub stream events are delivered through the app's webhook.
      </Typography>
      {body}
    </Paper>
  )
}

// GitHubAppPanel is the Settings-page form of the connector.
export const GitHubAppPanel: FC = () => <GitHubAppConnect mode="panel" />

export default GitHubAppPanel
