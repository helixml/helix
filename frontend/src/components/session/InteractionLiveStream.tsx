import React, { FC, useEffect } from 'react'
import Typography from '@mui/material/Typography'
import Progress from '../widgets/Progress'

import WaitingInQueue from './WaitingInQueue'
import LoadingSpinner from '../widgets/LoadingSpinner'
import useLiveInteraction from '../../hooks/useLiveInteraction'

import useAccount from '../../hooks/useAccount'

import {
  IInteraction,
  ISession,
  IServerConfig,
} from '../../types'

import {
  replaceMessageText,
} from '../../utils/session'

export const InteractionLiveStream: FC<{
  session_id: string,
  interaction: IInteraction,
  hasSubscription?: boolean,
  serverConfig?: IServerConfig,
  session: ISession,
  onMessageChange?: {
    (message: string): void,
  },
}> = ({
  session_id,
  serverConfig,
  session,
  interaction,
  hasSubscription = false,
  onMessageChange,
}) => {
  const account = useAccount()
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

  const getFileURL = (url: string) => {
    if(!serverConfig) return ''
    return `${serverConfig.filestore_prefix}/${url}?access_token=${account.token}&redirect_urls=true`
  }

  if(!serverConfig || !serverConfig.filestore_prefix) return null
  
  return (
    <>
      {
        showLoading && (
          <LoadingSpinner />
        )
      }
      {
        message && (
          <div>
            <Typography dangerouslySetInnerHTML={{__html: replaceMessageText(message, session, getFileURL) + `
              <style>
                .blinker-class {
                  animation: blink 1s linear infinite;
                }

                @keyframes blink {
                  25% {
                    opacity: 0.5;
                  }
                  50% {
                    opacity: 0;
                  }
                  75% {
                    opacity: 0.5;
                  }
                }
              </style><span style="color: yellow; font-weight:bold;" class="blinker-class">┃</span>`}}></Typography>
          </div>
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