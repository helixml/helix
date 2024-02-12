import React, { FC, useState, useCallback, useEffect } from 'react'
import useApi from '../hooks/useApi'

import {
  ITool,
} from '../types'

import {
  generateAmusingName,
} from '../utils/names'

export const useTools = () => {
  const api = useApi()
  
  const [ data, setData ] = useState<ITool[]>([])
  
  const loadData = useCallback(async () => {
    const result = await api.get<ITool[]>(`/api/v1/tools`, undefined, {
      snackbar: true,
    })
    if(!result) return
    setData(result)
  }, [])

  const createTool = useCallback(async (url: string, schema: string): Promise<ITool | undefined> => {
    const result = await api.post<Partial<ITool>, ITool>(`/api/v1/tools`, {
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

  const updateTool = useCallback(async (id: string, data: Partial<ITool>): Promise<ITool | undefined> => {
    const result = await api.put<Partial<ITool>, ITool>(`/api/v1/tools/${id}`, data, {}, {
      snackbar: true,
    })
    if(!result) return
    loadData()
    return result
  }, [
    loadData,
  ])

  return {
    data,
    loadData,
    createTool,
    updateTool,
  }
}

export default useTools