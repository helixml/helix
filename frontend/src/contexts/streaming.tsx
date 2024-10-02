import React, { FC, createContext, useContext, useState, useCallback, useMemo } from 'react'
import { v4 as uuidv4 } from 'uuid'
import { useApi } from '../hooks/useApi'

interface StreamingRequest {
  id: string
  buffer: string
  callbacks: ((content: string) => void)[]
  errorCallbacks: ((error: Error) => void)[]
  completed: boolean
}

export interface IStreamingContext {
  createRequest: (messages: any[], appId?: string) => Promise<string>
  attachCallback: (id: string, callback: (content: string) => void, errorCallback: (error: Error) => void) => void
  updateRequest: (id: string, chunk: string) => void
  completeRequest: (id: string) => void
  removeRequest: (id: string) => void
}

const StreamingContext = createContext<IStreamingContext | null>(null)

export const useStreamingContext = (): IStreamingContext => {
  const context = useContext(StreamingContext)
  if (!context) {
    throw new Error('useStreamingContext must be used within a StreamingContextProvider')
  }
  return context
}

export const StreamingContextProvider: FC = ({ children }) => {
  const [requests, setRequests] = useState<Record<string, StreamingRequest>>({})
  const api = useApi()

  const createRequest = useCallback(async (messages: any[], appId?: string) => {
    const id = uuidv4()
    setRequests(prev => ({
      ...prev,
      [id]: { id, buffer: '', callbacks: [], errorCallbacks: [], completed: false }
    }))

    try {
      const response = await api.post('/api/v1/sessions/chat', {
        messages,
        stream: true,
        app_id: appId, // Add the app_id to the request payload
      }, {
        responseType: 'stream',
        onDownloadProgress: (progressEvent) => {
          const chunk = progressEvent.event.target.response
          updateRequest(id, chunk)
        }
      })

      // Handle the completion of the stream
      response.data.on('end', () => {
        completeRequest(id)
      })

    } catch (error) {
      console.error('Error in createRequest:', error)
      setRequests(prev => {
        const request = prev[id]
        if (!request) return prev

        request.errorCallbacks.forEach(callback => callback(error as Error))
        return prev
      })
      removeRequest(id)
    }

    return id
  }, [api])

  const attachCallback = useCallback((id: string, callback: (content: string) => void, errorCallback: (error: Error) => void) => {
    setRequests(prev => {
      const request = prev[id]
      if (!request) return prev

      // Call the callback immediately with the buffered content
      if (request.buffer) {
        callback(request.buffer)
      }

      return {
        ...prev,
        [id]: {
          ...request,
          callbacks: [...request.callbacks, callback],
          errorCallbacks: [...request.errorCallbacks, errorCallback]
        }
      }
    })
  }, [])

  const updateRequest = useCallback((id: string, chunk: string) => {
    setRequests(prev => {
      const request = prev[id]
      if (!request) return prev

      const updatedBuffer = request.buffer + chunk
      request.callbacks.forEach(callback => callback(chunk))

      return {
        ...prev,
        [id]: {
          ...request,
          buffer: updatedBuffer
        }
      }
    })
  }, [])

  const completeRequest = useCallback((id: string) => {
    setRequests(prev => {
      const request = prev[id]
      if (!request) return prev

      return {
        ...prev,
        [id]: {
          ...request,
          completed: true
        }
      }
    })
  }, [])

  const removeRequest = useCallback((id: string) => {
    setRequests(prev => {
      const { [id]: _, ...rest } = prev
      return rest
    })
  }, [])

  const value = useMemo(() => ({
    createRequest,
    attachCallback,
    updateRequest,
    completeRequest,
    removeRequest
  }), [createRequest, attachCallback, updateRequest, completeRequest, removeRequest])

  return (
    <StreamingContext.Provider value={value}>
      {children}
    </StreamingContext.Provider>
  )
}