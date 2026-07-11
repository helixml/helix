import { FC, useEffect, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Typography from '@mui/material/Typography'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import SaveIcon from '@mui/icons-material/Save'

import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import { useHelixOrgBot, useUpdateBot } from '../services/helixOrgService'

// The contact channels surfaced as editable fields. Any other identity keys
// the node happens to carry are preserved untouched on save.
const CHANNELS: { key: string; label: string; placeholder: string }[] = [
  { key: 'slack', label: 'Slack', placeholder: '@handle' },
  { key: 'github', label: 'GitHub', placeholder: 'login' },
  { key: 'email', label: 'Email', placeholder: 'name@example.com' },
]

// HelixOrgHumanDetail is the editable profile for a person (a Bot with
// kind=human). The key bits are the contact channels the org reaches them on
// and their responsibility (what bots resolve for "who owns X"). No agent
// surfaces — a human never runs.
const HelixOrgHumanDetail: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const botId = router.params.bot_id as string | undefined
  const { data, isLoading } = useHelixOrgBot(botId)
  const bot = data?.bot
  const updateBot = useUpdateBot()

  const [handles, setHandles] = useState<Record<string, string>>({})
  const [responsibility, setResponsibility] = useState('')

  useEffect(() => {
    setHandles({ ...(bot?.identity ?? {}) })
    setResponsibility(bot?.content ?? '')
  }, [bot?.identity, bot?.content])

  const dirty = useMemo(() => {
    if (!bot) return false
    if ((bot.content ?? '') !== responsibility) return true
    const orig = bot.identity ?? {}
    const keys = new Set([...Object.keys(orig), ...Object.keys(handles)])
    for (const k of keys) {
      if ((orig[k] ?? '') !== (handles[k] ?? '')) return true
    }
    return false
  }, [bot, handles, responsibility])

  const setHandle = (key: string, value: string) => setHandles((h) => ({ ...h, [key]: value }))

  const handleSave = async () => {
    if (!bot?.id) return
    // Rebuild the identity map from the current fields, dropping empties.
    const identity: Record<string, string> = {}
    for (const [k, v] of Object.entries(handles)) {
      if (v.trim() !== '') identity[k] = v.trim()
    }
    try {
      await updateBot.mutateAsync({ id: bot.id, content: responsibility, identity })
      snackbar.success('Saved')
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'save failed')
    }
  }

  return (
    <HelixOrgShell
      title={bot?.name || botId || 'Person'}
      topbarActions={(
        <Button
          variant="contained"
          color="secondary"
          size="small"
          startIcon={<SaveIcon />}
          disabled={!dirty || updateBot.isPending}
          onClick={handleSave}
        >
          {updateBot.isPending ? 'Saving…' : 'Save'}
        </Button>
      )}
    >
      <Box sx={{ height: '100%', overflow: 'auto' }}>
      <Container maxWidth="md" sx={{ mb: 4, pt: 3 }}>
        {isLoading || !bot ? (
          <LoadingSpinner />
        ) : (
          <Stack spacing={3}>
            <Stack direction="row" alignItems="center" spacing={1.5}>
              <PersonOutlineIcon sx={{ fontSize: 28, color: 'rgba(60,140,210,0.9)' }} />
              <Box sx={{ minWidth: 0 }}>
                <Typography variant="h5">{bot.name || bot.id}</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                  {bot.id}
                </Typography>
              </Box>
              <Chip size="small" label="Person" sx={{ ml: 1 }} />
            </Stack>

            <Paper variant="outlined" sx={{ p: 2.5 }}>
              <Typography variant="subtitle2" sx={{ mb: 0.5 }}>Contact channels</Typography>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 2 }}>
                How the org reaches this person. Bots resolve these when they need to message them.
              </Typography>
              <Stack spacing={2}>
                {CHANNELS.map((c) => (
                  <TextField
                    key={c.key}
                    label={c.label}
                    value={handles[c.key] ?? ''}
                    onChange={(e) => setHandle(c.key, e.target.value)}
                    placeholder={c.placeholder}
                    size="small"
                    fullWidth
                  />
                ))}
              </Stack>
            </Paper>

            <Paper variant="outlined" sx={{ p: 2.5 }}>
              <Typography variant="subtitle2" sx={{ mb: 0.5 }}>Responsibility</Typography>
              <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mb: 2 }}>
                What this person is responsible for — bots resolve this for "who owns X".
              </Typography>
              <TextField
                value={responsibility}
                onChange={(e) => setResponsibility(e.target.value)}
                placeholder="e.g. Point of contact for the billing code; runs commercial sales meetings."
                multiline
                minRows={3}
                fullWidth
              />
            </Paper>
          </Stack>
        )}
      </Container>
      </Box>
    </HelixOrgShell>
  )
}

export default HelixOrgHumanDetail
