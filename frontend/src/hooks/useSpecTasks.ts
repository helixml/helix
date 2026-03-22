import { useState, useCallback } from 'react'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import {
  TypesSpecTask,
  TypesSpecTaskUpdateRequest,
  TypesSpecApprovalResponse,
  ServerTaskProgressResponse,
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
    include_archived?: boolean;
    with_depends_on?: boolean;
    labels?: string;
    limit?: number;
    offset?: number;
  }) => {
    setLoading(true)
    try {
      const result = await api.getApiClient().v1SpecTasksList(filters as { project_id: string; status?: string; user_id?: string; include_archived?: boolean; with_depends_on?: boolean; labels?: string; limit?: number; offset?: number })
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

  // TODO: These methods use API endpoints not yet in the generated client
  const approveWithHandoff = useCallback(async (_taskId: string, _request: Record<string, unknown>): Promise<Record<string, unknown> | null> => {
    snackbar.error('approveWithHandoff: API endpoint not available')
    return null
  }, [snackbar])

  const executeHandoff = useCallback(async (_taskId: string, _config: Record<string, unknown>): Promise<Record<string, unknown> | null> => {
    snackbar.error('executeHandoff: API endpoint not available')
    return null
  }, [snackbar])

  const generateDocuments = useCallback(async (_taskId: string, _config: Record<string, unknown>): Promise<Record<string, unknown> | null> => {
    snackbar.error('generateDocuments: API endpoint not available')
    return null
  }, [snackbar])

  const getCoordinationLog = useCallback(async (_taskId: string, _filters?: Record<string, unknown>): Promise<Record<string, unknown> | null> => {
    snackbar.error('getCoordinationLog: API endpoint not available')
    return null
  }, [snackbar])

  const getDocumentStatus = useCallback(async (_taskId: string): Promise<Record<string, unknown> | null> => {
    snackbar.error('getDocumentStatus: API endpoint not available')
    return null
  }, [snackbar])

  const getDocument = useCallback(async (_taskId: string, _document: string): Promise<Record<string, unknown> | null> => {
    snackbar.error('getDocument: API endpoint not available')
    return null
  }, [snackbar])

  const getMultiSessionOverview = useCallback(async (_taskId: string): Promise<Record<string, unknown> | null> => {
    snackbar.error('getMultiSessionOverview: API endpoint not available')
    return null
  }, [snackbar])

  const getProgress = useCallback(async (taskId: string): Promise<ServerTaskProgressResponse | null> => {
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