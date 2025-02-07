import React, { FC, useMemo } from 'react'
import InteractionContainer from './InteractionContainer'
import InteractionFinetune from './InteractionFinetune'
import InteractionInference from './InteractionInference'
import Box from '@mui/material/Box'

import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'
import useAccount from '../../hooks/useAccount'

import {
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  SESSION_MODE_INFERENCE,
  SESSION_MODE_FINETUNE,
  SESSION_CREATOR_ASSISTANT,
  SESSION_CREATOR_USER,
  ISession,
  IInteraction,
  IServerConfig,
  ICloneInteractionMode,
} from '../../types'

import {
  isImage,
} from '../../utils/filestore'

// Prop comparison function for React.memo
const areEqual = (prevProps: InteractionProps, nextProps: InteractionProps) => {
  // Compare serverConfig
  if (prevProps.serverConfig?.filestore_prefix !== nextProps.serverConfig?.filestore_prefix) {
    return false
  }

  // Compare interaction
  if (prevProps.interaction?.id !== nextProps.interaction?.id ||
      prevProps.interaction?.finished !== nextProps.interaction?.finished ||
      prevProps.interaction?.message !== nextProps.interaction?.message ||
      prevProps.interaction?.display_message !== nextProps.interaction?.display_message ||
      prevProps.interaction?.error !== nextProps.interaction?.error ||
      prevProps.interaction?.state !== nextProps.interaction?.state) {
    return false
  }

  // Compare session
  if (prevProps.session?.id !== nextProps.session?.id ||
      prevProps.session?.type !== nextProps.session?.type ||
      prevProps.session?.mode !== nextProps.session?.mode ||
      prevProps.session?.config?.shared !== nextProps.session?.config?.shared) {
    return false
  }

  // Compare other props
  if (prevProps.highlightAllFiles !== nextProps.highlightAllFiles ||
      prevProps.showFinetuning !== nextProps.showFinetuning) {
    return false
  }

  // Compare function references
  if (prevProps.retryFinetuneErrors !== nextProps.retryFinetuneErrors ||
      prevProps.onReloadSession !== nextProps.onReloadSession ||
      prevProps.onClone !== nextProps.onClone ||
      prevProps.onAddDocuments !== nextProps.onAddDocuments ||
      prevProps.onRestart !== nextProps.onRestart) {
    return false
  }

  return true
}

interface InteractionProps {
  serverConfig: IServerConfig,
  interaction: IInteraction,
  session: ISession,
  showFinetuning?: boolean,
  highlightAllFiles?: boolean,
  headerButtons?: React.ReactNode,
  retryFinetuneErrors?: () => void,
  onReloadSession?: () => void,
  onClone?: (mode: ICloneInteractionMode, interactionID: string) => Promise<boolean>,
  onAddDocuments?: () => void,
  onRestart?: () => void,
  children?: React.ReactNode,
}

export const Interaction: FC<InteractionProps> = ({
  serverConfig,
  interaction,
  session,
  highlightAllFiles = false,
  showFinetuning = true,
  headerButtons,
  retryFinetuneErrors,
  onReloadSession,
  onClone,
  onAddDocuments,
  onRestart,
  children,
}) => {
  const account = useAccount()

  // Memoize computed values
  const displayData = useMemo(() => {
    let displayMessage: string = ''
    let imageURLs: string[] = []
    let isLoading = interaction?.creator == SESSION_CREATOR_ASSISTANT && !interaction.finished
    let useMessageText = interaction ? (interaction.display_message || interaction.message || '') : ''

    if(!isLoading) {
      if(session.type == SESSION_TYPE_TEXT) {
        if(!interaction?.lora_dir) {
          if (interaction?.message) {
            displayMessage = useMessageText
          } else {
            displayMessage = interaction.status || ''
          }        
        }
      } else if(session.type == SESSION_TYPE_IMAGE) {
        if(interaction?.creator == SESSION_CREATOR_USER) {
          displayMessage = useMessageText || ''
        }
        else {
          if(session.mode == SESSION_MODE_INFERENCE && interaction?.files && interaction?.files.length > 0) {
            imageURLs = interaction.files.filter(isImage)
          }
        }
      }
    }

    return {
      displayMessage,
      imageURLs,
      isLoading
    }
  }, [interaction, session])

  const { displayMessage, imageURLs, isLoading } = displayData

  const isAssistant = interaction?.creator == SESSION_CREATOR_ASSISTANT
  const useName = isAssistant ? 'Helix' : account.user?.name || 'User'
  const useBadge = isAssistant ? 'AI' : ''  

  if(!serverConfig || !serverConfig.filestore_prefix) return null

  return (
    <Box
      sx={{
        mb: 0.5,
      }}
    >
      <InteractionContainer
        name={ useName }
        badge={ useBadge }
        buttons={ headerButtons }
        background={ isAssistant }
      >
          {
            showFinetuning && (
              <InteractionFinetune
                serverConfig={ serverConfig }
                interaction={ interaction }
                session={ session }
                highlightAllFiles={ highlightAllFiles }
                retryFinetuneErrors={ retryFinetuneErrors }
                onReloadSession={ onReloadSession }
                onClone={ onClone }
                onAddDocuments={ onAddDocuments }
              />
            )
          }

          <InteractionInference
            serverConfig={ serverConfig }
            session={ session }
            imageURLs={ imageURLs }
            message={ displayMessage }
            error={ interaction?.error }
            isShared={ session.config.shared }
            onRestart={ onRestart }
            upgrade={ interaction.data_prep_limited }
            isFromAssistant={interaction?.creator == SESSION_CREATOR_ASSISTANT}
          />
          
          {
            children
          }
      </InteractionContainer>
    </Box>
  )   
}

export default React.memo(Interaction, areEqual)