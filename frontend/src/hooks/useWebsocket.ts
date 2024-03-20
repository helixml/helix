import React, { useEffect } from 'react'
import ReconnectingWebSocket from 'reconnecting-websocket'
import useAccount from '../hooks/useAccount'

import {
  IWebsocketEvent,
} from '../types'

export const useWebsocket = (
  session_id: string,
  handler: {
    (ev: IWebsocketEvent): void,
  },
) => {
  const account = useAccount()

  useEffect(() => {
    if(!account.token) return
    if(!session_id) return
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsHostname = window.location.hostname
    const url = `${wsProtocol}//${wsHostname}/api/v1/ws/user?access_token=${account.token}&session_id=${session_id}`
    const rws = new ReconnectingWebSocket(url)
    const messageHandler = (event: MessageEvent<any>) => {
      const parsedData = JSON.parse(event.data) as IWebsocketEvent
      if(parsedData.session_id != session_id) return
      handler(parsedData)
    }
    rws.addEventListener('message', messageHandler)
    return () => {
      rws.removeEventListener('message', messageHandler)
      rws.close()
    }
  }, [
    account.token,
    session_id,
  ])
}

export default useWebsocket