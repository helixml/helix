import React, { FC } from 'react'
import InteractionContainer from './InteractionContainer'
import InteractionFinetune from './InteractionFinetune'
import InteractionInference from './InteractionInference'

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
  ICloneTextMode,
} from '../../types'

import {
  isImage,
} from '../../utils/filestore'

export const Interaction: FC<{
  serverConfig: IServerConfig,
  interaction: IInteraction,
  session: ISession,
  showFinetuning?: boolean,
  retryFinetuneErrors?: () => void,
  onReloadSession?: () => void,
  onClone?: (mode: ICloneTextMode, interactionID: string) => Promise<boolean>,
  onRestart?: () => void,
}> = ({
  serverConfig,
  interaction,
  session,
  showFinetuning = true,
  retryFinetuneErrors,
  onReloadSession,
  onClone,
  onRestart,
  children,
}) => {
  let displayMessage: string = ''
  let imageURLs: string[] = []

  let isLoading = interaction?.creator == SESSION_CREATOR_SYSTEM && !interaction.finished

  if(isLoading) {
    // we don't display the message here - we render a LiveInteraction which handles the websockets
    // without reloading the entire app
  } else {
    if(session.type == SESSION_TYPE_TEXT) {
      if(!interaction?.lora_dir) {
        displayMessage = interaction?.message || ''
      }
    } else if(session.type == SESSION_TYPE_IMAGE) {
      if(interaction?.creator == SESSION_CREATOR_USER) {
        displayMessage = interaction.message || ''
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

  if(!serverConfig || !serverConfig.filestore_prefix) return null

  return (
    <InteractionContainer
      name={ useName }
    >
      {
        showFinetuning && (
          <InteractionFinetune
            serverConfig={ serverConfig }
            interaction={ interaction }
            session={ session }
            retryFinetuneErrors={ retryFinetuneErrors }
            onReloadSession={ onReloadSession }
            onClone={ onClone }
          />
        )
      }
      
      <InteractionInference
        serverConfig={ serverConfig }
        imageURLs={ imageURLs }
        message={ displayMessage }
        error={ interaction?.error }
        onRestart={ onRestart }
      />

      {
        children
      }
    </InteractionContainer>  
  )   
}

export default Interaction