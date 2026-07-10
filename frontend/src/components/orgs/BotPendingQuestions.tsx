import React from 'react'
import { Paper, Box, Stack, Typography, IconButton, Tooltip } from '@mui/material'
import ReactMarkdown from 'react-markdown'
import { MessageSquare, Check } from 'lucide-react'
import { useAttentionEvents } from '../../hooks/useAttentionEvents'
import useLightTheme from '../../hooks/useLightTheme'

const ACCENT = '#14b8a6'

// BotPendingQuestions renders the questions this bot has asked the current user
// (via ask_human) as its own section on the bot page, above the chat panel. The
// ask_human message is delivered as a notification, not written into the bot's
// session, so the chat transcript only shows the agent's raw tool-calls / state
// summary — not the actual question. This surfaces the question (markdown, good
// contrast) so the human sees exactly what to respond to; the reply box is the
// chat panel directly below. "Mark answered" dismisses the attention event.
const BotPendingQuestions: React.FC<{ botId: string; botName?: string }> = ({ botId, botName }) => {
  // Default (non-"mine") query: the user's own active (not dismissed) events —
  // exactly the org_messages addressed to this user. The "mine" variant filters
  // these out, so don't use it here.
  const { events, dismiss } = useAttentionEvents(true, false)
  const lightTheme = useLightTheme()

  const questions = (events || []).filter(
    (e) =>
      e.event_type === 'org_message' &&
      (e.metadata as { bot_id?: string } | undefined)?.bot_id === botId,
  )
  if (questions.length === 0) return null

  return (
    <Paper
      variant="outlined"
      sx={{
        p: 2,
        borderLeft: `3px solid ${ACCENT}`,
        backgroundColor: (theme) =>
          theme.palette.mode === 'light' ? 'rgba(20,184,166,0.05)' : 'rgba(20,184,166,0.08)',
      }}
    >
      <Stack spacing={2.5}>
        {questions.map((q) => (
          <Box key={q.id}>
            <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 0.75 }}>
              <MessageSquare size={16} style={{ color: ACCENT, flexShrink: 0 }} />
              <Typography variant="subtitle2" sx={{ fontWeight: 700, color: lightTheme.textColor, flex: 1 }}>
                {botName ? `${botName} asked you` : q.title}
              </Typography>
              <Tooltip title="Mark answered" placement="left">
                <IconButton size="small" onClick={() => dismiss(q.id)} sx={{ p: 0.5 }}>
                  <Check size={16} />
                </IconButton>
              </Tooltip>
            </Stack>
            <Box
              sx={{
                color: lightTheme.textColor,
                fontSize: '0.9rem',
                lineHeight: 1.6,
                '& p': { m: 0, mb: 1 },
                '& p:last-child': { mb: 0 },
                '& ol, & ul': { pl: 3, m: 0, mb: 1 },
                '& li': { mb: 0.5 },
                '& strong': { fontWeight: 700 },
                '& code': {
                  px: 0.5,
                  borderRadius: 0.5,
                  fontSize: '0.85em',
                  backgroundColor: (theme) =>
                    theme.palette.mode === 'light' ? 'rgba(0,0,0,0.06)' : 'rgba(255,255,255,0.1)',
                },
              }}
            >
              <ReactMarkdown>{q.description || ''}</ReactMarkdown>
            </Box>
          </Box>
        ))}
      </Stack>
    </Paper>
  )
}

export default BotPendingQuestions
