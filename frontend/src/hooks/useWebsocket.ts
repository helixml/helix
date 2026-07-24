import React, { useEffect, useRef } from 'react'
import ReconnectingWebSocket from 'reconnecting-websocket'

import {
  IWebsocketEvent,
} from '../types'

export const useWebsocket = (
  session_id: string,
  handler: {
    (ev: IWebsocketEvent): void,
  },
) => {
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
    // With BFF auth, session cookie is automatically sent with WebSocket connections
    if(!session_id) return
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsHost = window.location.host
    const url = `${wsProtocol}//${wsHost}/api/v1/ws/user?session_id=${session_id}`

    const rws = new ReconnectingWebSocket(url, [], {
      // Never permanently give up: a long offline stretch (laptop asleep, wifi
      // down) would otherwise burn through a finite retry budget and leave the
      // socket dead forever, forcing a manual page refresh to resume streaming.
      maxRetries: Infinity,
      reconnectionDelayGrowFactor: 1.3,
      maxReconnectionDelay: 10000,
      minReconnectionDelay: 1000,
    })

    wsRef.current = rws

    const messageHandler = (event: MessageEvent<any>) => {
      const parsedData = JSON.parse(event.data) as IWebsocketEvent

      if(parsedData.session_id != session_id) {
        return
      }

      // Add message to queue
      messageQueue.current.push(parsedData)

      // Process queue in next animation frame
      requestAnimationFrame(processMessageQueue)
    }

    rws.addEventListener('message', messageHandler)

    // After a laptop sleep / wifi switch the socket is often left half-open (the
    // browser reports it OPEN but no data flows and no 'close' fires). Force a
    // reconnect when the tab regains focus or the network returns so updates
    // resume without a manual refresh.
    const forceReconnectOnResume = () => {
      if (document.visibilityState === 'visible') rws.reconnect()
    }
    const handleOnline = () => rws.reconnect()
    document.addEventListener('visibilitychange', forceReconnectOnResume)
    window.addEventListener('online', handleOnline)

    return () => {
      document.removeEventListener('visibilitychange', forceReconnectOnResume)
      window.removeEventListener('online', handleOnline)
      if (wsRef.current) {
        wsRef.current.removeEventListener('message', messageHandler)
        wsRef.current.close()
      }
    }
  }, [session_id])
}

export default useWebsocket