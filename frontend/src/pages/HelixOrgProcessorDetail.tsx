import { FC, useState } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Container from '@mui/material/Container'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import EditIcon from '@mui/icons-material/Edit'

import HelixOrgShell from '../components/helix-org/HelixOrgShell'
import ProcessorConfigDrawer from '../components/helix-org/ProcessorConfigDrawer'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import useHelixOrgBreadcrumbs from '../components/helix-org/useHelixOrgBreadcrumbs'
import useRouter from '../hooks/useRouter'
import { useHelixOrgProcessor } from '../services/helixOrgService'

const HelixOrgProcessorDetail: FC = () => {
  const router = useRouter()
  const processorId = router.params.processor_id as string | undefined
  const { data: processor, isLoading } = useHelixOrgProcessor(processorId)
  const [editing, setEditing] = useState(false)
  const breadcrumbs = useHelixOrgBreadcrumbs({ title: 'Chart', routeName: 'helix_org_chart' })

  return (
    <HelixOrgShell showChat={false} breadcrumbs={breadcrumbs} breadcrumbTitle={processor?.name || processorId || 'Processor'}>
      <Box sx={{ height: '100%', overflow: 'auto' }}>
        <Container maxWidth="md" sx={{ py: 3 }}>
          {isLoading ? <LoadingSpinner /> : !processor ? (
            <Typography color="text.secondary">Processor not found.</Typography>
          ) : (
            <Stack spacing={2}>
              <Stack direction="row" justifyContent="space-between" alignItems="center">
                <Box>
                  <Stack direction="row" spacing={1} alignItems="center">
                    <Typography variant="h5">{processor.name}</Typography>
                    <Chip label={processor.kind} size="small" />
                  </Stack>
                  <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                    {processor.id}
                  </Typography>
                </Box>
                <Button variant="contained" startIcon={<EditIcon />} onClick={() => setEditing(true)}>Edit</Button>
              </Stack>
              <Box>
                <Typography variant="overline" color="text.secondary">Input topic</Typography>
                <Typography sx={{ fontFamily: 'monospace' }}>{processor.input_topic_id}</Typography>
              </Box>
              <Box>
                <Typography variant="overline" color="text.secondary">Outputs</Typography>
                <Stack spacing={1}>
                  {processor.outputs.map((output) => (
                    <Box key={output.topic_id} sx={{ p: 1.5, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                      <Typography variant="body2" sx={{ fontFamily: 'monospace' }}>{output.topic_id}</Typography>
                      {(output.label || output.match) && (
                        <Typography variant="caption" color="text.secondary">
                          {[output.label, output.match].filter(Boolean).join(' · ')}
                        </Typography>
                      )}
                    </Box>
                  ))}
                </Stack>
              </Box>
            </Stack>
          )}
        </Container>
      </Box>
      <ProcessorConfigDrawer open={editing} processor={processor} onClose={() => setEditing(false)} />
    </HelixOrgShell>
  )
}

export default HelixOrgProcessorDetail
