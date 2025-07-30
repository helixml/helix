import { useMemo } from 'react'

import {
  SESSION_TYPE_TEXT,
} from '../types'

import { TypesInteraction, TypesSession, TypesInteractionState } from '../api/api'

export const useInteraction = ({
  session,
  id,
  isLast = false,
}: {
  session: TypesSession,
  id: string,
  isLast?: boolean,
}) => {

  const interaction = useMemo(() => {
    return session?.interactions?.find((interaction: TypesInteraction) => interaction.id == id)
  }, [
    session,
    id,
  ])

  let displayMessage: string = '' 
  
  let isLoading = isLast && interaction?.state == TypesInteractionState.InteractionStateWaiting

  const useErrorText = interaction?.error || ''  

  if(isLoading) {
    // we don't display the message here - we render a LiveInteraction which handles the websockets
    // without reloading the entire app
  } else {
    if(session.type == SESSION_TYPE_TEXT) {
      displayMessage = interaction?.prompt_message || ''
    } 
  }

  const useSystemName = session.name || 'System'
  const useName = useSystemName

  return {
    name: useName,
  }
}

export default useInteraction