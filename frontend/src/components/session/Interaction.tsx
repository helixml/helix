import React, { FC } from 'react'
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
  SESSION_CREATOR_SYSTEM,
  SESSION_CREATOR_USER,
  ISession,
  IInteraction,
  IServerConfig,
  ICloneInteractionMode,
} from '../../types'

import {
  isImage,
} from '../../utils/filestore'

export const Interaction: FC<{
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
}> = ({
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
  let displayMessage: string = ''
  let imageURLs: string[] = []

  let isLoading = interaction?.creator == SESSION_CREATOR_SYSTEM && !interaction.finished

  let useMessageText = ''

  if(interaction) {
    useMessageText = interaction.display_message || interaction.message || ''
  }

  if(isLoading) {
    // we don't display the message here - we render a LiveInteraction which handles the websockets
    // without reloading the entire app
  } else {
    if(session.type == SESSION_TYPE_TEXT) {
      if(!interaction?.lora_dir) {
        // If single message is shown, display it
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

  const isSystem = interaction?.creator == SESSION_CREATOR_SYSTEM
  const useName = isSystem ? 'Helix System' : account.user?.name || 'User'
  const useBadge = isSystem ? 'AI' : ''

  if(!serverConfig || !serverConfig.filestore_prefix) return null

  return (
    <Box
      sx={{
        mb: 1,
      }}
    >
      <InteractionContainer
        name={ useName }
        badge={ useBadge }
        buttons={ headerButtons }
        background={ isSystem }
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
            isFromSystem={interaction?.creator == SESSION_CREATOR_SYSTEM}
          />
          
          {
            children
          }
      </InteractionContainer>
    </Box>
  )   
}

export default Interaction