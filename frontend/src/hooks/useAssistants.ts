import React, { FC, useState, useCallback, useEffect } from 'react'
import useApi from '../hooks/useApi'

import {
  IAssistant,
} from '../types'

import {
  generateAmusingName,
} from '../utils/names'

export const useAssistants = () => {
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

  return {
    data,
    loadData,
    createTool,
  }
}

export default useAssistants