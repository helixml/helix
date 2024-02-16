import React, { FC, useState, useCallback, useEffect } from 'react'
import useApi from '../hooks/useApi'

import {
  IAssistant,
} from '../types'

import {
  generateAmusingName,
} from '../utils/names'

export const useTools = () => {
  const api = useApi()
  
  const [ data, setData ] = useState<IAssistant[]>([])
  
  const loadData = useCallback(async () => {
    const result = await api.get<IAssistant[]>(`/api/v1/tools`, undefined, {
      snackbar: true,
    })
    if(!result) return
    setData(result)
  }, [])

  const createTool = useCallback(async (url: string, schema: string): Promise<IAssistant | undefined> => {
    const result = await api.post<Partial<IAssistant>, IAssistant>(`/api/v1/tools`, {
      name: generateAmusingName(),
      tool_type: 'api',
      config: {
        api: {
          url,
          schema,
          actions: [],
          headers: {},
          query: {},
        }
      }
    }, {}, {
      snackbar: true,
    })
    if(!result) return
    loadData()
    return result
  }, [
    loadData,
  ])

  const updateTool = useCallback(async (id: string, data: Partial<IAssistant>): Promise<IAssistant| undefined> => {
    const result = await api.put<Partial<IAssistant>, IAssistant>(`/api/v1/tools/${id}`, data, {}, {
      snackbar: true,
    })
    if(!result) return
    loadData()
    return result
  }, [
    loadData,
  ])

  const deleteTool = useCallback(async (id: string): Promise<boolean | undefined> => {
    await api.delete(`/api/v1/tools/${id}`, {}, {
      snackbar: true,
    })
    loadData()
    return true
  }, [
    loadData,
  ])

  return {
    data,
    loadData,
    createTool,
    updateTool,
    deleteTool,
  }
}

export default useTools