import { FC } from 'react'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import Page from '../components/system/Page'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useRouter from '../hooks/useRouter'
import { ChartNode, useHelixOrgChart } from '../services/helixOrgService'

// HelixOrgChart renders the position-tree returned by GET
// /api/v1/org/chart. Each Position card shows its role + assigned
// Worker badges and recurses into children. Worker badges navigate to
// the detail page so the org chart doubles as an entry point into
// the worker editor.
const PositionCard: FC<{ node: ChartNode }> = ({ node }) => {
  const router = useRouter()
  return (
    <Box
      sx={{
        border: '1px solid rgba(255,255,255,0.12)',
        borderRadius: 1,
        p: 2,
        backgroundColor: 'rgba(255,255,255,0.02)',
        minWidth: 220,
      }}
    >
      <Typography variant="caption" sx={{ color: 'text.secondary', fontFamily: 'monospace' }}>
        {node.position_id}
      </Typography>
      <Typography variant="body2" sx={{ mt: 0.5, fontWeight: 600 }}>
        {node.role_id}
      </Typography>
      {node.workers && node.workers.length > 0 && (
        <Stack direction="row" spacing={1} sx={{ mt: 1, flexWrap: 'wrap', gap: 1 }}>
          {node.workers.map((w) => (
            <Chip
              key={w.id}
              label={`${w.id} · ${w.kind}`}
              size="small"
              sx={{ fontFamily: 'monospace', fontSize: '0.7rem', cursor: 'pointer' }}
              onClick={(e) => {
                e.stopPropagation()
                router.navigate('helix_org_worker_detail', { worker_id: w.id })
              }}
            />
          ))}
        </Stack>
      )}
    </Box>
  )
}

const ChartTree: FC<{ nodes: ChartNode[] }> = ({ nodes }) => (
  <Stack spacing={2}>
    {nodes.map((node) => (
      <Box key={node.position_id}>
        <PositionCard node={node} />
        {node.children && node.children.length > 0 && (
          <Box sx={{ pl: 4, mt: 2, borderLeft: '1px dashed rgba(255,255,255,0.12)' }}>
            <ChartTree nodes={node.children} />
          </Box>
        )}
      </Box>
    ))}
  </Stack>
)

const HelixOrgChart: FC = () => {
  const { data, isLoading } = useHelixOrgChart()
  const roots = data?.roots ?? []

  return (
    <Page breadcrumbTitle="Org Chart" breadcrumbParent={{ title: 'Helix Org' }}>
      <Container maxWidth="lg" sx={{ py: 3 }}>
        <Stack spacing={2}>
          <Box>
            <Typography variant="h5" sx={{ mb: 1 }}>Org Chart</Typography>
            <Typography variant="body2" color="text.secondary">
              Positions form the tree; Workers attach as badges. Click a Worker to edit its role/identity.
            </Typography>
          </Box>

          {isLoading ? (
            <LoadingSpinner />
          ) : roots.length === 0 ? (
            <Box sx={{ textAlign: 'center', py: 6 }}>
              <Typography variant="body1" color="text.secondary">
                No positions yet. Open the chat and try “hire me a CEO”.
              </Typography>
            </Box>
          ) : (
            <ChartTree nodes={roots} />
          )}
        </Stack>
      </Container>
    </Page>
  )
}

export default HelixOrgChart
