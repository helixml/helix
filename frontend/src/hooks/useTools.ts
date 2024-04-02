import React, { FC, useState, useCallback, useMemo } from 'react'
import useApi from '../hooks/useApi'

import {
  ITool,
  IToolConfig,
  IToolType,
} from '../types'

import {
  generateAmusingName,
} from '../utils/names'

export const useTools = () => {
  const api = useApi()
  
  const [ data, setData ] = useState<ITool[]>([])

  const userTools = useMemo(() => {
    return data.filter(tool => !tool.global)
  }, [
    data,
  ])

  const globalTools = useMemo(() => {
    return data.filter(tool => tool.global)
  }, [
    data,
  ])
  
  const loadData = useCallback(async () => {
    const result = await api.get<ITool[]>(`/api/v1/tools`, undefined, {
      snackbar: true,
    })
    if(!result) return
    setData(result)
  }, [])

  // const createTool = useCallback(async (url: string, schema: string): Promise<ITool | undefined> => {
  const createTool = useCallback(async (name: string, tool_type: IToolType, description: string, config: IToolConfig): Promise<ITool | undefined> => {
    const result = await api.post<Partial<ITool>, ITool>(`/api/v1/tools`, {
      name: name ? name: generateAmusingName(),
      description: description,
      tool_type: tool_type,
      config: config,
      // config: {
      //   api: {
      //     url,
      //     schema,
      //     actions: [],
      //     headers: {},
      //     query: {},
      //   }
      // }
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
    userTools,
    globalTools,
    loadData,
    createTool,
    updateTool,
    deleteTool,
  }
}

export default useTools