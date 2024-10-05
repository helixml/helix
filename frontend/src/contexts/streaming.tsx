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
    console.log(`[Streaming] Creating new request with id: ${id}`)
    console.log(`[Streaming] Messages:`, messages)
    console.log(`[Streaming] App ID:`, appId)

    setRequests(prev => {
      console.log(`[Streaming] Setting initial request state for id: ${id}`)
      return {
        ...prev,
        [id]: { id, buffer: '', callbacks: [], errorCallbacks: [], completed: false }
      }
    })

    try {
      console.log(`[Streaming] Sending POST request to /api/v1/sessions/chat`)
      const response = await api.post('/api/v1/sessions/chat', {
        messages,
        stream: true,
        app_id: appId,
      }, {
        responseType: 'text',
        onDownloadProgress: (progressEvent) => {
          const chunk = progressEvent.event.target.response
          const lines = chunk.split('\n')
          lines.forEach((line: string) => {
            if (line.startsWith('data: ')) {
              const jsonData = line.slice(6)
              if (jsonData === '[DONE]') {
                console.log(`[Streaming] Received [DONE] for request ${id}`)
                completeRequest(id)
              } else {
                try {
                  const parsed = JSON.parse(jsonData)
                  const content = parsed.choices[0].delta.content
                  if (content) {
                    console.log(`[Streaming] Received chunk:`, content)
                    updateRequest(id, content)
                  }
                } catch (error) {
                  console.error(`[Streaming] Error parsing chunk:`, error)
                }
              }
            }
          })
        }
      })

      console.log(`[Streaming] Response received:`, response)

      if (response.data && typeof response.data.on === 'function') {
        console.log(`[Streaming] Response is a stream, attaching 'end' event listener`)
        response.data.on('end', () => {
          console.log(`[Streaming] Stream ended for request ${id}`)
          completeRequest(id)
        })
      } else {
        console.log(`[Streaming] Response is not a stream, completing request immediately`)
        completeRequest(id)
      }

    } catch (error) {
      console.error('[Streaming] Error in createRequest:', error)
      setRequests(prev => {
        const request = prev[id]
        if (!request) {
          console.log(`[Streaming] No request found for id: ${id}`)
          return prev
        }

        console.log(`[Streaming] Calling error callbacks for request ${id}`)
        request.errorCallbacks.forEach(callback => callback(error as Error))
        return prev
      })
      removeRequest(id)
    }

    console.log(`[Streaming] Returning request id: ${id}`)
    return id
  }, [api])

  const attachCallback = useCallback((id: string, callback: (content: string) => void, errorCallback: (error: Error) => void) => {
    console.log(`[Streaming] Attaching callback for request ${id}`)
    setRequests(prev => {
      const request = prev[id]
      if (!request) {
        console.log(`[Streaming] No request found for id: ${id}`)
        return prev
      }

      if (request.buffer) {
        console.log(`[Streaming] Calling callback immediately with buffered content for request ${id}`)
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
  // data:
  // {"id":"ses_01j993z0ffc3dxgg70j27ax12t","object":"chat.completion.chunk","created":1727956756,"model":"llama3.1:8b","choices":[{"index":0,"delta":{"content":"
  // regarding","role":"assistant"},"finish_reason":null,"content_filter_results":{"hate":{"filtered":false},"self_harm":{"filtered":false},"sexual":{"filtered":false},"violence":{"filtered":false}}}],"system_fingerprint":"fp_ollama"}


  const updateRequest = useCallback((id: string, chunk: string) => {
    console.log(`[Streaming] Updating request ${id} with chunk:`, chunk)
    setRequests(prev => {
      const request = prev[id]
      if (!request) {
        console.log(`[Streaming] No request found for id: ${id}`)
        return prev
      }

      const updatedBuffer = request.buffer + chunk
      console.log(`[Streaming] Calling ${request.callbacks.length} callbacks for request ${id}`)
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
    console.log(`[Streaming] Completing request ${id}`)
    setRequests(prev => {
      const request = prev[id]
      if (!request) {
        console.log(`[Streaming] No request found for id: ${id}`)
        return prev
      }

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
    console.log(`[Streaming] Removing request ${id}`)
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