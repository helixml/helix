import React, { useState, useMemo, useEffect } from 'react'
import useWebsocket from './useWebsocket'

import {
  WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE,
  WORKER_TASK_RESPONSE_TYPE_PROGRESS,
  WORKER_TASK_RESPONSE_TYPE_STREAM,
  IInteraction,
} from '../types'

export const useLiveInteraction = ({
  session_id,
  interaction,
  staleThreshold = 10000,
}: {
  session_id: string,
  interaction: IInteraction,
  // after how much time without an event do we mark ourselves as stale?
  // this gives the UI a chance to say "you've been in the queue for a while"
  staleThreshold?: number,
}) => {
  const [ message, setMessage ] = useState(interaction.message)
  const [ progress, setProgress ] = useState(interaction.progress)
  const [ status, setStatus ] = useState(interaction.status)
  const [ recentTimestamp, setRecentTimestamp ] = useState(Date.now())
  const [ stateCounter, setStaleCounter ] = useState(0)

  const isStale = useMemo(() => {
    return (Date.now() - recentTimestamp) > staleThreshold
  }, [
    recentTimestamp,
    staleThreshold,
    stateCounter,
  ])

  useWebsocket(session_id, (parsedData) => {
    if(!session_id) return
    setRecentTimestamp(Date.now())
    if(parsedData.type == WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE && parsedData.worker_task_response) {
      const workerResponse = parsedData.worker_task_response
      if(workerResponse.type == WORKER_TASK_RESPONSE_TYPE_STREAM && workerResponse.message) {
        setMessage(m => m + workerResponse.message)
      } else if(workerResponse.type == WORKER_TASK_RESPONSE_TYPE_PROGRESS) {
        if(workerResponse.message) {
          setMessage(workerResponse.message)
        }
        if(workerResponse.progress) {
          setProgress(workerResponse.progress)
        }
        if(workerResponse.status) {
          setStatus(workerResponse.status)
        }
      }
    }
  })

  useEffect(() => {
    const intervalID = setInterval(() => {
      setStaleCounter(c => c + 1)
    }, 1000)
    return () => clearInterval(intervalID)
  }, [])

  return {
    message,
    progress,
    status,
    isStale,
  }
}

export default useLiveInteraction