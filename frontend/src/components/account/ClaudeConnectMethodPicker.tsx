import { FC } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import { Clock, FileJson, LogIn, LucideIcon, Terminal, UserCheck, UserX } from 'lucide-react'

import useLightTheme from '../../hooks/useLightTheme'

export type ClaudeConnectMethod = 'oauth' | 'credentials' | 'setup_token'

interface Props {
  value: ClaudeConnectMethod
  onChange: (next: ClaudeConnectMethod) => void
}

interface MethodMeta {
  label: string
  /** One line on what the user actually does. */
  description: string
  /** How long the credential lasts, and who keeps it alive. */
  lifetime: string
  /** What the user needs before they can use this method. */
  requirement: string
  /** Whether Helix can name the Claude account behind the credential. */
  identity: string
  identityKnown: boolean
  accent: string
  Icon: LucideIcon
}

// Every claim here is verified rather than assumed:
//
// - The one-year setup token and its model-requests-only limitation are stated
//   in Anthropic's own docs (code.claude.com/docs/en/authentication,
//   "Generate a long-lived token").
// - The ~9 day refresh window is measured from a live credential file:
//   refreshTokenExpiresAt - expiresAt = 9.04 days.
// - "Helix refreshes it" is the background refresher in
//   claude_subscription_refresher.go, verified against real Anthropic.
// - Setup tokens cannot be profiled because /api/oauth/profile requires
//   user:profile, which they do not carry — confirmed by a 403 against real
//   Anthropic using a stored setup token. See
//   design/2026-08-18-claude-profile-identity.md.
const METHODS: Record<ClaudeConnectMethod, MethodMeta> = {
  oauth: {
    label: 'Sign in with Claude',
    description: 'Authorize in your browser and paste back a short code.',
    lifetime: 'Stays connected — Helix refreshes it automatically',
    requirement: 'Nothing to install',
    identity: 'Shows the Claude account and plan',
    identityKnown: true,
    accent: '#D97757',
    Icon: LogIn,
  },
  credentials: {
    label: 'Paste credentials',
    description: 'Reuse a login you already did with `claude login`.',
    lifetime: 'Stays connected — Helix refreshes it automatically',
    requirement: 'Needs Claude Code signed in on your machine',
    identity: 'Shows the Claude account and plan',
    identityKnown: true,
    accent: '#6E9EE8',
    Icon: FileJson,
  },
  setup_token: {
    label: 'Setup token',
    description: 'Run `claude setup-token` and paste the token.',
    lifetime: 'Lasts one year, then must be replaced by hand',
    requirement: 'Needs the Claude Code CLI on your machine',
    identity: 'Cannot show which account it belongs to',
    identityKnown: false,
    accent: '#8E8E93',
    Icon: Terminal,
  },
}

const ORDER: ClaudeConnectMethod[] = ['oauth', 'credentials', 'setup_token']

const ClaudeConnectMethodPicker: FC<Props> = ({ value, onChange }) => {
  const lightTheme = useLightTheme()

  return (
    <Box
      sx={{
        display: 'grid',
        gridTemplateColumns: { xs: '1fr', sm: 'repeat(3, 1fr)' },
        gap: 1.25,
      }}
    >
      {ORDER.map((method) => {
        const meta = METHODS[method]
        const selected = value === method
        return (
          <Box
            key={method}
            role="radio"
            aria-checked={selected}
            aria-label={meta.label}
            tabIndex={0}
            onClick={() => onChange(method)}
            onKeyDown={(e) => {
              if (e.key === 'Enter' || e.key === ' ') {
                e.preventDefault()
                onChange(method)
              }
            }}
            sx={{
              display: 'flex',
              flexDirection: 'column',
              p: 1.5,
              borderRadius: 1.5,
              cursor: 'pointer',
              userSelect: 'none',
              border: '1px solid',
              borderColor: selected
                ? 'primary.main'
                : lightTheme.isLight
                  ? 'rgba(0,0,0,0.08)'
                  : 'rgba(255,255,255,0.08)',
              bgcolor: selected
                ? 'rgba(33, 150, 243, 0.08)'
                : lightTheme.isLight
                  ? 'rgba(0,0,0,0.02)'
                  : 'rgba(255,255,255,0.02)',
              transition: 'border-color 120ms, background-color 120ms',
              '&:hover': {
                borderColor: selected
                  ? 'primary.main'
                  : lightTheme.isLight
                    ? 'rgba(0,0,0,0.18)'
                    : 'rgba(255,255,255,0.18)',
              },
              '&:focus-visible': {
                outline: '2px solid',
                outlineColor: 'primary.main',
                outlineOffset: 2,
              },
            }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.75 }}>
              <Box
                sx={{
                  width: 28,
                  height: 28,
                  borderRadius: '50%',
                  display: 'flex',
                  alignItems: 'center',
                  justifyContent: 'center',
                  flexShrink: 0,
                  bgcolor: selected ? `${meta.accent}33` : `${meta.accent}1f`,
                }}
              >
                <meta.Icon size={16} color={meta.accent} />
              </Box>
              <Typography variant="body2" sx={{ fontWeight: 600, lineHeight: 1.2 }}>
                {meta.label}
              </Typography>
            </Box>

            <Typography
              variant="caption"
              color="text.secondary"
              sx={{ display: 'block', mb: 1, lineHeight: 1.4 }}
            >
              {meta.description}
            </Typography>

            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5, mt: 'auto' }}>
              <Fact icon={<Clock size={13} />} text={meta.lifetime} />
              <Fact icon={<Terminal size={13} />} text={meta.requirement} />
              <Fact
                icon={
                  meta.identityKnown ? <UserCheck size={13} /> : <UserX size={13} />
                }
                text={meta.identity}
                muted={!meta.identityKnown}
              />
            </Box>
          </Box>
        )
      })}
    </Box>
  )
}

const Fact: FC<{ icon: React.ReactNode; text: string; muted?: boolean }> = ({
  icon,
  text,
  muted = false,
}) => (
  <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 0.75 }}>
    <Box
      sx={{
        display: 'flex',
        alignItems: 'center',
        mt: '1px',
        flexShrink: 0,
        color: muted ? 'warning.main' : 'text.secondary',
      }}
    >
      {icon}
    </Box>
    <Typography
      variant="caption"
      sx={{
        fontSize: '0.68rem',
        lineHeight: 1.35,
        color: muted ? 'warning.main' : 'text.secondary',
      }}
    >
      {text}
    </Typography>
  </Box>
)

export default ClaudeConnectMethodPicker
