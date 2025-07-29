import React, { FC, useEffect, useState, useMemo, useCallback, useRef } from 'react'
import Typography from '@mui/material/Typography'
import Progress from '../widgets/Progress'
import WaitingInQueue from './WaitingInQueue'
import LoadingSpinner from '../widgets/LoadingSpinner'
import useLiveInteraction from '../../hooks/useLiveInteraction'
import Markdown from './Markdown'
import useAccount from '../../hooks/useAccount'
import { IServerConfig } from '../../types'
import { TypesInteraction, TypesSession } from '../../api/api'
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
  useInstantScroll?: boolean,
}> = ({
  session_id,
  serverConfig,
  session,
  interaction,
  hasSubscription = false,
  onMessageChange,
  onMessageUpdate,
  onFilterDocument,
  useInstantScroll = false,
}) => {
    const account = useAccount()
    const {
      message,
      progress,
      status,
      isStale,
      stepInfos,
      isComplete,
    } = useLiveInteraction(session_id, interaction)
    
    // Add state to track if we're still in streaming mode or completed
    const [isActivelyStreaming, setIsActivelyStreaming] = useState(true);

    // Memoize values that don't change frequently to prevent unnecessary re-renders
    const showLoading = useMemo(() => 
      !message && progress === 0 && !status && stepInfos.length === 0,
      [message, progress, status, stepInfos.length]
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

    // Only log when component actually needs to re-render due to important prop changes
    const shouldLogRender = useMemo(() => {
      // Create a stable identifier for this render to reduce unnecessary logging
      return {
        isActivelyStreaming,
        isComplete,
        messageLength: message?.length,
        interactionId: interaction?.id,
        state: interaction?.state,
        isSecondOrLaterInteraction: session?.interactions ? session.interactions.indexOf(interaction || {}) > 0 : false
      };
    }, [isActivelyStreaming, isComplete, message?.length, interaction?.id, 
        interaction?.state, session?.interactions]);
    
    if (!serverConfig || !serverConfig.filestore_prefix) return null

    return (
      <>
        {showLoading && <LoadingSpinner />}

        {stepInfos.length > 0 && (
          <ToolStepsWidget 
            steps={toolSteps} 
            isLiveStreaming={isActivelyStreaming}
          />
        )}

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

        {progress > 0 && (
          <Progress
            progress={progress}
          />
        )}

        {status && (
          <Typography variant="caption">{status}</Typography>
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