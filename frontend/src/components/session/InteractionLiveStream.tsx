import React, { FC, useEffect } from 'react'
import Typography from '@mui/material/Typography'
import Progress from '../widgets/Progress'

import WaitingInQueue from './WaitingInQueue'
import LoadingSpinner from '../widgets/LoadingSpinner'
import useLiveInteraction from '../../hooks/useLiveInteraction'

import {
  IInteraction,
} from '../../types'

export const InteractionLiveStream: FC<{
  session_id: string,
  interaction: IInteraction,
  hasSubscription?: boolean,
  onMessageChange?: {
    (message: string): void,
  },
}> = ({
  session_id,
  interaction,
  hasSubscription = false,
  onMessageChange,
}) => {
  const {
    message,
    progress,
    status,
    isStale,
  } = useLiveInteraction({
    session_id,
    interaction,
  })

  const showLoading = !message && progress==0 && !status

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
        showLoading && (
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
      {
        showLoading && isStale && (
          <WaitingInQueue
            hasSubscription={ hasSubscription }
          />
        )
      }
    </>
  )   
}

export default InteractionLiveStream