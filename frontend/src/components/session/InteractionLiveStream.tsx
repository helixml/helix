import React, { FC, useEffect } from 'react'
import Typography from '@mui/material/Typography'
import Progress from '../widgets/Progress'

import LoadingSpinner from '../widgets/LoadingSpinner'
import useLiveInteraction from '../../hooks/useLiveInteraction'

import {
  IInteraction,
} from '../../types'

export const InteractionLiveStream: FC<{
  session_id: string,
  interaction: IInteraction,
  onMessageChange?: {
    (message: string): void,
  },
}> = ({
  session_id,
  interaction,
  onMessageChange,
}) => {
  const {
    message,
    progress,
    status,
  } = useLiveInteraction({
    session_id,
    interaction,
  })

  useEffect(() => {
    if(!message) return
    if(!onMessageChange) return
    onMessageChange(message)
  }, [
    message,
  ])
  
  return (
    <>
      {
        !message && progress==0 && !status && (
          <LoadingSpinner />
        )
      }
      {
        message && (
            <Typography dangerouslySetInnerHTML={{__html: message.trim().replace(/</g, '&lt;').replace(/\n/g, '<br/>')}}></Typography>
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

export default InteractionLiveStream