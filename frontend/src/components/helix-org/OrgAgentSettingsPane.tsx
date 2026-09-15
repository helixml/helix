// OrgAgentSettingsPane is the "Settings" view of the Org Bot workspace — the
// bot's counterpart of the spec task Details view, and the same content the
// standalone agent page shows. What the agent runs in (environment, size,
// status), where it lives (sandbox, session, project), the agent's own
// settings (name, harness, sandbox, instructions, tools, triggers, project
// access), and the shareable preview URLs. Edits save in place; there is no
// separate page to go to.

import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { Box as BoxIcon, Cpu, FolderKanban, Monitor, SquareTerminal } from 'lucide-react'

import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import { BotDTO, useHelixOrgBot } from '../../services/helixOrgService'
import { TypesSandboxRuntime } from '../../api/api'
import OrgAgentSettings from '../app/OrgAgentSettings'
import CopyButton from '../common/CopyButton'
import { SandboxIndicatorState } from '../tasks/SandboxStatusIndicator'
import SharePreviewSection from '../tasks/SharePreviewSection'
import { PRESENCE_OFFLINE_COLOR, PRESENCE_ONLINE_COLOR } from '../widgets/PresenceDot'
import { sandboxRuntimeLabel, sandboxSizeLabel } from './BotSandboxForm'

interface OrgAgentSettingsPaneProps {
  bot: BotDTO
  sessionId: string
  organizationId: string
  indicatorState: SandboxIndicatorState
}

const STATUS_LABEL: Record<SandboxIndicatorState, string> = {
  running: 'Running',
  starting: 'Starting',
  stopped: 'Stopped',
}

// One fact of the sandbox: a label, a value, and an optional "org default"
// note. The stat strip is the same dense treatment cards use elsewhere.
const Stat: FC<{ icon: ReactNode; label: string; value: string; note?: string }> = ({ icon, label, value, note }) => {
  const lightTheme = useLightTheme()
  return (
    <Box sx={{ minWidth: 0 }}>
      <Stack direction="row" alignItems="center" spacing={0.5} sx={{ color: 'text.secondary', mb: 0.25 }}>
        {icon}
        <Typography variant="caption" sx={{ fontSize: '0.65rem', letterSpacing: '0.06em', textTransform: 'uppercase' }}>
          {label}
        </Typography>
      </Stack>
      <Typography variant="body2" sx={{ fontSize: '0.85rem', fontWeight: 600, lineHeight: 1.3 }} noWrap>
        {value}
      </Typography>
      {note && (
        <Typography variant="caption" sx={{ color: lightTheme.isLight ? 'rgba(113,113,122,0.9)' : 'rgba(163,163,163,0.7)' }}>
          {note}
        </Typography>
      )}
    </Box>
  )
}

// An identifier with its own copy control, optionally opening the thing it
// names. Kept monospace and single-line so ids never wrap.
const IdRow: FC<{ label: string; value: string; onOpen?: () => void; openLabel?: string }> = ({ label, value, onOpen, openLabel }) => (
  <Stack direction="row" alignItems="center" spacing={1} sx={{ minWidth: 0, minHeight: 30 }}>
    <Typography variant="body2" color="text.secondary" sx={{ width: 72, flexShrink: 0 }}>
      {label}
    </Typography>
    {onOpen ? (
      <Typography
        component="button"
        type="button"
        variant="body2"
        onClick={onOpen}
        aria-label={openLabel}
        sx={{
          appearance: 'none',
          border: 0,
          p: 0,
          background: 'none',
          color: 'primary.main',
          cursor: 'pointer',
          fontFamily: 'monospace',
          fontSize: '0.8rem',
          minWidth: 0,
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          whiteSpace: 'nowrap',
          textAlign: 'left',
          '&:hover': { textDecoration: 'underline' },
        }}
      >
        {value}
      </Typography>
    ) : (
      <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.8rem', minWidth: 0 }} noWrap>
        {value}
      </Typography>
    )}
    <Box sx={{ flexShrink: 0, ml: 'auto' }}>
      <CopyButton content={value} size="small" />
    </Box>
  </Stack>
)

// A titled settings block. The embedded OrgAgentSettings sections drop their
// own page headings, so the pane supplies compact ones.
const Section: FC<{ title: string; description?: string; children: ReactNode; sx: object }> = ({ title, description, children, sx }) => (
  <Box sx={sx}>
    <Typography variant="subtitle1" sx={{ fontWeight: 600, mb: description ? 0.25 : 1.5 }}>
      {title}
    </Typography>
    {description && (
      <Typography variant="body2" color="text.secondary" sx={{ mb: 1.5 }}>
        {description}
      </Typography>
    )}
    {children}
  </Box>
)

const OrgAgentSettingsPane: FC<OrgAgentSettingsPaneProps> = ({ bot, sessionId, organizationId, indicatorState }) => {
  const router = useRouter()
  const lightTheme = useLightTheme()
  const { data: detail, refetch } = useHelixOrgBot(bot.id || undefined, { enabled: !!bot.id })
  const agentID = bot.legacy_app_id
  const headless = bot.effective_sandbox_runtime === TypesSandboxRuntime.SandboxRuntimeHeadlessUbuntu
  const runtime = sandboxRuntimeLabel(bot.effective_sandbox_runtime) || 'Full Desktop'
  const size = sandboxSizeLabel(
    bot.effective_sandbox_resource_overrides?.vcpus,
    bot.effective_sandbox_resource_overrides?.memory_mb,
  ) || 'Standard'
  const ownRuntime = !!bot.sandbox_runtime
  const ownSize = !!bot.sandbox_resource_overrides?.vcpus
  const statusLabel = bot.sandbox_status
    ? bot.sandbox_status.charAt(0).toUpperCase() + bot.sandbox_status.slice(1)
    : STATUS_LABEL[indicatorState]
  const statusColor = indicatorState === 'running'
    ? PRESENCE_ONLINE_COLOR
    : indicatorState === 'starting' ? '#fbbf24' : PRESENCE_OFFLINE_COLOR
  const panelSx = {
    border: '1px solid',
    borderColor: lightTheme.isLight ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)',
    borderRadius: 2,
    backgroundColor: 'background.paper',
    p: 2,
  } as const
  const onSaved = () => { void refetch() }

  return (
    <Stack spacing={2} sx={{ maxWidth: 760 }}>
      <Box sx={panelSx}>
        <Stack direction="row" alignItems="center" spacing={1.25} sx={{ mb: 2 }}>
          <Box
            component="span"
            sx={{ width: 9, height: 9, borderRadius: '50%', backgroundColor: statusColor, flexShrink: 0 }}
          />
          <Typography variant="subtitle1" sx={{ fontWeight: 600, flex: 1, minWidth: 0 }} noWrap>
            {bot.name || bot.id}
          </Typography>
          <Typography variant="body2" color="text.secondary">
            {statusLabel}
          </Typography>
        </Stack>

        {bot.sandbox_status_message && (
          <Typography variant="body2" color="error.main" sx={{ mb: 1.5 }}>
            {bot.sandbox_status_message}
          </Typography>
        )}
        {bot.restart_required && (
          <Typography variant="body2" color="warning.main" sx={{ mb: 1.5 }}>
            The running sandbox predates the latest settings. Restart the agent to apply them.
          </Typography>
        )}

        <Box
          sx={{
            display: 'grid',
            gridTemplateColumns: { xs: '1fr 1fr', sm: '1fr 1fr 1fr' },
            gap: 2,
            p: 1.5,
            borderRadius: 2,
            background: 'linear-gradient(145deg, rgba(255,255,255,0.03) 0%, rgba(255,255,255,0.01) 100%)',
            border: '1px solid rgba(255,255,255,0.06)',
          }}
        >
          <Stat
            icon={headless ? <SquareTerminal size={13} /> : <Monitor size={13} />}
            label="Environment"
            value={runtime}
            note={ownRuntime ? undefined : 'Org default'}
          />
          <Stat icon={<Cpu size={13} />} label="Compute" value={size} note={ownSize ? undefined : 'Org default'} />
          <Stat icon={<BoxIcon size={13} />} label="Status" value={statusLabel} />
        </Box>

        <Stack spacing={0} sx={{ mt: 2 }}>
          {bot.sandbox_id && (
            <IdRow
              label="Sandbox"
              value={bot.sandbox_id}
              openLabel="Open sandbox"
              onOpen={() => router.navigate('org_sandbox_detail', { org_id: organizationId, sandbox_id: bot.sandbox_id! })}
            />
          )}
          <IdRow label="Session" value={sessionId} />
          {bot.project_id && (
            <IdRow
              label="Project"
              value={bot.project_id}
              openLabel="Open project"
              onOpen={() => router.navigate('org_project-specs', { org_id: organizationId, id: bot.project_id! })}
            />
          )}
        </Stack>

        {bot.project_id && (
          <Stack direction="row" spacing={1} sx={{ mt: 2 }}>
            <Button
              size="small"
              variant="outlined"
              startIcon={<FolderKanban size={15} />}
              onClick={() => router.navigate('org_project-specs', { org_id: organizationId, id: bot.project_id! })}
            >
              Project board
            </Button>
          </Stack>
        )}
      </Box>

      {agentID && detail?.bot?.id && (
        <>
          <Section sx={panelSx} title="Agent" description="Name, coding harness, model and reasoning effort. Changes save as you make them.">
            <OrgAgentSettings agentID={agentID} section="basics" readOnly={false} embedded detail={detail} onCanonicalUpdate={onSaved} />
          </Section>
          <Section
            sx={panelSx}
            title="Sandbox"
            description="Environment and compute for this agent's container, and whether its conversation survives between runs. Sandbox changes apply on the next restart."
          >
            <OrgAgentSettings agentID={agentID} section="runtime" readOnly={false} embedded detail={detail} onCanonicalUpdate={onSaved} />
          </Section>
          <Section sx={panelSx} title="Instructions" description="Markdown the agent reads on every run.">
            <OrgAgentSettings agentID={agentID} section="instructions" readOnly={false} embedded detail={detail} onCanonicalUpdate={onSaved} />
          </Section>
          <Section sx={panelSx} title="Org tools" description="Helix organization capabilities available to this agent.">
            <OrgAgentSettings agentID={agentID} section="tools" readOnly={false} embedded detail={detail} onCanonicalUpdate={onSaved} />
          </Section>
          <Section sx={panelSx} title="Triggers" description="What starts this agent: a Trigger directly, or the output of a Processor.">
            <OrgAgentSettings agentID={agentID} section="subscriptions" readOnly={false} embedded detail={detail} onCanonicalUpdate={onSaved} />
          </Section>
          <Section sx={panelSx} title="Project access" description="Projects this agent can work in through its organization tools.">
            <OrgAgentSettings agentID={agentID} section="access" readOnly={false} embedded detail={detail} onCanonicalUpdate={onSaved} />
          </Section>
        </>
      )}

      <Box sx={{ ...panelSx, '& > .MuiBox-root': { mb: 0 } }}>
        <SharePreviewSection sessionId={sessionId} />
      </Box>
    </Stack>
  )
}

export default OrgAgentSettingsPane
