import React, { FC } from 'react'
import Typography from '@mui/material/Typography'
import Progress from '../widgets/Progress'

import LoadingSpinner from '../widgets/LoadingSpinner'
import useLiveInteraction from '../../hooks/useLiveInteraction'

import {
  IInteraction,
} from '../../types'

export const LiveInteraction: FC<{
  session_id: string,
  interaction: IInteraction,
}> = ({
  session_id,
  interaction,
}) => {

  const {
    message,
    progress,
    status,
  } = useLiveInteraction({
    session_id,
    interaction,
  })
  
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