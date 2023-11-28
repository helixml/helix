import React, { useState } from 'react'
import useWebsocket from './useWebsocket'

import {
  WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE,
  WORKER_TASK_RESPONSE_TYPE_PROGRESS,
  WORKER_TASK_RESPONSE_TYPE_STREAM,
} from '../types'

export const useLiveInteraction = (session_id: string) => {
  const [ message, setMessage ] = useState('')
  const [ progress, setProgress ] = useState(0)
  const [ status, setStatus ] = useState('')

  useWebsocket(session_id, (parsedData) => {
    if(!session_id) return
    if(parsedData.type == WEBSOCKET_EVENT_TYPE_WORKER_TASK_RESPONSE && parsedData.worker_task_response) {
      const workerResponse = parsedData.worker_task_response
      if(workerResponse.type == WORKER_TASK_RESPONSE_TYPE_STREAM && workerResponse.message) {
        setMessage(m => m + workerResponse.message)
      } else if(workerResponse.type == WORKER_TASK_RESPONSE_TYPE_PROGRESS) {
        if(workerResponse.progress) {
          setProgress(workerResponse.progress)
        }
        if(workerResponse.status) {
          setStatus(workerResponse.status)
        }
      }
    }
  })

  return {
    message,
    progress,
    status,
  }
}

export default useLiveInteraction