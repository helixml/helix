import { useState, useEffect, useCallback } from 'react'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import { 
  ServerSampleTypesResponse,
  ServerSampleType,
  ServerCreateSampleRepositoryRequest,
  ServicesGitRepository,
  ServerInitializeSampleRepositoriesRequest,
  ServerInitializeSampleRepositoriesResponse
} from '../api/api'

export const useSampleTypes = () => {
  const api = useApi()
  const snackbar = useSnackbar()
  
  const [data, setData] = useState<ServerSampleType[]>([])
  const [loading, setLoading] = useState(true)

  // Auto-load sample types on mount
  useEffect(() => {
    const loadData = async () => {
      try {
        const result = await api.getApiClient().v1SpecsSampleTypesList()
        if (result.data?.sample_types) {
          setData(result.data.sample_types)
        }
      } catch (error) {
        console.error('Error loading sample types:', error)
        snackbar.error('Failed to load sample types')
      } finally {
        setLoading(false)
      }
    }
    
    loadData()
  }, []) // Empty dependency array - load once on mount

  const loadSampleTypes = useCallback(async () => {
    setLoading(true)
    try {
      const result = await api.getApiClient().v1SpecsSampleTypesList()
      if (result.data?.sample_types) {
        setData(result.data.sample_types)
        return result.data.sample_types
      }
    } catch (error) {
      console.error('Error loading sample types:', error)
      snackbar.error('Failed to load sample types')
    } finally {
      setLoading(false)
    }
    return []
  }, []) // No dependencies to avoid infinite loops

  const createSampleRepository = useCallback(async (request: ServerCreateSampleRepositoryRequest): Promise<ServicesGitRepository> => {
    const result = await api.getApiClient().v1SamplesRepositoriesCreate(request)
    if (!result.data) {
      throw new Error('No data returned from API')
    }
    return result.data
  }, [api])

  const initializeSampleRepositories = useCallback(async (request: ServerInitializeSampleRepositoriesRequest): Promise<ServerInitializeSampleRepositoriesResponse | null> => {
    try {
      const result = await api.getApiClient().v1SamplesInitializeCreate(request)
      if (result.data) {
        snackbar.success('Sample repositories initialized successfully')
        return result.data
      }
    } catch (error) {
      snackbar.error('Failed to initialize sample repositories')
      console.error('Error initializing sample repositories:', error)
    }
    return null
  }, [])

  const reload = useCallback(async () => {
    return await loadSampleTypes()
  }, [])

  return {
    data,
    loading,
    loadSampleTypes,
    createSampleRepository,
    initializeSampleRepositories,
    reload,
    setData
  }
}

export const useCreateSampleRepository = () => {
  const [loading, setLoading] = useState(false)
  const { createSampleRepository } = useSampleTypes()

  const create = useCallback(async (request: ServerCreateSampleRepositoryRequest) => {
    setLoading(true)
    try {
      return await createSampleRepository(request)
    } finally {
      setLoading(false)
    }
  }, [])

  return {
    create,
    loading
  }
}