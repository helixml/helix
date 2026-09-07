// OrgAgentDetailsPane is the "Details" view of the org agent workspace — the
// bot's counterpart of the spec task Details view. It shows the sandbox the
// agent runs in (status, environment, size, host row), the session and
// project it is attached to, and the shareable preview URLs. Editing lives
// on the agent settings page; this pane links there.

import { FC } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Link from '@mui/material/Link'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { Settings2 } from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import { BotDTO } from '../../services/helixOrgService'
import SandboxStatusIndicator, { SandboxIndicatorState } from '../tasks/SandboxStatusIndicator'
import SharePreviewSection from '../tasks/SharePreviewSection'
import { sandboxRuntimeLabel, sandboxSizeLabel } from './BotSandboxForm'

interface OrgAgentDetailsPaneProps {
  bot: BotDTO
  sessionId: string
  organizationId: string
  indicatorState: SandboxIndicatorState
}

const Row: FC<{ label: string; children: React.ReactNode }> = ({ label, children }) => (
  <Stack direction="row" spacing={2} alignItems="baseline" sx={{ minWidth: 0 }}>
    <Typography variant="body2" color="text.secondary" sx={{ width: 120, flexShrink: 0 }}>
      {label}
    </Typography>
    <Box sx={{ minWidth: 0, flex: 1 }}>{children}</Box>
  </Stack>
)

const OrgAgentDetailsPane: FC<OrgAgentDetailsPaneProps> = ({ bot, sessionId, organizationId, indicatorState }) => {
  const router = useRouter()
  const agentID = bot.agent_id ?? bot.agent_app_id
  const runtime = sandboxRuntimeLabel(bot.effective_sandbox_runtime) || 'Full Desktop'
  const size = sandboxSizeLabel(
    bot.effective_sandbox_resource_overrides?.vcpus,
    bot.effective_sandbox_resource_overrides?.memory_mb,
  )
  const ownRuntime = !!bot.sandbox_runtime
  const ownSize = !!bot.sandbox_resource_overrides?.vcpus
  const statusLabel = bot.sandbox_status
    ? bot.sandbox_status.charAt(0).toUpperCase() + bot.sandbox_status.slice(1)
    : indicatorState === 'running' ? 'Running' : 'Stopped'

  const openSettings = () => {
    if (!agentID) return
    router.navigate('org_agent', { org_id: organizationId, app_id: agentID })
  }
  const openSandbox = (e: React.MouseEvent) => {
    e.preventDefault()
    if (!bot.sandbox_id) return
    router.navigate('org_sandbox_detail', { org_id: organizationId, sandbox_id: bot.sandbox_id })
  }
  const openProject = (e: React.MouseEvent) => {
    e.preventDefault()
    if (!bot.project_id) return
    router.navigate('org_project-specs', { org_id: organizationId, id: bot.project_id })
  }

  return (
    <Stack spacing={2}>
      <Paper variant="outlined" sx={{ p: 2 }}>
        <Stack direction="row" alignItems="center" justifyContent="space-between" sx={{ mb: 1.5 }}>
          <Stack direction="row" alignItems="center" spacing={0.5}>
            <SandboxStatusIndicator state={indicatorState} />
            <Typography variant="subtitle1">Sandbox</Typography>
          </Stack>
          <Button
            size="small"
            variant="outlined"
            startIcon={<Settings2 size={16} />}
            onClick={openSettings}
            disabled={!agentID}
          >
            Agent settings
          </Button>
        </Stack>
        <Stack spacing={1}>
          <Row label="Status">
            <Typography variant="body2">{statusLabel}</Typography>
            {bot.sandbox_status_message && (
              <Typography variant="caption" color="error.main" sx={{ display: 'block' }}>
                {bot.sandbox_status_message}
              </Typography>
            )}
          </Row>
          <Row label="Environment">
            <Typography variant="body2">
              {runtime}
              {!ownRuntime && (
                <Typography component="span" variant="caption" color="text.secondary"> · org default</Typography>
              )}
            </Typography>
          </Row>
          <Row label="Compute">
            <Typography variant="body2">
              {size || 'Standard'}
              {!ownSize && (
                <Typography component="span" variant="caption" color="text.secondary"> · org default</Typography>
              )}
            </Typography>
          </Row>
          {bot.restart_required && (
            <Typography variant="caption" color="warning.main">
              The running sandbox predates the latest settings. Restart the agent to apply them.
            </Typography>
          )}
          {bot.sandbox_id && (
            <Row label="Sandbox">
              <Link href="#" onClick={openSandbox} variant="body2" sx={{ fontFamily: 'monospace' }} noWrap>
                {bot.sandbox_id}
              </Link>
            </Row>
          )}
          <Row label="Session">
            <Typography variant="body2" sx={{ fontFamily: 'monospace' }} noWrap>{sessionId}</Typography>
          </Row>
          {bot.project_id && (
            <Row label="Project">
              <Link href="#" onClick={openProject} variant="body2" sx={{ fontFamily: 'monospace' }} noWrap>
                {bot.project_id}
              </Link>
            </Row>
          )}
        </Stack>
      </Paper>
      <SharePreviewSection sessionId={sessionId} />
    </Stack>
  )
}

export default OrgAgentDetailsPane
