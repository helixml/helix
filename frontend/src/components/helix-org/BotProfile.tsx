import { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import { alpha } from '@mui/material/styles'

import type { BotDTO } from '../../services/helixOrgService'
import { CHIEF_OF_STAFF_BOT_ID } from '../../utils/organizations'
import type { SandboxIndicatorState } from '../tasks/SandboxStatusIndicator'
import BotPortrait from './BotPortrait'

export const botSandboxIndicatorState = (bot?: BotDTO): SandboxIndicatorState => {
  if (!bot) return 'running'
  if (bot.status === 'running') return 'running'
  if (bot.status === 'starting' || bot.sandbox_status === 'pending') return 'starting'
  return 'stopped'
}

export const botPresence = (bot?: BotDTO): { label: string; color: string } => {
  if (bot?.activation_error) return { label: 'Needs attention', color: 'warning.main' }
  const state = botSandboxIndicatorState(bot)
  if (state === 'starting') return { label: 'Starting', color: 'warning.main' }
  if (state === 'stopped') return { label: 'Stopped', color: 'text.disabled' }
  if (bot?.agent_work_state === 'working') return { label: 'Replying', color: 'info.main' }
  return { label: 'Ready to chat', color: 'success.main' }
}

export const botRole = (bot?: BotDTO): string | undefined => (
  bot?.id === CHIEF_OF_STAFF_BOT_ID
    ? 'I help you set priorities and coordinate your team.'
    : undefined
)

interface BotProfileProps {
  bot: BotDTO
  expanded?: boolean
}

const BotProfile: FC<BotProfileProps> = ({ bot, expanded = false }) => {
  const botName = bot.name || bot.id || 'Helix bot'
  const presence = botPresence(bot)
  const role = botRole(bot)
  const status = (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
      <Box sx={{ width: 8, height: 8, borderRadius: '50%', backgroundColor: presence.color }} />
      <Typography variant="body2" color="text.secondary">
        {presence.label}
      </Typography>
    </Box>
  )

  if (!expanded) {
    return (
      <Box
        data-testid="bot-profile-compact"
        sx={(theme) => ({
          minHeight: 96,
          width: '100%',
          px: { xs: 2, sm: 3 },
          display: 'flex',
          alignItems: 'center',
          position: 'relative',
          overflow: 'hidden',
          backgroundColor: alpha(theme.palette.primary.main, theme.palette.mode === 'light' ? 0.2 : 0.16),
        })}
      >
        <Box sx={{ minWidth: 0, maxWidth: 'calc(100% - 164px)', position: 'relative', zIndex: 1 }}>
          <Typography variant="h5" component="h1" title={botName} noWrap sx={{ fontWeight: 700 }}>
            {botName}
          </Typography>
          <Box sx={{ mt: 0.5, display: 'flex', alignItems: 'center', gap: 1.5, flexWrap: 'wrap' }}>
            <Typography variant="body2" color="text.secondary">Your Helix bot</Typography>
            {status}
          </Box>
        </Box>
        <BotPortrait
          portraitIndex={bot.portrait_index}
          sx={{
            width: 116,
            height: 116,
            position: 'absolute',
            right: { xs: 38, sm: 56 },
            bottom: -28,
          }}
        />
      </Box>
    )
  }

  return (
    <Box
      data-testid="bot-profile-expanded"
      sx={(theme) => ({
        minHeight: { xs: 240, sm: 288 },
        width: '100%',
        px: { xs: 2.5, sm: 4.5 },
        py: { xs: 3, sm: 4 },
        display: 'flex',
        alignItems: 'center',
        position: 'relative',
        overflow: 'hidden',
        backgroundColor: alpha(theme.palette.primary.main, theme.palette.mode === 'light' ? 0.24 : 0.18),
      })}
    >
      <Box sx={{ width: { xs: '62%', sm: '54%' }, minWidth: 0, position: 'relative', zIndex: 1 }}>
        <Typography
          variant="h3"
          component="h1"
          title={botName}
          sx={{
            fontWeight: 700,
            lineHeight: 0.98,
            overflowWrap: 'anywhere',
            display: '-webkit-box',
            WebkitBoxOrient: 'vertical',
            WebkitLineClamp: 3,
            overflow: 'hidden',
          }}
        >
          {botName}
        </Typography>
        <Typography variant="h6" color="text.secondary" sx={{ mt: 1.5, fontWeight: 500 }}>
          Your Helix bot
        </Typography>
        <Box sx={{ mt: 2 }}>{status}</Box>
        {role && (
          <Typography variant="body1" sx={{ mt: 3, maxWidth: 360, lineHeight: 1.55 }}>
            {role}
          </Typography>
        )}
      </Box>
      <BotPortrait
        portraitIndex={bot.portrait_index}
        sx={{
          width: { xs: '54%', sm: '58%' },
          height: '100%',
          position: 'absolute',
          right: { xs: '-12%', sm: '-7%' },
          bottom: 0,
          objectPosition: 'center bottom',
        }}
      />
    </Box>
  )
}

export default BotProfile
