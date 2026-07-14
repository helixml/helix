import { FC } from 'react'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import HubOutlinedIcon from '@mui/icons-material/HubOutlined'

import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import LoadingSpinner from '../widgets/LoadingSpinner'
import CopyButtonWithCheck from '../session/CopyButtonWithCheck'
import { useHelixOrgTopic, useUpdateHelixOrgTopic } from '../../services/helixOrgService'
import { ClearTopicMessagesButton, TopicConfigSection } from '../../pages/HelixOrgTopicDetail'
import HelixOrgOverviewCard from './HelixOrgOverviewCard'
import HelixOrgSideDrawer from './HelixOrgSideDrawer'

type TopicDetailDrawerProps = {
  topicId?: string
  consumerCount?: number
  onClose: () => void
}

const TopicDetailDrawer: FC<TopicDetailDrawerProps> = ({ topicId, consumerCount, onClose }) => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const { data: topic, isLoading } = useHelixOrgTopic(topicId)
  const updateTopic = useUpdateHelixOrgTopic()

  return (
    <HelixOrgSideDrawer
      open={Boolean(topicId)}
      onClose={onClose}
      title={topic?.name || topicId || 'Topic'}
      width={560}
      headerAction={topicId ? (
        <Button
          size="small"
          onClick={() => router.navigate('helix_org_topic_detail', {
            org_id: router.params.org_id,
            topic_id: topicId,
          })}
        >
          Details
        </Button>
      ) : undefined}
    >
      {isLoading ? <LoadingSpinner /> : !topic ? (
        <Typography color="text.secondary">Topic not found.</Typography>
      ) : (
        <Stack spacing={2}>
          <HelixOrgOverviewCard
            title={topic.name || topic.id || 'Topic'}
            id={topic.id}
            idAction={<CopyButtonWithCheck text={topic.id || ''} />}
            icon={<HubOutlinedIcon sx={{ fontSize: 20 }} />}
            status={<Chip label={topic.kind} size="small" sx={{ color: 'common.white', backgroundColor: 'rgba(255,255,255,0.11)', border: '1px solid rgba(255,255,255,0.22)' }} />}
          >
            <Chip
              label={`${consumerCount ?? topic.subscribers?.length ?? 0} subscriber${(consumerCount ?? topic.subscribers?.length) === 1 ? '' : 's'}`}
              size="small"
              sx={{ color: 'common.white', backgroundColor: 'rgba(255,255,255,0.11)' }}
            />
          </HelixOrgOverviewCard>
          <TopicConfigSection
            key={topic.id}
            topic={topic}
            saving={updateTopic.isPending}
            onSave={async (payload) => {
              try {
                await updateTopic.mutateAsync({ topicId: topic.id, payload })
                snackbar.success('topic updated')
                return true
              } catch (e: any) {
                snackbar.error(e?.response?.data?.error || e?.message || 'update failed')
                return false
              }
            }}
            onCancel={onClose}
          />
          <Stack direction="row" justifyContent="flex-end">
            <ClearTopicMessagesButton topic={topic} />
          </Stack>
        </Stack>
      )}
    </HelixOrgSideDrawer>
  )
}

export default TopicDetailDrawer
