import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Progress from '../widgets/Progress'

import LoadingSpinner from '../widgets/LoadingSpinner'
import useLiveInteraction from '../../hooks/useLiveInteraction'

export const LiveInteraction: FC<{
  session_id?: string,
}> = ({
  session_id,
}) => {

  const {
    message,
    progress,
    status,
  } = useLiveInteraction(session_id || '')
  
  return (
    <>
      {
        !message && progress==0 && !status && (
          <LoadingSpinner />
        )
      }
      {
        message && (
          <Typography>{ message }</Typography>
        )
      }
      {
        progress > 0 && (
          <Progress
            progress={ progress }
          />
        )
      }
      {
        status && (
          <Typography variant="caption">{ status }</Typography>
        )
      }
    </>
  )   
}

export default LiveInteraction