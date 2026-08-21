import { FC, MouseEvent, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import Container from '@mui/material/Container'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { EllipsisVertical, Plus, Trash2 } from 'lucide-react'
import CardGrid from '../components/widgets/CardGrid'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import SimpleTable from '../components/widgets/SimpleTable'
import ViewModeToggle from '../components/widgets/ViewModeToggle'
import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import TriggerFormDialog from '../components/helix-org/TriggerFormDialog'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useViewMode from '../hooks/useViewMode'
import { matchesAllTokens } from '../utils/searchUtils'
import { TriggerDTO, useCreateTrigger, useDeleteTrigger, useTriggers } from '../services/triggerService'

const apiMessage = (error: any) => error?.response?.data?.summary ?? error?.message ?? 'The request failed.'

const HelixOrgTriggers: FC = () => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const breadcrumbs = useHelixOrgBreadcrumbs()
  const { data = [], isLoading } = useTriggers()
  const create = useCreateTrigger()
  const remove = useDeleteTrigger()
  const [mode, setMode] = useViewMode('helix-org-triggers-view')
  const [query, setQuery] = useState('')
  const [createOpen, setCreateOpen] = useState(false)
  const [deleting, setDeleting] = useState<TriggerDTO>()
  const [current, setCurrent] = useState<TriggerDTO>()
  const [anchor, setAnchor] = useState<HTMLElement | null>(null)
  const [formError, setFormError] = useState('')
  const orgID = router.params.org_id as string
  const filtered = useMemo(() => data.filter((t) => matchesAllTokens(query, [t.name, t.id, t.kind, t.description].filter(Boolean).join(' '))), [data, query])
  const open = (id?: string) => id && router.navigate('helix_org_trigger_detail', { org_id: orgID, trigger_id: id })
  const openMenu = (event: MouseEvent<HTMLElement>, trigger: TriggerDTO) => { event.stopPropagation(); setCurrent(trigger); setAnchor(event.currentTarget) }
  const closeMenu = () => { setAnchor(null); setCurrent(undefined) }

  const actions = (trigger: TriggerDTO) => (
    <Tooltip title="Trigger actions">
      <IconButton aria-label={`Actions for ${trigger.name}`} onClick={(e) => openMenu(e, trigger)}><EllipsisVertical size={18} /></IconButton>
    </Tooltip>
  )
  const tableData = useMemo(() => filtered.map((trigger) => ({
    id: trigger.id,
    _data: trigger,
    name: <a href="#" onClick={(e) => { e.preventDefault(); e.stopPropagation(); open(trigger.id) }} style={{ fontWeight: 600, color: 'inherit', textDecoration: 'none' }}>{trigger.name}</a>,
    kind: <Typography variant="body2" color="text.secondary">{trigger.kind}</Typography>,
    description: <Typography variant="body2" color="text.secondary">{trigger.description || '—'}</Typography>,
    created: <Typography variant="body2" color="text.secondary">{trigger.created_at ? new Date(trigger.created_at).toLocaleString() : '—'}</Typography>,
  })), [filtered, orgID])

  return (
    <HelixOrgShell showChat={false} breadcrumbs={breadcrumbs} breadcrumbTitle="Triggers">
      <Box sx={{ height: '100%', overflow: 'auto' }}><Container maxWidth="xl" sx={{ py: 3 }}><Stack spacing={2}>
        <Stack direction="row" justifyContent="space-between" alignItems="center">
          <Box><Typography variant="h5">Triggers</Typography><Typography variant="body2" color="text.secondary">Inbound events that can activate Workers directly or through Processor outputs.</Typography></Box>
          <Button variant="contained" startIcon={<Plus size={18} />} onClick={() => { setFormError(''); setCreateOpen(true) }}>New trigger</Button>
        </Stack>
        <TextField size="small" placeholder="Search triggers" value={query} onChange={(e) => setQuery(e.target.value)} sx={{ maxWidth: 360 }} />
        <Stack direction="row" justifyContent="flex-end"><ViewModeToggle mode={mode} onChange={setMode} /></Stack>
        {isLoading ? <LoadingSpinner /> : filtered.length === 0 ? <Typography color="text.secondary" sx={{ py: 6, textAlign: 'center' }}>{query ? 'No triggers match your search.' : 'No triggers yet.'}</Typography> : mode === 'table' ? (
          <SimpleTable authenticated fields={[{ name: 'name', title: 'Name' }, { name: 'kind', title: 'Source' }, { name: 'description', title: 'Description' }, { name: 'created', title: 'Created' }]} data={tableData} getActions={(row) => actions(row._data as TriggerDTO)} />
        ) : <CardGrid items={filtered} getKey={(t) => t.id!} renderCard={(trigger) => (
          <Card sx={{ border: '1px solid rgba(0, 0, 0, 0.08)', borderRadius: 1, boxShadow: 'none', height: '100%', '&:hover': { borderColor: 'rgba(0,0,0,0.12)', backgroundColor: 'rgba(0,0,0,0.01)' } }}>
            <CardContent onClick={() => open(trigger.id)} sx={{ p: 2, '&:last-child': { pb: 2 }, cursor: 'pointer' }}><Stack direction="row" justifyContent="space-between"><Box><Typography fontWeight={600}>{trigger.name}</Typography><Typography variant="caption" color="text.secondary">{trigger.kind}</Typography></Box>{actions(trigger)}</Stack><Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>{trigger.description || 'No description'}</Typography></CardContent>
          </Card>
        )} />}
      </Stack></Container></Box>
      <Menu anchorEl={anchor} open={!!anchor} onClose={closeMenu}><MenuItem onClick={(e) => { e.stopPropagation(); const selected = current; closeMenu(); setDeleting(selected) }}><Trash2 size={20} /> Delete</MenuItem></Menu>
      {deleting && <DeleteConfirmWindow title="trigger" submitTitle="Delete" onCancel={() => setDeleting(undefined)} onSubmit={async () => { try { await remove.mutateAsync(deleting.id!); snackbar.success('Trigger deleted') } catch (e) { snackbar.error(apiMessage(e)) } finally { setDeleting(undefined) } }}><Typography>Delete <b>{deleting.name}</b>? Attached Workers must be detached first.</Typography></DeleteConfirmWindow>}
      <TriggerFormDialog open={createOpen} saving={create.isPending} error={formError} onClose={() => setCreateOpen(false)} onSubmit={async (payload) => { try { const trigger = await create.mutateAsync(payload); setCreateOpen(false); open(trigger.id) } catch (e) { setFormError(apiMessage(e)) } }} />
    </HelixOrgShell>
  )
}

export default HelixOrgTriggers
