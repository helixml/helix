import React, { FC, useEffect } from 'react'
import Typography from '@mui/material/Typography'
import Progress from '../widgets/Progress'
import WaitingInQueue from './WaitingInQueue'
import LoadingSpinner from '../widgets/LoadingSpinner'
import useLiveInteraction from '../../hooks/useLiveInteraction'
import Markdown from './Markdown'
import useAccount from '../../hooks/useAccount'
import { IInteraction, ISession, IServerConfig } from '../../types'
import { replaceMessageText } from '../../utils/session'
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

const Orb = styled.div<{ isPulsating: boolean; colorIndex: number }>`
  width: 100%;
  height: 100%;
  border-radius: 50%;
  background: radial-gradient(circle at 30% 30%, ${props => orbColors[props.colorIndex]}, #000);
  box-shadow: 0 0 10px ${props => orbColors[props.colorIndex]};
  cursor: pointer;
  animation: ${props => props.isPulsating ? pulse : 'none'} 2s infinite;
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
  onStreamingComplete?: () => void,
}> = ({
  session_id,
  serverConfig,
  session,
  interaction,
  hasSubscription = false,
  onMessageChange,
  onMessageUpdate,
  onStreamingComplete,
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

  const showLoading = !message && progress === 0 && !status && stepInfos.length === 0

  useEffect(() => {
    if(!message) return
    if(!onMessageChange) return
    onMessageChange(message)
  }, [
    message,
    onMessageChange,
  ])

  useEffect(() => {
    if (!message || !onMessageUpdate) return
    onMessageUpdate()
  }, [message, onMessageUpdate])

  useEffect(() => {
    if (!isComplete || !onStreamingComplete) return
    const timer = setTimeout(() => {
      onStreamingComplete()
    }, 100)
    return () => clearTimeout(timer)
  }, [isComplete, onStreamingComplete])

  const getFileURL = (url: string) => {
    if(!serverConfig) return ''
    return `${serverConfig.filestore_prefix}/${url}?access_token=${account.tokenUrlEscaped}&redirect_urls=true`
  }

  if(!serverConfig || !serverConfig.filestore_prefix) return null

  const blinker = `<span class="blinker-class">┃</span>`
  
  return (
    <>
      {showLoading && <LoadingSpinner />}
      
      {stepInfos.length > 0 && (
        <OrbContainer>
          {stepInfos.map((stepInfo, index) => (
            <OrbWrapper key={index}>
              <Orb 
                isPulsating={index === stepInfos.length - 1 && !message} 
                colorIndex={index % orbColors.length}
              />
              <OrbTooltip>
                <strong>{stepInfo.type}: {stepInfo.name}</strong><br/>
                {stepInfo.message}
              </OrbTooltip>
            </OrbWrapper>
          ))}
        </OrbContainer>
      )}
      
      {message && (
        <div>
          <Markdown
            text={(() => {
              console.debug(`InteractionLiveStream: Replacing message text for session ${session.id}`);
              console.debug(`InteractionLiveStream: Message before replacement: "${message.substring(0, 100)}${message.length > 100 ? '...' : ''}"`);
              
              // First process the message with replaceMessageText
              let replacedText = replaceMessageText(message, session, getFileURL);
              
              // Check if the message contains RAG citation data
              // Match both escaped and unescaped versions of the container
              const hasCitation = 
                /<div class="rag-citations-container">/.test(replacedText) || 
                /&lt;div class="rag-citations-container"&gt;/.test(replacedText);
              
              // Only add the blinker if there's no citation block, or add it before the citation
              if (hasCitation) {
                // Insert the blinker before the citation container, handling both escaped and unescaped versions
                if (replacedText.includes('<div class="rag-citations-container">')) {
                  replacedText = replacedText.replace(
                    /<div class="rag-citations-container">/,
                    `<span class="blinker-class">┃</span><div class="rag-citations-container">`
                  );
                } else {
                  // Try with escaped version
                  replacedText = replacedText.replace(
                    /&lt;div class="rag-citations-container"&gt;/,
                    `<span class="blinker-class">┃</span>&lt;div class="rag-citations-container"&gt;`
                  );
                }
              } else {
                // No citation block, just append the blinker
                replacedText += `<span class="blinker-class">┃</span>`;
              }
              
              console.debug(`InteractionLiveStream: Message after replacement: "${replacedText.substring(0, 100)}${replacedText.length > 100 ? '...' : ''}"`);
              return replacedText;
            })()}
          />
        </div>
      )}
      
      {progress > 0 && (
        <Progress
          progress={ progress }
        />
      )}
      
      {status && (
        <Typography variant="caption">{ status }</Typography>
      )}
      
      {showLoading && isStale && (
        <WaitingInQueue
          hasSubscription={ hasSubscription }
        />
      )}
    </>
  )   
}

export default InteractionLiveStream