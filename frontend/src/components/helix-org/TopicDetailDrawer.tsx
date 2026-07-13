import { FC } from 'react'
import Button from '@mui/material/Button'
import Chip from '@mui/material/Chip'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'

import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import LoadingSpinner from '../widgets/LoadingSpinner'
import CopyButtonWithCheck from '../session/CopyButtonWithCheck'
import { useHelixOrgTopic, useUpdateHelixOrgTopic } from '../../services/helixOrgService'
import { TopicConfigSection } from '../../pages/HelixOrgTopicDetail'
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
          <Stack direction="row" spacing={1} alignItems="center">
            <Typography variant="body2" sx={{ fontFamily: 'monospace', overflowWrap: 'anywhere' }}>
              {topic.id}
            </Typography>
            <CopyButtonWithCheck text={topic.id} />
            <Chip label={topic.kind} size="small" />
          </Stack>
          <Typography variant="body2" color="text.secondary">
            {consumerCount ?? topic.subscribers?.length ?? 0} subscriber{(consumerCount ?? topic.subscribers?.length) === 1 ? '' : 's'}
          </Typography>
          <TopicConfigSection
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
          />
        </Stack>
      )}
    </HelixOrgSideDrawer>
  )
}

export default TopicDetailDrawer
