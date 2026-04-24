import React, { FC } from 'react'
import { useRoute } from 'react-router5'
import { Box, CircularProgress } from '@mui/material'
import SpecTaskDetailContent from '../components/tasks/SpecTaskDetailContent'
import { useSpecTask } from '../services/specTaskService'

const EmbedTaskPage: FC = () => {
  const { route } = useRoute()
  const taskId = route.params.taskId as string
  const { isLoading } = useSpecTask(taskId, { enabled: !!taskId })

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', height: '100vh' }}>
        <CircularProgress />
      </Box>
    )
  }

  return (
    <Box sx={{ height: '100vh', overflow: 'hidden' }}>
      <SpecTaskDetailContent taskId={taskId} />
    </Box>
  )
}

export default EmbedTaskPage
