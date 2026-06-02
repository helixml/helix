import { FC, useMemo, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Stack from '@mui/material/Stack'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import AddIcon from '@mui/icons-material/Add'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import CardGrid from '../components/widgets/CardGrid'
import SimpleTable from '../components/widgets/SimpleTable'
import ViewModeToggle from '../components/widgets/ViewModeToggle'
import useRouter from '../hooks/useRouter'
import useViewMode from '../hooks/useViewMode'
import { useHelixOrgWorkers, WorkerDTO } from '../services/helixOrgService'

// HelixOrgWorkers lists every Worker. Click a row/card to open the
// detail editor. "Hire Worker" is deferred — hiring today flows through
// the helix-org chat ("hire me a CEO"), and the underlying tool
// (api/pkg/org/tools/hire_worker.go) is only wired through the MCP
// surface, not as a REST endpoint. Exposing it cleanly requires its
// own API design (slot resolution, request shape, error model) which
// belongs in a follow-up commit, not buried here.
// TODO(Phase-B-followup): wire a direct POST /api/v1/org/workers
// endpoint backed by hire_worker.Tool so the button below can post
// straight from the React form, rather than asking users to bounce
// through chat.

const HelixOrgWorkers: FC = () => {
  const router = useRouter()
  const [viewMode, setViewMode] = useViewMode('helix-org-workers', 'table')
  const { data, isLoading } = useHelixOrgWorkers()
  const workers = data ?? []

  const [menuAnchor, setMenuAnchor] = useState<HTMLElement | null>(null)
  const [currentWorker, setCurrentWorker] = useState<WorkerDTO | null>(null)

  const openMenu = (e: React.MouseEvent<HTMLElement>, worker: WorkerDTO) => {
    e.stopPropagation()
    setMenuAnchor(e.currentTarget)
    setCurrentWorker(worker)
  }
  const closeMenu = () => {
    setMenuAnchor(null)
    setCurrentWorker(null)
  }

  const handleOpen = (worker: WorkerDTO) => {
    router.navigate('helix_org_worker_detail', { org_id: router.params.org_id, worker_id: worker.id })
  }

  const tableData = useMemo(
    () =>
      workers.map((w) => ({
        id: w.id,
        _data: w,
        name: (
          <a
            href="#"
            style={{ color: 'inherit', textDecoration: 'none', fontWeight: 600 }}
            onClick={(e) => {
              e.preventDefault()
              e.stopPropagation()
              handleOpen(w)
            }}
          >
            <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{w.id}</Typography>
          </a>
        ),
        kind: <Typography variant="body2" color="text.secondary">{w.kind}</Typography>,
        position: (
          <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
            {w.position_id || '—'}
          </Typography>
        ),
        org: (
          <Typography variant="body2" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
            {w.organization_id || '—'}
          </Typography>
        ),
      })),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [workers],
  )

  return (
    <Page
      breadcrumbTitle="Workers"
      breadcrumbParent={{ title: 'Helix Org' }}
      topbarContent={(
        <Tooltip title="Hiring runs through the helix-org chat today — open chat and try “hire me a CEO”.">
          <span>
            <Button
              variant="contained"
              color="secondary"
              startIcon={<AddIcon />}
              disabled
            >
              Hire Worker
            </Button>
          </span>
        </Tooltip>
      )}
    >
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Workers</Typography>
            <Typography variant="body2" color="text.secondary">
              Every Worker in the org. Click one to edit its identity or the role it inherits from its Position.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : workers.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 6 }}>
              <Typography variant="body1" color="text.secondary">
                No workers yet — hire one from the chat.
              </Typography>
            </Box>
          ) : (
            <>
              <Box sx={{ display: 'flex', justifyContent: 'flex-end' }}>
                <ViewModeToggle mode={viewMode} onChange={setViewMode} />
              </Box>

              {viewMode === 'table' ? (
                <SimpleTable
                  authenticated
                  fields={[
                    { name: 'name', title: 'ID' },
                    { name: 'kind', title: 'Kind' },
                    { name: 'position', title: 'Position' },
                    { name: 'org', title: 'Organization' },
                  ]}
                  data={tableData}
                  onRowClick={(row) => handleOpen(row._data as WorkerDTO)}
                  getActions={(row) => (
                    <IconButton
                      size="small"
                      onClick={(e) => openMenu(e, row._data as WorkerDTO)}
                    >
                      <MoreVertIcon fontSize="small" />
                    </IconButton>
                  )}
                />
              ) : (
                <CardGrid
                  items={workers}
                  getKey={(w) => w.id}
                  renderCard={(w) => (
                    <Card
                      sx={{
                        border: '1px solid rgba(0, 0, 0, 0.08)',
                        borderRadius: 1,
                        boxShadow: 'none',
                        height: '100%',
                        display: 'flex',
                        flexDirection: 'column',
                        '&:hover': {
                          borderColor: 'rgba(0, 0, 0, 0.12)',
                          backgroundColor: 'rgba(0, 0, 0, 0.01)',
                        },
                      }}
                    >
                      <CardContent
                        sx={{ p: 2, '&:last-child': { pb: 2 }, cursor: 'pointer' }}
                        onClick={() => handleOpen(w)}
                      >
                        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                          <Box sx={{ minWidth: 0 }}>
                            <Typography variant="body2" sx={{ fontWeight: 600, fontFamily: 'monospace' }}>
                              {w.id}
                            </Typography>
                            <Typography variant="caption" color="text.secondary">{w.kind}</Typography>
                          </Box>
                          <IconButton size="small" onClick={(e) => openMenu(e, w)}>
                            <MoreVertIcon sx={{ fontSize: 16 }} />
                          </IconButton>
                        </Box>

                        <Stack direction="row" spacing={1} sx={{ mt: 1.5, flexWrap: 'wrap', gap: 1 }}>
                          {w.position_id && (
                            <Chip
                              size="small"
                              label={`pos ${w.position_id}`}
                              sx={{ fontFamily: 'monospace', fontSize: '0.65rem' }}
                            />
                          )}
                          {w.organization_id && (
                            <Chip
                              size="small"
                              label={w.organization_id}
                              sx={{ fontFamily: 'monospace', fontSize: '0.65rem' }}
                            />
                          )}
                        </Stack>

                        {w.identity_content && (
                          <Typography
                            variant="caption"
                            color="text.secondary"
                            sx={{
                              mt: 1.5,
                              display: '-webkit-box',
                              WebkitLineClamp: 3,
                              WebkitBoxOrient: 'vertical',
                              overflow: 'hidden',
                            }}
                          >
                            {w.identity_content}
                          </Typography>
                        )}
                      </CardContent>
                    </Card>
                  )}
                />
              )}
            </>
          )}
        </Stack>
      </Container>

      <Menu anchorEl={menuAnchor} open={Boolean(menuAnchor)} onClose={closeMenu}>
        <MenuItem
          onClick={(e) => {
            e.stopPropagation()
            if (currentWorker) handleOpen(currentWorker)
            closeMenu()
          }}
        >
          <OpenInNewIcon sx={{ mr: 1, fontSize: 20 }} />
          Open
        </MenuItem>
      </Menu>
    </Page>
  )
}

export default HelixOrgWorkers
