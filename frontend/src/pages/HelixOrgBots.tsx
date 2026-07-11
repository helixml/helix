// HelixOrgBots lists every bot defined in the current org and is the
// entry point for the standalone "Bots" page in the helix-org middle-nav.
// Clicking a row navigates to /helix-org/bots/:bot_id, which shows the
// bot's content, tools, subscriptions, reporting lines and an inline chat
// transcript. The chart's bot nodes navigate to the same detail page.
//
// A Bot is the merge of the former Role and Worker concepts: it carries
// its own markdown content (its identity/prompt), an MCP tools list, a
// topics manifest, and the bots it reports to (parent_ids).

import { FC, MouseEvent, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Container from '@mui/material/Container'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import useTheme from '@mui/material/styles/useTheme'
import AddIcon from '@mui/icons-material/Add'
import DeleteOutlineIcon from '@mui/icons-material/DeleteOutline'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'

import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import NewBotDialog from '../components/helix-org/NewBotDialog'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleTable from '../components/widgets/SimpleTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  BotDTO,
  useDeleteBot,
  useListHelixOrgBots,
} from '../services/helixOrgService'

const HelixOrgBots: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const theme = useTheme()
  const orgSlug = router.params.org_id as string | undefined

  const { data, isLoading } = useListHelixOrgBots()
  const deleteBot = useDeleteBot()

  // People (kind=human) live in the chart's People panel, not the Bots list.
  const bots = (data ?? []).filter((b) => b.kind !== 'human')
  const [deleting, setDeleting] = useState<BotDTO | undefined>()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentBot, setCurrentBot] = useState<BotDTO | null>(null)
  const [newBotOpen, setNewBotOpen] = useState(false)

  const openBot = (botId: string) => {
    if (!orgSlug) return
    router.navigate('helix_org_bot_detail', { org_id: orgSlug, bot_id: botId })
  }

  const handleMenuOpen = (e: MouseEvent<HTMLElement>, bot: BotDTO) => {
    e.stopPropagation()
    setAnchorEl(e.currentTarget)
    setCurrentBot(bot)
  }
  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentBot(null)
  }

  const handleDelete = async () => {
    if (!deleting) return
    try {
      await deleteBot.mutateAsync(deleting.id ?? '')
      snackbar.success(`deleted bot ${deleting.id}`)
    } catch (e: any) {
      const status = e?.response?.status
      if (status === 409) {
        snackbar.error('owner bot is protected and cannot be deleted')
      } else {
        snackbar.error(e?.response?.data?.error ?? e?.message ?? 'delete failed')
      }
    } finally {
      setDeleting(undefined)
    }
  }

  const tableData = useMemo(() => bots.map((b) => ({
    id: b.id,
    _data: b,
    name: (
      <Typography variant="body1">
        <a
          style={{
            textDecoration: 'none',
            fontWeight: 'bold',
            color: theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
            fontFamily: 'monospace',
          }}
          href="#"
          onClick={(e) => { e.preventDefault(); e.stopPropagation(); openBot(b.id ?? '') }}
        >
          {b.id}
        </a>
      </Typography>
    ),
    contentPreview: (
      <Typography
        variant="body2"
        color="text.secondary"
        sx={{
          whiteSpace: 'nowrap',
          overflow: 'hidden',
          textOverflow: 'ellipsis',
          maxWidth: 360,
        }}
      >
        {(b.content || '').split('\n').find((l) => l.trim() !== '')?.replace(/^#+\s*/, '').slice(0, 80) || '—'}
      </Typography>
    ),
    tools: (
      <Typography variant="body2" color="text.secondary">
        {b.tools?.length ?? 0}
      </Typography>
    ),
    reportsTo: (
      <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
        {(b.parent_ids ?? []).join(', ') || '—'}
      </Typography>
    ),
    updated: (
      <Typography variant="body2" color="text.secondary">
        {b.updated_at ? new Date(b.updated_at).toLocaleString() : '—'}
      </Typography>
    ),
  })), [bots, theme])

  const getActions = (row: any) => {
    const bot = row._data as BotDTO
    return (
      <IconButton size="small" onClick={(e) => handleMenuOpen(e, bot)}>
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <HelixOrgShell>
      <Box sx={{ height: '100%', overflow: 'auto' }}>
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={2}>
            <Box sx={{ flex: 1 }}>
              <Typography variant="h5" sx={{ mb: 1 }}>Bots</Typography>
              <Typography variant="body2" color="text.secondary">
                Agents in this org. Click a bot to open its detail page — edit instructions,
                tools and subscriptions. Chat lives in the left panel.
              </Typography>
            </Box>
            <Button
              variant="contained"
              color="secondary"
              startIcon={<AddIcon />}
              onClick={() => setNewBotOpen(true)}
              sx={{ flexShrink: 0, mt: 0.5 }}
            >
              New bot
            </Button>
          </Stack>

          {isLoading ? (
            <LoadingSpinner />
          ) : bots.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="body1" color="text.secondary" gutterBottom>
                No bots defined yet.
              </Typography>
              <Button
                variant="contained"
                color="secondary"
                startIcon={<AddIcon />}
                onClick={() => setNewBotOpen(true)}
                sx={{ mt: 1 }}
              >
                New bot
              </Button>
            </Box>
          ) : (
            <SimpleTable
              authenticated={true}
              fields={[
                { name: 'name', title: 'ID' },
                { name: 'contentPreview', title: 'Content' },
                { name: 'tools', title: 'Tools' },
                { name: 'reportsTo', title: 'Reports to' },
                { name: 'updated', title: 'Updated' },
              ]}
              data={tableData}
              getActions={getActions}
            />
          )}
        </Stack>
      </Container>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={handleMenuClose}>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentBot) openBot(currentBot.id ?? '')
          }}
        >
          <OpenInNewIcon sx={{ mr: 1, fontSize: 20 }} />
          Open
        </MenuItem>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentBot) setDeleting(currentBot)
          }}
        >
          <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>

      {deleting && (
        <DeleteConfirmWindow
          title="bot"
          submitTitle="Delete"
          onSubmit={handleDelete}
          onCancel={() => setDeleting(undefined)}
        >
          <Typography variant="body1">
            Deleting bot <b style={{ fontFamily: 'monospace' }}>{deleting.id}</b> tears down its
            per-bot Helix project + agent app, drops its subscriptions, and removes it as a
            manager from its direct reports. This is irreversible.
          </Typography>
        </DeleteConfirmWindow>
      )}

      <NewBotDialog open={newBotOpen} onClose={() => setNewBotOpen(false)} />
      </Box>
    </HelixOrgShell>
  )
}

export default HelixOrgBots
