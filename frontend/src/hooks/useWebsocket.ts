import React, { useEffect, useRef } from 'react'
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
  const wsRef = useRef<ReconnectingWebSocket>()
  const messageQueue = useRef<IWebsocketEvent[]>([])
  const processingRef = useRef(false)

  const processMessageQueue = () => {
    if (processingRef.current || messageQueue.current.length === 0) return;
    processingRef.current = true;

    // Process all messages in the queue
    while (messageQueue.current.length > 0) {
      const message = messageQueue.current.shift();
      if (message) {
        handler(message);
      }
    }

    processingRef.current = false;
  }

  useEffect(() => {
    if(!account.token) return
    if(!session_id) return
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsHost = window.location.host
    const url = `${wsProtocol}//${wsHost}/api/v1/ws/user?session_id=${session_id}`
    
    console.log('fox: useWebsocket - Connecting to websocket', { session_id, url });
    
    const rws = new ReconnectingWebSocket(url, [], {
      maxRetries: 10,
      reconnectionDelayGrowFactor: 1.3,
      maxReconnectionDelay: 10000,
      minReconnectionDelay: 1000,
    })

    wsRef.current = rws

    const messageHandler = (event: MessageEvent<any>) => {
      const parsedData = JSON.parse(event.data) as IWebsocketEvent
      
      console.log('fox: useWebsocket - Received message', { 
        type: parsedData.type, 
        session_id: parsedData.session_id,
        hasSessionData: !!parsedData.session,
        interactionCount: parsedData.session?.interactions?.length
      });
      
      if(parsedData.session_id != session_id) {
        console.log('fox: useWebsocket - Ignoring message for different session');
        return
      }

      // Add message to queue
      messageQueue.current.push(parsedData)

      // Process queue in next animation frame
      requestAnimationFrame(processMessageQueue)
    }

    rws.addEventListener('message', messageHandler)

    return () => {
      console.log('fox: useWebsocket - Disconnecting websocket', { session_id });
      if (wsRef.current) {
        wsRef.current.removeEventListener('message', messageHandler)
        wsRef.current.close()
      }
    }
  }, [
    account.token,
    session_id,
  ])
}

export default useWebsocket