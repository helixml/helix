import React, { FC, useEffect, useState, useMemo, useCallback, useRef } from 'react'
import Typography from '@mui/material/Typography'
import WaitingInQueue from './WaitingInQueue'
import LoadingSpinner from '../widgets/LoadingSpinner'
import useLiveInteraction from '../../hooks/useLiveInteraction'
import Markdown from './Markdown'
import { IServerConfig } from '../../types'
import { TypesInteraction, TypesInteractionState, TypesSession } from '../../api/api'
import ToolStepsWidget from './ToolStepsWidget'

export const InteractionLiveStream: FC<{
  session_id: string,
  interaction: TypesInteraction,
  hasSubscription?: boolean,
  serverConfig?: IServerConfig,
  session: TypesSession,
  onMessageChange?: {
    (message: string): void,
  },
  onMessageUpdate?: () => void,
  onFilterDocument?: (docId: string) => void,
}> = ({
  session_id,
  serverConfig,
  session,
  interaction,
  hasSubscription = false,
  onMessageChange,
  onMessageUpdate,
  onFilterDocument,
}) => {
    const {
      message,
      status,
      isStale,
      stepInfos,
      isComplete,
    } = useLiveInteraction(session_id, interaction)
    
    // Add state to track if we're still in streaming mode or completed
    const [isActivelyStreaming, setIsActivelyStreaming] = useState(true);

    // Memoize values that don't change frequently to prevent unnecessary re-renders
    const showLoading = useMemo(() => 
      !message && interaction.state === TypesInteractionState.InteractionStateWaiting,
      [message, status]
    );

    // Memoize the useClientURL function
    const useClientURL = useCallback((url: string) => {
      if (!url) return '';
      if (!serverConfig) return '';
      return `${serverConfig.filestore_prefix}/${url}?redirect_urls=true`;
    }, [serverConfig]);

    // Transform stepInfos to match ToolStepsWidget format
    const toolSteps = useMemo(() => 
      stepInfos.map((step, index) => ({
        id: `step-${index}`,
        name: step.name,
        icon: step.icon,
        type: step.type,
        message: step.message,
        details: {
          arguments: {}
        },
        created: step.created || '',
      })),
      [stepInfos]
    );

    // Reset streaming state when a new interaction starts or interaction ID changes
    useEffect(() => {
      // Always reset to streaming state when interaction ID changes
      setIsActivelyStreaming(true);
    }, [interaction?.id]);

    // Effect to detect completion from the server (WebSocket)
    useEffect(() => {
      if (isComplete && isActivelyStreaming) {
        setIsActivelyStreaming(false);
      }
    }, [isComplete, isActivelyStreaming]);   

    useEffect(() => {
      if (!message) return
      if (!onMessageChange) return
      onMessageChange(message)
    }, [
      message,
      onMessageChange,
    ])

    useEffect(() => {
      if (!message || !onMessageUpdate) return
      onMessageUpdate()
    }, [message, onMessageUpdate])
    
    if (!serverConfig || !serverConfig.filestore_prefix) return null

    return (
      <>        
        {stepInfos.length > 0 && (
          <ToolStepsWidget 
            steps={toolSteps} 
            isLiveStreaming={isActivelyStreaming}
          />
        )}

        {showLoading && <LoadingSpinner />}

        {message && (
          <div>
            <Markdown
              text={message}
              session={session}
              getFileURL={useClientURL}
              showBlinker={true}
              isStreaming={isActivelyStreaming} // Now reactive to completion state
              onFilterDocument={onFilterDocument}
            />
          </div>
        )}

        {showLoading && isStale && (
          <WaitingInQueue
            hasSubscription={hasSubscription}
          />
        )}
      </>
    )
  }

export default React.memo(InteractionLiveStream)