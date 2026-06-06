// HelixOrgChart renders the helix-org overview: every Role with the
// Workers currently holding it. Replaces the old position-tree canvas
// after Positions were removed from the domain. The route name
// `helix_org_chart` is kept so existing deep links keep resolving.

import { FC } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Card from '@mui/material/Card'
import CardContent from '@mui/material/CardContent'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import PersonOutlineIcon from '@mui/icons-material/PersonOutline'
import SmartToyOutlinedIcon from '@mui/icons-material/SmartToyOutlined'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useRouter from '../hooks/useRouter'
import {
  useHelixOrgOverview,
  WorkerBadge,
} from '../services/helixOrgService'

const HelixOrgChart: FC = () => {
  const router = useRouter()
  const orgSlug = router.params.org_id as string | undefined
  const { data, isLoading } = useHelixOrgOverview()

  const groups = data?.groups ?? []

  const openWorker = (workerId: string) => {
    if (!orgSlug) return
    router.navigate('helix_org_worker_detail', { org_id: orgSlug, worker_id: workerId })
  }
  const openRole = (roleId: string) => {
    if (!orgSlug) return
    router.navigate('helix_org_role_detail', { org_id: orgSlug, role_id: roleId })
  }

  return (
    <Page breadcrumbTitle="Overview">
      <Container maxWidth="xl" sx={{ mb: 4, pt: 3 }}>
        <Stack spacing={2}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Overview</Typography>
            <Typography variant="body2" color="text.secondary">
              Roles in this org, with the Workers currently holding each one.
              Click a Role to edit its prompt and tool set, or a Worker to
              open its detail page.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : groups.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 8 }}>
              <Typography variant="body1" color="text.secondary">
                No roles yet. Create one from the Roles tab.
              </Typography>
            </Box>
          ) : (
            <Box
              sx={{
                display: 'grid',
                gridTemplateColumns: 'repeat(auto-fill, minmax(320px, 1fr))',
                gap: 2,
              }}
            >
              {groups.map((g) => (
                <Card
                  key={g.role_id}
                  variant="outlined"
                  sx={{
                    borderRadius: 1,
                    boxShadow: 'none',
                    border: '1px solid rgba(0, 0, 0, 0.08)',
                    height: '100%',
                    display: 'flex',
                    flexDirection: 'column',
                  }}
                >
                  <CardContent sx={{ p: 2, '&:last-child': { pb: 2 } }}>
                    <Stack spacing={1.5}>
                      <Button
                        variant="text"
                        onClick={() => openRole(g.role_id!)}
                        sx={{
                          alignSelf: 'flex-start',
                          fontFamily: 'monospace',
                          textTransform: 'none',
                          fontSize: '0.95rem',
                          fontWeight: 600,
                          p: 0,
                          minWidth: 0,
                        }}
                      >
                        {g.role_id}
                      </Button>
                      <Typography variant="caption" color="text.secondary">
                        {(g.workers ?? []).length === 0
                          ? 'No Workers yet'
                          : `${(g.workers ?? []).length} Worker${(g.workers ?? []).length === 1 ? '' : 's'}`}
                      </Typography>
                      <Stack direction="row" spacing={1} flexWrap="wrap" useFlexGap>
                        {(g.workers ?? []).map((w: WorkerBadge) => (
                          <Chip
                            key={w.id}
                            label={w.id}
                            icon={
                              w.kind === 'ai' ? (
                                <SmartToyOutlinedIcon sx={{ fontSize: 14 }} />
                              ) : (
                                <PersonOutlineIcon sx={{ fontSize: 14 }} />
                              )
                            }
                            onClick={(e) => {
                              e.stopPropagation()
                              openWorker(w.id!)
                            }}
                            size="small"
                            sx={{ fontFamily: 'monospace' }}
                            clickable
                          />
                        ))}
                      </Stack>
                    </Stack>
                  </CardContent>
                </Card>
              ))}
            </Box>
          )}
        </Stack>
      </Container>
    </Page>
  )
}

export default HelixOrgChart
