// HelixOrgTopics lists every event topic defined in the current
// org. Topics are the inbound side of the org's I/O: GitHub webhooks,
// Postmark inboxes, plain in-process buses. Workers subscribe via MCP;
// the chart edges (added in the same PR as this page) show which
// worker pulls from which topic.

import { FC, MouseEvent, useEffect, useMemo, useState } from 'react'
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

import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import NewTopicDrawer from '../components/helix-org/NewTopicDrawer'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleTable from '../components/widgets/SimpleTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import {
  TopicDTO,
  useDeleteHelixOrgTopic,
  useListHelixOrgTopics,
} from '../services/helixOrgService'

// topicRowId is the canonical HTML id assigned to each row in the
// topics table. Exported so other components (the chart deep-link,
// for example) can pin the contract — change the format here and all
// callers update at once.
export const topicRowId = (topicId: string) => `topic-row-${topicId}`

// HIGHLIGHT_DURATION_MS is how long the focused-row highlight stays
// up after the chart deep-links into the topics page. Kept short so
// the page doesn't feel busy; long enough for the user to register
// which row they landed on.
const HIGHLIGHT_DURATION_MS = 2400

const HelixOrgTopics: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const theme = useTheme()
  const breadcrumbs = useHelixOrgBreadcrumbs()

  const { data, isLoading } = useListHelixOrgTopics()
  const deleteTopic = useDeleteHelixOrgTopic()

  const topics = data?.topics ?? []
  const [createOpen, setCreateOpen] = useState(false)
  const [deleting, setDeleting] = useState<TopicDTO | undefined>()
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [currentTopic, setCurrentTopic] = useState<TopicDTO | null>(null)

  const focusId = (router.params.focus as string | undefined) ?? undefined
  const [highlightId, setHighlightId] = useState<string | undefined>(undefined)

  // When the page lands with `?focus=<topicId>` (the chart's
  // topic-node click sets this) scroll the matching row into view
  // and pulse a highlight on it. Runs after every render where the
  // focus param or the topics list changes, so the deep-link works
  // even if the API call is still in flight on initial mount.
  useEffect(() => {
    if (!focusId) {
      setHighlightId(undefined)
      return
    }
    if (!topics.some((s) => s.id === focusId)) return
    const el = document.getElementById(topicRowId(focusId))
    if (!el) return
    el.scrollIntoView({ block: 'center', behavior: 'smooth' })
    setHighlightId(focusId)
    const t = setTimeout(() => setHighlightId(undefined), HIGHLIGHT_DURATION_MS)
    return () => clearTimeout(t)
  }, [focusId, topics])

  const handleMenuOpen = (e: MouseEvent<HTMLElement>, s: TopicDTO) => {
    e.stopPropagation()
    setAnchorEl(e.currentTarget)
    setCurrentTopic(s)
  }
  const handleMenuClose = () => {
    setAnchorEl(null)
    setCurrentTopic(null)
  }

  const handleDelete = async () => {
    if (!deleting) return
    try {
      await deleteTopic.mutateAsync(deleting.id)
      snackbar.success(`topic ${deleting.id} deleted`)
    } catch (e: any) {
      snackbar.error(e?.response?.data?.error ?? e?.message ?? 'delete failed')
    } finally {
      setDeleting(undefined)
    }
  }

  const orgSlug = (router.params.org_id as string | undefined) ?? ''
  const openTopicDetail = (sid: string) => {
    if (!orgSlug) return
    router.navigate('helix_org_topic_detail', { org_id: orgSlug, topic_id: sid })
  }

  const tableData = useMemo(() => topics.map((s) => ({
    id: s.id,
    _data: s,
    _isHighlighted: highlightId === s.id,
    name: (
      <Typography variant="body1">
        <a
          href="#"
          onClick={(e) => { e.preventDefault(); e.stopPropagation(); openTopicDetail(s.id) }}
          style={{
            fontWeight: 'bold',
            color: highlightId === s.id
              ? theme.palette.warning.main
              : theme.palette.mode === 'dark' ? theme.palette.text.primary : theme.palette.text.secondary,
            fontFamily: 'monospace',
            textDecoration: 'none',
            cursor: 'pointer',
          }}
        >
          {s.id}
        </a>
      </Typography>
    ),
    nameField: (
      <Typography variant="body2" color="text.secondary">{s.name}</Typography>
    ),
    kind: (
      <Typography variant="body2" sx={{ fontFamily: 'monospace', color: 'text.secondary' }}>{s.kind}</Typography>
    ),
    subscribers: (
      <Typography variant="body2" color="text.secondary">{s.subscribers?.length ?? 0}</Typography>
    ),
    created: (
      <Typography variant="body2" color="text.secondary">
        {s.created_at ? new Date(s.created_at).toLocaleString() : '—'}
      </Typography>
    ),
  })), [topics, theme, highlightId])

  const getActions = (row: any) => {
    const s = row._data as TopicDTO
    return (
      <IconButton size="small" onClick={(e) => handleMenuOpen(e, s)}>
        <MoreVertIcon />
      </IconButton>
    )
  }

  return (
    <HelixOrgShell showChat={false} breadcrumbs={breadcrumbs} breadcrumbTitle="Topics">
      <Box sx={{ height: '100%', overflow: 'auto' }}>
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          <Stack direction="row" justifyContent="space-between" alignItems="flex-start" spacing={2}>
            <Box sx={{ flex: 1 }}>
              <Typography variant="body2" color="text.secondary">
                Named event channels Workers can subscribe to. Each topic carries a Transport (local
                pub/sub, GitHub webhooks, Postmark inbound email, plain webhooks). Workers subscribe via
                the <code>subscribe</code> MCP tool; the chart shows the resulting (worker → topic)
                edges as dashed lines.
              </Typography>
            </Box>
            <Button
              variant="contained"
              color="secondary"
              startIcon={<AddIcon />}
              onClick={() => setCreateOpen(true)}
              sx={{ flexShrink: 0, mt: 0.5 }}
            >
              New topic
            </Button>
          </Stack>

          {isLoading ? (
            <LoadingSpinner />
          ) : topics.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="body1" color="text.secondary" gutterBottom>
                No topics yet.
              </Typography>
              <Button
                variant="contained"
                color="secondary"
                startIcon={<AddIcon />}
                onClick={() => setCreateOpen(true)}
                sx={{ mt: 1 }}
              >
                New topic
              </Button>
            </Box>
          ) : (
            <SimpleTable
              authenticated={true}
              fields={[
                { name: 'name', title: 'ID' },
                { name: 'nameField', title: 'Name' },
                { name: 'kind', title: 'Transport' },
                { name: 'subscribers', title: 'Subscribers' },
                { name: 'created', title: 'Created' },
              ]}
              data={tableData}
              getActions={getActions}
              getRowId={(row) => topicRowId(row.id as string)}
            />
          )}
        </Stack>
      </Container>

      <Menu anchorEl={anchorEl} open={Boolean(anchorEl)} onClose={handleMenuClose}>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            handleMenuClose()
            if (currentTopic) setDeleting(currentTopic)
          }}
        >
          <DeleteOutlineIcon sx={{ mr: 1, fontSize: 20 }} />
          Delete
        </MenuItem>
      </Menu>

      {deleting && (
        <DeleteConfirmWindow
          title="topic"
          submitTitle="Delete"
          onSubmit={handleDelete}
          onCancel={() => setDeleting(undefined)}
        >
          <Typography variant="body1">
            Deleting topic <b style={{ fontFamily: 'monospace' }}>{deleting.id}</b> removes the row.
            Subscriptions + events stay until drained explicitly. This is irreversible.
          </Typography>
        </DeleteConfirmWindow>
      )}

      <NewTopicDrawer open={createOpen} onClose={() => setCreateOpen(false)} />
      </Box>
    </HelixOrgShell>
  )
}

export default HelixOrgTopics
