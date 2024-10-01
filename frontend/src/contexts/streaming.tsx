import React, { FC, createContext, useContext, useState, useCallback, useMemo } from 'react'
import { v4 as uuidv4 } from 'uuid'

interface StreamingRequest {
  id: string
  buffer: string
  callbacks: ((content: string) => void)[]
  completed: boolean
}

export interface IStreamingContext {
  createRequest: () => string
  attachCallback: (id: string, callback: (content: string) => void) => void
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

  const createRequest = useCallback(() => {
    const id = uuidv4()
    setRequests(prev => ({
      ...prev,
      [id]: { id, buffer: '', callbacks: [], completed: false }
    }))
    return id
  }, [])

  const attachCallback = useCallback((id: string, callback: (content: string) => void) => {
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
          callbacks: [...request.callbacks, callback]
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