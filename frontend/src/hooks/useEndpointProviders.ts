import { useState, useCallback, useRef, useEffect } from 'react'
import { throttle } from 'lodash'
import useApi from './useApi'
import {
  TypesProviderEndpoint,
} from '../api/api'

export const useEndpointProviders = () => {
  const api = useApi()
  const mountedRef = useRef(true)
  const [isLoading, setIsLoading] = useState(false)
  const [data, setData] = useState<TypesProviderEndpoint[]>([])
  const [endpoint, setEndpoint] = useState<TypesProviderEndpoint>()

  // Create a ref for the throttled function
  const throttledLoadRef = useRef<any>(null)

  // Initialize the throttled function once
  useEffect(() => {
    throttledLoadRef.current = throttle(async (force = false) => {
      if (isLoading) return
      setIsLoading(true)
      
      try {
        let result = await api.get<TypesProviderEndpoint[]>('/api/v1/provider-endpoints', undefined, {
          snackbar: true,
        })
        
        if(result === null) result = []
        if(!mountedRef.current) return
        setData(result)
      } finally {
        if(mountedRef.current) {
          setIsLoading(false)
        }
      }
    }, 1000, { leading: true, trailing: false })

    // Cleanup
    return () => {
      if (throttledLoadRef.current?.cancel) {
        throttledLoadRef.current.cancel()
      }
      mountedRef.current = false
    }
  }, [])

  // Expose loadData as a wrapper around the throttled function
  const loadData = useCallback(async (force = false) => {
    if (throttledLoadRef.current) {
      await throttledLoadRef.current(force)
    }
  }, [])

  const createEndpoint = useCallback(async (endpoint: Partial<TypesProviderEndpoint>): Promise<TypesProviderEndpoint | undefined> => {
    try {
      const result = await api.post<Partial<TypesProviderEndpoint>, TypesProviderEndpoint>('/api/v1/provider-endpoints', endpoint, {}, {
        snackbar: true,
      })
      if (!result) return undefined
      await loadData()
      return result
    } catch (error) {
      console.error('useEndpointProviders: Error creating endpoint:', error)
      throw error
    }
  }, [api, loadData])

  const updateEndpoint = useCallback(async (id: string, updatedEndpoint: Partial<TypesProviderEndpoint>): Promise<TypesProviderEndpoint | undefined> => {
    try {
      const result = await api.put<Partial<TypesProviderEndpoint>, TypesProviderEndpoint>(`/api/v1/provider-endpoints/${id}`, updatedEndpoint, {}, {
        snackbar: true,
      })
      if (!result) return undefined
      await loadData()
      return result
    } catch (error) {
      console.error('useEndpointProviders: Error updating endpoint:', error)
      throw error
    }
  }, [api, loadData])

  const deleteEndpoint = useCallback(async (id: string): Promise<boolean> => {
    try {
      await api.delete(`/api/v1/provider-endpoints/${id}`, {}, {
        snackbar: true,
      })
      await loadData()
      return true
    } catch (error) {
      console.error('useEndpointProviders: Error deleting endpoint:', error)
      throw error
    }
  }, [api, loadData])

  const getEndpoint = useCallback(async (id: string, showErrors: boolean = true): Promise<void> => {
    if (!id) return
    const result = await api.get<TypesProviderEndpoint>(`/api/v1/provider-endpoints/${id}`, undefined, {
      snackbar: showErrors,
    })
    if (!result || !mountedRef.current) return
    setEndpoint(result)
    setData(prevData => prevData.map(e => e.id === id ? result : e))
  }, [api])

  return {
    data,
    endpoint,
    setEndpoint,
    isLoading,
    loadData,
    createEndpoint,
    updateEndpoint,
    deleteEndpoint,
    getEndpoint,
  }
}

export default useEndpointProviders
