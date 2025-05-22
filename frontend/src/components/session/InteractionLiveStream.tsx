import React, { FC, useEffect, useState, useMemo, useCallback, useRef } from 'react'
import Typography from '@mui/material/Typography'
import Progress from '../widgets/Progress'
import WaitingInQueue from './WaitingInQueue'
import LoadingSpinner from '../widgets/LoadingSpinner'
import useLiveInteraction from '../../hooks/useLiveInteraction'
import Markdown from './Markdown'
import useAccount from '../../hooks/useAccount'
import { IInteraction, ISession, IServerConfig } from '../../types'
import styled, { keyframes } from 'styled-components'

const pulse = keyframes`
  0% {
    transform: scale(1);
    opacity: 0.7;
  }
  50% {
    transform: scale(1.05);
    opacity: 1;
  }
  100% {
    transform: scale(1);
    opacity: 0.7;
  }
`

const OrbContainer = styled.div`
  display: flex;
  gap: 10px;
  margin-bottom: 15px;
  padding-top: 10px;
`

const OrbWrapper = styled.div`
  position: relative;
  width: 20px;
  height: 20px;
`

const orbColors = ['#FFBF00', '#00FF00', '#0000FF', '#800080']

const Orb = styled.div<{ $isPulsating: boolean; $colorIndex: number }>`
  width: 100%;
  height: 100%;
  border-radius: 50%;
  background: radial-gradient(circle at 30% 30%, ${props => orbColors[props.$colorIndex]}, #000);
  box-shadow: 0 0 10px ${props => orbColors[props.$colorIndex]};
  cursor: pointer;
  animation: ${props => props.$isPulsating ? pulse : 'none'} 2s infinite;
`

const OrbTooltip = styled.div`
  position: absolute;
  top: 50%;
  left: 100%;
  transform: translateY(-50%);
  background-color: rgba(0, 0, 0, 0.8);
  color: white;
  padding: 5px 10px;
  border-radius: 4px;
  font-size: 12px;
  white-space: nowrap;
  opacity: 0;
  transition: opacity 0.3s;
  pointer-events: none;
  z-index: 1000;
  margin-left: 10px;

  ${OrbWrapper}:hover & {
    opacity: 1;
  }
`

export const InteractionLiveStream: FC<{
  session_id: string,
  interaction: IInteraction,
  hasSubscription?: boolean,
  serverConfig?: IServerConfig,
  session: ISession,
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
    
    // Track previous message length to detect when streaming stops
    const [prevMessageLength, setPrevMessageLength] = useState(0);
    // Replace the state with a ref since we don't need to re-render when the timer changes
    const noChangeTimerRef = useRef<NodeJS.Timeout | null>(null);

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

    // Reset streaming state when a new interaction starts or interaction ID changes
    useEffect(() => {
      // Always reset to streaming state when interaction ID changes
      setIsActivelyStreaming(true);
      setPrevMessageLength(0);
      if (noChangeTimerRef.current) {
        clearTimeout(noChangeTimerRef.current);
        noChangeTimerRef.current = null;
      }
    }, [interaction?.id]);

    // Effect to detect completion from the server (WebSocket)
    useEffect(() => {
      if (isComplete && isActivelyStreaming) {
        setIsActivelyStreaming(false);
      }
    }, [isComplete, isActivelyStreaming]);

    // Safety mechanism to detect when streaming has stopped by monitoring message length
    useEffect(() => {
      // Only run this when we have a message and are streaming
      if (!message || !isActivelyStreaming) return;

      const currentLength = message?.length || 0;
      
      // If message length hasn't changed in 1.5 seconds, consider streaming complete
      if (currentLength > 0 && currentLength === prevMessageLength) {
        // Clear any existing timer
        if (noChangeTimerRef.current) {
          clearTimeout(noChangeTimerRef.current);
        }
        
        // Set a new timer
        const timer = setTimeout(() => {
          setIsActivelyStreaming(false);
        }, 15000);
        
        noChangeTimerRef.current = timer;
      } else {
        // Message length changed, update the previous length
        setPrevMessageLength(currentLength);
        
        // Clear any existing timer
        if (noChangeTimerRef.current) {
          clearTimeout(noChangeTimerRef.current);
          noChangeTimerRef.current = null;
        }
      }
      
      // Cleanup timer on unmount
      return () => {
        if (noChangeTimerRef.current) {
          clearTimeout(noChangeTimerRef.current);
        }
      };
    }, [message, isActivelyStreaming, prevMessageLength]);

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

    // Add a cleanup effect for component unmount - use the ref here
    useEffect(() => {
      return () => {
        // Clean up all timers and state updates when component unmounts
        if (noChangeTimerRef.current) {
          clearTimeout(noChangeTimerRef.current);
        }
      };
    }, []);

    // Only log when component actually needs to re-render due to important prop changes
    const shouldLogRender = useMemo(() => {
      // Create a stable identifier for this render to reduce unnecessary logging
      return {
        isActivelyStreaming,
        isComplete,
        messageLength: message?.length,
        interactionId: interaction?.id,
        state: interaction?.state,
        finished: interaction?.finished,
        isSecondOrLaterInteraction: session?.interactions?.indexOf(interaction) > 0
      };
    }, [isActivelyStreaming, isComplete, message?.length, interaction?.id, 
        interaction?.state, interaction?.finished, session?.interactions]);
    
    if (!serverConfig || !serverConfig.filestore_prefix) return null

    return (
      <>
        {showLoading && <LoadingSpinner />}

        {stepInfos.length > 0 && (
          <OrbContainer>
            {stepInfos.map((stepInfo, index) => (
              <OrbWrapper key={index}>
                <Orb
                  $isPulsating={index === stepInfos.length - 1 && !message}
                  $colorIndex={index % orbColors.length}
                />
                <OrbTooltip>
                  <strong>{stepInfo.type}: {stepInfo.name}</strong><br />
                  {stepInfo.message}
                </OrbTooltip>
              </OrbWrapper>
            ))}
          </OrbContainer>
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