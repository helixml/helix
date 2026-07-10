import React from 'react'
import { Box, Stack, Typography, IconButton, Tooltip } from '@mui/material'
import ReactMarkdown from 'react-markdown'
import { MessageSquare, Check } from 'lucide-react'
import { useAttentionEvents } from '../../hooks/useAttentionEvents'
import useLightTheme from '../../hooks/useLightTheme'

// BotPendingQuestions surfaces the questions this bot has asked the current user
// (via ask_human) as readable message bubbles at the top of the bot's chat
// panel. The ask_human message is delivered as a notification, not written into
// the bot's session, so the chat transcript only shows the agent's raw
// tool-calls / "thinking" — not the actual question. This renders the question
// (markdown, good contrast) so the human sees exactly what to respond to, with
// the reply box directly below. "Mark answered" dismisses the attention event.
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
    <Stack
      spacing={1.5}
      sx={{
        p: 1.5,
        flexShrink: 0,
        maxHeight: 240,
        overflowY: 'auto',
        borderBottom: (theme) =>
          `1px solid ${theme.palette.mode === 'light' ? 'rgba(0,0,0,0.08)' : 'rgba(255,255,255,0.08)'}`,
        backgroundColor: (theme) =>
          theme.palette.mode === 'light' ? 'rgba(20,184,166,0.06)' : 'rgba(20,184,166,0.08)',
      }}
    >
      {questions.map((q) => (
        <Box key={q.id}>
          <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 0.5 }}>
            <MessageSquare size={15} style={{ color: '#14b8a6', flexShrink: 0 }} />
            <Typography variant="caption" sx={{ fontWeight: 700, color: lightTheme.textColor, flex: 1 }}>
              {botName ? `${botName} asked you` : q.title}
            </Typography>
            <Tooltip title="Mark answered" placement="left">
              <IconButton size="small" onClick={() => dismiss(q.id)} sx={{ p: 0.25 }}>
                <Check size={15} />
              </IconButton>
            </Tooltip>
          </Stack>
          <Box
            sx={{
              color: lightTheme.textColor,
              fontSize: '0.85rem',
              lineHeight: 1.55,
              '& p': { m: 0, mb: 1 },
              '& p:last-child': { mb: 0 },
              '& ol, & ul': { pl: 2.5, m: 0, mb: 1 },
              '& li': { mb: 0.5 },
              '& strong': { fontWeight: 700 },
              '& code': {
                px: 0.5,
                borderRadius: 0.5,
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
  )
}

export default BotPendingQuestions
