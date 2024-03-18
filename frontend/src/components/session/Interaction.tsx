import React, { FC } from 'react'
import InteractionContainer from './InteractionContainer'
import InteractionFinetune from './InteractionFinetune'
import InteractionInference from './InteractionInference'
import Box from '@mui/material/Box'

import useTheme from '@mui/material/styles/useTheme'
import useThemeConfig from '../../hooks/useThemeConfig'

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

  const useSystemName = session.name || 'System'
  const useName = interaction?.creator == SESSION_CREATOR_SYSTEM ? useSystemName : interaction?.creator
  const theme = useTheme()
  const themeConfig = useThemeConfig()

  if(!serverConfig || !serverConfig.filestore_prefix) return null

  return (
    <Box
      sx={{
        backgroundColor: interaction?.creator == SESSION_CREATOR_SYSTEM ? (theme.palette.mode === 'dark' ? themeConfig.darkPanel : themeConfig.lightPanel) : 'none',
        p: 2,
        borderRadius: '0.5rem',
      }}
    >
      <InteractionContainer
        name={ useName }
        buttons={ headerButtons }
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