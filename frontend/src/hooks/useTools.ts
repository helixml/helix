import React, { FC, useState, useCallback, useMemo, useRef, useEffect } from 'react'
import useApi from '../hooks/useApi'

import {
  ITool,
  IToolConfig,
  IToolType,
  IApiOptions,
} from '../types'

import {
  generateAmusingName,
} from '../utils/names'

export const useTools = () => {
  const api = useApi()
  
  const [ data, setData ] = useState<ITool[]>([])
  const abortControllerRef = useRef<AbortController | null>(null)

  useEffect(() => {
    return () => {
      if (abortControllerRef.current) {
        abortControllerRef.current.abort()
      }
    }
  }, [])

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
    if (abortControllerRef.current) {
      abortControllerRef.current.abort()
    }
    abortControllerRef.current = new AbortController()

    const options: IApiOptions = {
      snackbar: true,
      signal: abortControllerRef.current.signal,
    }

    const result = await api.get<ITool[]>(`/api/v1/tools`, undefined, options)
    if(!result) return
    setData(result)
  }, [api])

  const createTool = useCallback(async (name: string, tool_type: IToolType, description: string, config: IToolConfig): Promise<ITool | undefined> => {
    const result = await api.post<Partial<ITool>, ITool>(`/api/v1/tools`, {
      name: name ? name: generateAmusingName(),
      description: description,
      tool_type: tool_type,
      config: config,
    }, {}, {
      snackbar: true,
    })
    if(!result) return
    loadData()
    return result
  }, [
    loadData,
  ])

  const updateTool = useCallback(async (id: string, updatedTool: ITool): Promise<ITool | undefined> => {
    console.log("Updating tool:", id, updatedTool)
    const result = await api.put<ITool, ITool>(`/api/v1/tools/${id}`, updatedTool, {}, {
      snackbar: true,
    })
    console.log("Update result:", result)
    if(!result) return
    loadData()
    return result
  }, [loadData])

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