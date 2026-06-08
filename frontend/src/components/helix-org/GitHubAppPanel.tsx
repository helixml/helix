// GitHubAppConnect is the single create / install / add-repos / manage surface
// for the org's Helix GitHub App, shared by the Settings page (mode="panel")
// and the New Stream dialog (mode="gate"). It owns the install-status query and
// the popup flows (useGitHubAppActions) so both call sites stay identical.
//
// The flow it guides:
//   1. Create the Helix app (org-owned).
//   2. Install it on a GitHub org (choosing repos during install).
//   3. Add more repositories / install on another org anytime.

import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import Divider from '@mui/material/Divider'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'

import { useGitHubAppInstallation } from '../../services/helixOrgService'
import { useGitHubAppActions } from './useGitHubAppActions'

const Step: FC<{ n: number; label: string; done: boolean; active: boolean }> = ({ n, label, done, active }) => (
  <Stack direction="row" spacing={1.25} alignItems="center">
    <Box
      sx={{
        width: 22, height: 22, borderRadius: '50%', flex: '0 0 auto',
        display: 'flex', alignItems: 'center', justifyContent: 'center',
        fontSize: 12, fontWeight: 700,
        bgcolor: done ? 'success.main' : active ? 'primary.main' : 'action.disabledBackground',
        color: done || active ? 'common.white' : 'text.secondary',
      }}
    >
      {done ? '✓' : n}
    </Box>
    <Typography variant="body2" color={done || active ? 'text.primary' : 'text.secondary'}>
      {label}
    </Typography>
  </Stack>
)

export const GitHubAppConnect: FC<{ mode: 'panel' | 'gate'; onChange?: () => void }> = ({ mode, onChange }) => {
  const q = useGitHubAppInstallation({ pollWhileNotInstalled: true })
  const status = q.data
  const appExists = status?.app_exists === true
  const installed = status?.installed === true
  const installUrl = status?.install_url ?? ''
  const manageUrl = status?.manage_url ?? ''
  const { createApp, installApp, openManage, creating } = useGitHubAppActions(() => { q.refetch(); onChange?.() })
  const [githubOrg, setGithubOrg] = useState('')

  const small = mode === 'gate'
  const manageBtn = manageUrl ? (
    <Button size="small" variant="text" onClick={() => openManage(manageUrl)} data-testid="github-app-manage-button">
      Manage app on GitHub →
    </Button>
  ) : null
  // The install URL doubles as "add repositories to an existing install" and
  // "install on another org" — GitHub's installation screen handles both.
  const addReposBtn = (
    <Button size="small" variant="outlined" onClick={() => installApp(installUrl)} disabled={!installUrl} data-testid="github-app-add-repos-button">
      Add repositories / another org
    </Button>
  )

  let statusLine: React.ReactNode = null
  let action: React.ReactNode = null
  if (q.isLoading) {
    action = <CircularProgress size={20} />
  } else if (!appExists) {
    statusLine = (
      <Typography variant="body2">
        <strong>Create the Helix GitHub App for your org.</strong> Owned by your GitHub organization (one click — GitHub pre-fills the permissions). Afterwards Helix acts as the <code>helix</code> bot.
      </Typography>
    )
    action = (
      <Stack spacing={1}>
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
            size={small ? 'small' : 'medium'}
            onClick={() => createApp(githubOrg)}
            disabled={!githubOrg.trim() || creating}
            data-testid="github-app-create-button"
          >
            {creating ? 'Starting…' : 'Create Helix app'}
          </Button>
        </Stack>
        <Typography variant="caption" color="text.secondary">You must be an owner of that GitHub org.</Typography>
      </Stack>
    )
  } else if (!installed) {
    statusLine = (
      <Typography variant="body2">
        <strong>App created — now install it on a GitHub org.</strong> On GitHub's install screen, choose <em>All repositories</em> (or pick the ones you want); that's exactly what Helix can access.
      </Typography>
    )
    action = (
      <Stack direction="row" spacing={1} alignItems="center">
        <Button variant="contained" size={small ? 'small' : 'medium'} onClick={() => installApp(installUrl)} disabled={!installUrl} data-testid="github-app-install-button">
          Install Helix
        </Button>
        {manageBtn}
      </Stack>
    )
  } else {
    statusLine = (
      <Typography variant="body2">
        <strong>✓ Helix App installed.</strong> It can access the repositories you granted during install. Add more repos — or install it on another GitHub org — anytime.
      </Typography>
    )
    action = (
      <Stack direction="row" spacing={1} alignItems="center">
        {addReposBtn}
        {manageBtn}
      </Stack>
    )
  }

  const steps = !q.isLoading && (
    <Stack spacing={0.75}>
      <Step n={1} label="Create the Helix app" done={appExists} active={!appExists} />
      <Step n={2} label="Install it on a GitHub org" done={installed} active={appExists && !installed} />
      <Step n={3} label="Choose which repositories Helix can access (during install — add more anytime)" done={installed} active={false} />
    </Stack>
  )

  if (mode === 'gate') {
    if (q.isLoading) return null
    return (
      <Box sx={{ p: 1.5, borderRadius: 1, backgroundColor: 'action.hover' }} data-testid="github-app-gate">
        <Stack spacing={1}>
          {statusLine}
          {action}
        </Stack>
      </Box>
    )
  }
  return (
    <Paper variant="outlined" sx={{ p: 3 }}>
      <Typography variant="h6" sx={{ mb: 0.5 }}>GitHub App</Typography>
      <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
        Helix acts on GitHub as a bot via an org-owned GitHub App: Workers clone / push / open PRs as the bot, and GitHub stream events are delivered through the app's webhook.
      </Typography>
      <Stack spacing={2}>
        {steps}
        <Divider />
        <Stack spacing={1.5}>
          {statusLine}
          {action}
        </Stack>
      </Stack>
    </Paper>
  )
}

// GitHubAppPanel is the Settings-page form of the connector.
export const GitHubAppPanel: FC = () => <GitHubAppConnect mode="panel" />

export default GitHubAppPanel
