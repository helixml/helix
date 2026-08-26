import { FC } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { ExternalLink, RadioTower } from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import { useTriggerKind } from '../../services/triggerKindService'
import { useTrigger, useTriggerEvents } from '../../services/triggerService'
import LoadingSpinner from '../widgets/LoadingSpinner'
import HelixOrgSideDrawer from './HelixOrgSideDrawer'
import HelixOrgOverviewCard from './HelixOrgOverviewCard'
import TriggerConfig from './trigger/TriggerConfig'

interface Props {
  triggerID?: string
  agentCount?: number
  onClose: () => void
}

const TriggerDetailDrawer: FC<Props> = ({ triggerID, agentCount = 0, onClose }) => {
  const router = useRouter()
  const orgID = router.params.org_id as string
  const { data: trigger, isLoading } = useTrigger(triggerID)
  const { data: history } = useTriggerEvents(triggerID)
  const kindDescriptor = useTriggerKind(trigger?.kind)

  return (
    <HelixOrgSideDrawer
      open={!!triggerID}
      onClose={onClose}
      title={trigger?.name || triggerID || 'Trigger'}
      width={480}
      headerAction={triggerID ? (
        <Button
          size="small"
          startIcon={<ExternalLink size={16} />}
          onClick={() => router.navigate('helix_org_trigger_detail', { org_id: orgID, trigger_id: triggerID })}
        >
          Open
        </Button>
      ) : undefined}
    >
      {isLoading ? <LoadingSpinner /> : !trigger ? (
        <Typography color="text.secondary">Trigger not found.</Typography>
      ) : (
        <Stack spacing={2}>
          <HelixOrgOverviewCard
            title={kindDescriptor?.label ?? trigger.kind ?? 'Trigger'}
            id={trigger.id}
            icon={<RadioTower size={20} />}
          >
            <Typography variant="body2" sx={{ width: '100%', color: 'rgba(255,255,255,0.82)' }}>
              {trigger.description || 'No description'}
            </Typography>
            <Typography variant="caption">{agentCount} agent{agentCount === 1 ? '' : 's'}</Typography>
            <Typography variant="caption">{history?.total ?? 0} event{history?.total === 1 ? '' : 's'}</Typography>
          </HelixOrgOverviewCard>
          <TriggerConfig trigger={trigger} density="compact" mode="read" orgID={orgID} />
          <Box>
            <Typography variant="subtitle2" sx={{ mb: 1 }}>Recent events</Typography>
            {!history?.events?.length ? (
              <Typography variant="body2" color="text.secondary">No events received yet.</Typography>
            ) : (
              <Stack spacing={1}>
                {history.events.slice(0, 5).map((event) => (
                  <Box key={event.id} sx={{ p: 1.25, border: '1px solid', borderColor: 'divider', borderRadius: 1 }}>
                    <Typography variant="caption" color="text.secondary">
                      {event.created_at ? new Date(event.created_at).toLocaleString() : ''}
                    </Typography>
                    <Typography variant="body2" component="pre" sx={{ m: 0, mt: 0.5, whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' }}>
                      {event.body}
                    </Typography>
                  </Box>
                ))}
              </Stack>
            )}
          </Box>
        </Stack>
      )}
    </HelixOrgSideDrawer>
  )
}

export default TriggerDetailDrawer
