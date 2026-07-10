import React, { useState } from 'react'
import { Paper, Box, Stack, Typography, IconButton, Tooltip, TextField, Button, CircularProgress } from '@mui/material'
import ReactMarkdown from 'react-markdown'
import { MessageSquare, X, Send } from 'lucide-react'
import { useAttentionEvents } from '../../hooks/useAttentionEvents'
import useLightTheme from '../../hooks/useLightTheme'

const ACCENT = '#14b8a6'

// BotPendingQuestions renders the questions this bot has asked the current user
// (via ask_human) as its own section on the bot page, above the agent panel,
// with a reply box directly underneath — so answering feels like replying to a
// message, not hunting for a separate chat. The ask_human message is delivered
// as a notification, not written into the bot's session, so the raw transcript
// only shows the agent's tool-calls; this surfaces the actual question. Sending
// a reply drives the bot's session (onReply) and dismisses the question(s).
const BotPendingQuestions: React.FC<{
  botId: string
  botName?: string
  onReply?: (message: string) => Promise<void>
}> = ({ botId, botName, onReply }) => {
  // Default (non-"mine") query: the user's own active (not dismissed) events —
  // exactly the org_messages addressed to this user. The "mine" variant filters
  // these out, so don't use it here.
  const { events, dismiss } = useAttentionEvents(true, false)
  const lightTheme = useLightTheme()
  const [reply, setReply] = useState('')
  const [sending, setSending] = useState(false)

  const questions = (events || []).filter(
    (e) =>
      e.event_type === 'org_message' &&
      (e.metadata as { bot_id?: string } | undefined)?.bot_id === botId,
  )
  if (questions.length === 0) return null

  const send = async () => {
    const text = reply.trim()
    if (!text || !onReply || sending) return
    setSending(true)
    try {
      await onReply(text)
      questions.forEach((q) => dismiss(q.id))
      setReply('')
    } finally {
      setSending(false)
    }
  }

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
      <Stack spacing={2}>
        {questions.map((q) => (
          <Box key={q.id}>
            <Stack direction="row" alignItems="center" spacing={1} sx={{ mb: 0.75 }}>
              <MessageSquare size={16} style={{ color: ACCENT, flexShrink: 0 }} />
              <Typography variant="subtitle2" sx={{ fontWeight: 700, color: lightTheme.textColor, flex: 1 }}>
                {botName ? `${botName} asked you` : q.title}
              </Typography>
              <Tooltip title="Dismiss without replying" placement="left">
                <IconButton size="small" onClick={() => dismiss(q.id)} sx={{ p: 0.5 }}>
                  <X size={15} />
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

        {onReply ? (
          <Box>
            <TextField
              fullWidth
              multiline
              minRows={2}
              maxRows={8}
              size="small"
              placeholder={`Reply to ${botName || 'the bot'}…`}
              value={reply}
              onChange={(e) => setReply(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter' && !e.shiftKey) {
                  e.preventDefault()
                  void send()
                }
              }}
              disabled={sending}
              sx={{ backgroundColor: (theme) => theme.palette.background.paper, borderRadius: 1 }}
            />
            <Stack direction="row" justifyContent="flex-end" alignItems="center" spacing={1} sx={{ mt: 1 }}>
              <Typography variant="caption" color="text.secondary">Enter to send · Shift+Enter for a new line</Typography>
              <Button
                variant="contained"
                size="small"
                onClick={() => void send()}
                disabled={!reply.trim() || sending}
                startIcon={sending ? <CircularProgress size={14} color="inherit" /> : <Send size={14} />}
              >
                {sending ? 'Sending…' : 'Send reply'}
              </Button>
            </Stack>
          </Box>
        ) : (
          <Typography variant="caption" color="text.secondary">
            The agent isn’t running yet — open Agent activity below to start it, then reply.
          </Typography>
        )}
      </Stack>
    </Paper>
  )
}

export default BotPendingQuestions
