import { useState, useCallback } from 'react'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import { 
  TypesSpecTask, 
  TypesSpecTaskUpdateRequest,
  TypesSpecApprovalResponse,
  ServerApprovalWithHandoffRequest,
  ServerCombinedApprovalHandoffResult,
  ServicesDocumentHandoffConfig,
  ServicesHandoffResult,
  ServicesSpecDocumentConfig,
  ServicesSpecDocumentResult,
  ServerCoordinationLogResponse,
  ServicesDocumentHandoffStatus,
  ServerSpecDocumentContentResponse,
  TypesSpecTaskMultiSessionOverviewResponse,
  TypesSpecTaskProgressResponse
} from '../api/api'

export const useSpecTasks = () => {
  const api = useApi()
  const snackbar = useSnackbar()
  
  const [data, setData] = useState<TypesSpecTask[]>([])
  const [loading, setLoading] = useState(false)

  const listTasks = useCallback(async (filters?: {
    project_id?: string;
    status?: string;
    user_id?: string;
    type?: string;
    priority?: string;
    limit?: number;
    offset?: number;
  }) => {
    setLoading(true)
    try {
      const result = await api.getApiClient().v1SpecTasksList(filters)
      if (result.data) {
        setData(result.data)
        return result.data
      }
    } catch (error) {
      snackbar.error('Failed to load spec tasks')
      console.error('Error loading spec tasks:', error)
    } finally {
      setLoading(false)
    }
    return []
  }, [api, snackbar])

  const getTask = useCallback(async (taskId: string) => {
    try {
      const result = await api.getApiClient().v1SpecTasksDetail(taskId)
      return result.data
    } catch (error) {
      snackbar.error('Failed to load spec task')
      console.error('Error loading spec task:', error)
      return null
    }
  }, [api, snackbar])

  const updateTask = useCallback(async (taskId: string, updates: TypesSpecTaskUpdateRequest) => {
    try {
      const result = await api.getApiClient().v1SpecTasksUpdate(taskId, updates)
      if (result.data) {
        // Update local data if task is in the list
        setData(prev => prev.map(task => 
          task.id === taskId ? result.data! : task
        ))
        snackbar.success('Task updated successfully')
        return result.data
      }
    } catch (error) {
      snackbar.error('Failed to update task')
      console.error('Error updating task:', error)
    }
    return null
  }, [api, snackbar])

  const approveSpecs = useCallback(async (taskId: string, approval: TypesSpecApprovalResponse) => {
    try {
      const result = await api.getApiClient().v1SpecTasksApproveSpecsCreate(taskId, approval)
      if (result.data) {
        snackbar.success('Specifications approved successfully')
        return result.data
      }
    } catch (error) {
      snackbar.error('Failed to approve specifications')
      console.error('Error approving specs:', error)
    }
    return null
  }, [api, snackbar])

  const approveWithHandoff = useCallback(async (taskId: string, request: ServerApprovalWithHandoffRequest): Promise<ServerCombinedApprovalHandoffResult | null> => {
    try {
      const result = await api.getApiClient().v1SpecTasksApproveWithHandoffCreate(taskId, request)
      if (result.data) {
        snackbar.success('Specifications approved and handoff initiated')
        return result.data
      }
    } catch (error) {
      snackbar.error('Failed to approve and handoff')
      console.error('Error in approve with handoff:', error)
    }
    return null
  }, [api, snackbar])

  const executeHandoff = useCallback(async (taskId: string, config: ServicesDocumentHandoffConfig): Promise<ServicesHandoffResult | null> => {
    try {
      const result = await api.getApiClient().v1SpecTasksExecuteHandoffCreate(taskId, config)
      if (result.data) {
        snackbar.success('Document handoff executed successfully')
        return result.data
      }
    } catch (error) {
      snackbar.error('Failed to execute handoff')
      console.error('Error executing handoff:', error)
    }
    return null
  }, [api, snackbar])

  const generateDocuments = useCallback(async (taskId: string, config: ServicesSpecDocumentConfig): Promise<ServicesSpecDocumentResult | null> => {
    try {
      const result = await api.getApiClient().v1SpecTasksGenerateDocumentsCreate(taskId, config)
      if (result.data) {
        snackbar.success('Documents generated successfully')
        return result.data
      }
    } catch (error) {
      snackbar.error('Failed to generate documents')
      console.error('Error generating documents:', error)
    }
    return null
  }, [api, snackbar])

  const getCoordinationLog = useCallback(async (taskId: string, filters?: {
    event_type?: string;
    limit?: number;
    offset?: number;
  }): Promise<ServerCoordinationLogResponse | null> => {
    try {
      const result = await api.getApiClient().v1SpecTasksCoordinationLogDetail(taskId, filters)
      return result.data || null
    } catch (error) {
      snackbar.error('Failed to load coordination log')
      console.error('Error loading coordination log:', error)
    }
    return null
  }, [api, snackbar])

  const getDocumentStatus = useCallback(async (taskId: string): Promise<ServicesDocumentHandoffStatus | null> => {
    try {
      const result = await api.getApiClient().v1SpecTasksDocumentStatusDetail(taskId)
      return result.data || null
    } catch (error) {
      snackbar.error('Failed to load document status')
      console.error('Error loading document status:', error)
    }
    return null
  }, [api, snackbar])

  const getDocument = useCallback(async (taskId: string, document: "requirements" | "design" | "tasks" | "metadata"): Promise<ServerSpecDocumentContentResponse | null> => {
    try {
      const result = await api.getApiClient().v1SpecTasksDocumentsDetail(taskId, document)
      return result.data || null
    } catch (error) {
      snackbar.error(`Failed to load ${document} document`)
      console.error(`Error loading ${document} document:`, error)
    }
    return null
  }, [api, snackbar])

  const getMultiSessionOverview = useCallback(async (taskId: string): Promise<TypesSpecTaskMultiSessionOverviewResponse | null> => {
    try {
      const result = await api.getApiClient().v1SpecTasksMultiSessionOverviewDetail(taskId)
      return result.data || null
    } catch (error) {
      snackbar.error('Failed to load multi-session overview')
      console.error('Error loading multi-session overview:', error)
    }
    return null
  }, [api, snackbar])

  const getProgress = useCallback(async (taskId: string): Promise<TypesSpecTaskProgressResponse | null> => {
    try {
      const result = await api.getApiClient().v1SpecTasksProgressDetail(taskId)
      return result.data || null
    } catch (error) {
      snackbar.error('Failed to load task progress')
      console.error('Error loading task progress:', error)
    }
    return null
  }, [api, snackbar])

  const reload = useCallback(async (filters?: Parameters<typeof listTasks>[0]) => {
    return await listTasks(filters)
  }, [listTasks])

  return {
    data,
    loading,
    listTasks,
    getTask,
    updateTask,
    approveSpecs,
    approveWithHandoff,
    executeHandoff,
    generateDocuments,
    getCoordinationLog,
    getDocumentStatus,
    getDocument,
    getMultiSessionOverview,
    getProgress,
    reload,
    setData
  }
}

export const useSpecTask = (taskId: string) => {
  const [data, setData] = useState<TypesSpecTask | null>(null)
  const [loading, setLoading] = useState(false)
  const { getTask } = useSpecTasks()

  const load = useCallback(async () => {
    if (!taskId) return
    setLoading(true)
    try {
      const task = await getTask(taskId)
      setData(task)
      return task
    } finally {
      setLoading(false)
    }
  }, [taskId, getTask])

  const reload = useCallback(() => load(), [load])

  return {
    data,
    loading,
    load,
    reload,
    setData
  }
}