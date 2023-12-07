import React, { useMemo } from 'react'

import {
  ISession,
  IInteraction,
} from '../types'

import {
  SESSION_TYPE_TEXT,
  SESSION_TYPE_IMAGE,
  SESSION_MODE_INFERENCE,
  SESSION_CREATOR_SYSTEM,
  SESSION_CREATOR_USER,
  INTERACTION_STATE_EDITING,
  TEXT_DATA_PREP_STAGE_NONE,
} from '../types'

import {
  mapFileExtension,
  isImage,
} from '../utils/filestore'

import {
  getTextDataPrepStageIndex,
  getTextDataPrepErrors,
  getTextDataPrepStats,
} from '../utils/session'

export const useInteraction = ({
  session,
  id,
  isLast = false,
}: {
  session: ISession,
  id: string,
  isLast?: boolean,
}) => {

  const interaction = useMemo(() => {
    return session.interactions.find((interaction) => interaction.id == id)
  }, [
    session,
    id,
  ])

  let displayMessage: string = ''
  let imageURLs: string[] = []
  
  let isLoading = isLast && interaction?.creator == SESSION_CREATOR_SYSTEM && !interaction.finished

  const isImageFinetune = interaction?.creator == SESSION_CREATOR_USER && session.type == SESSION_TYPE_IMAGE
  const isTextFinetune = interaction?.creator == SESSION_CREATOR_USER && session.type == SESSION_TYPE_TEXT
  const dataPrepStage = interaction?.data_prep_stage

  const isEditingConversations = interaction?.state == INTERACTION_STATE_EDITING ? true : false
  const hasFineTuned = interaction?.lora_dir ? true : false
  const useErrorText = interaction?.error || ''

  // in this state the last interaction is not yet "finished"
  if(isEditingConversations) {
    isLoading = false
  }

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

  const dataPrepErrors = useMemo(() => {
    if(!interaction) return []
    return getTextDataPrepErrors(interaction)
  }, [
    interaction,
  ])

  const dataPrepStats = useMemo(() => {
    if(!interaction) return []
    return getTextDataPrepStats(interaction)
  }, [
    interaction,
  ])

  return {
    name: useName,
  }
}

export default useInteraction