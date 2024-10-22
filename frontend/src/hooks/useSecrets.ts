import React, { FC, useState, useCallback, useMemo } from 'react'
import useApi from '../hooks/useApi'

import {
  ISecret,
  ICreateSecret,
} from '../types'

export const useSecrets = () => {
  const api = useApi()
  
  const [data, setData] = useState<ISecret[]>([])
  const [secret, setSecret] = useState<ISecret>()

  const loadData = useCallback(async () => {
    const result = await api.get<ISecret[]>(`/api/v1/secrets`, undefined, {
      snackbar: true,
    })
    if (!result) return
    setData(result)
  }, [api])

  const loadSecret = useCallback(async (id: string) => {
    if (!id) return
    const result = await api.get<ISecret>(`/api/v1/secrets/${id}`, undefined, {
      snackbar: true,
    })
    if (!result) return
    setSecret(result)
    setData(prevData => prevData.map(s => s.id === id ? result : s))
  }, [api])

  const createSecret = useCallback(async (newSecret: ICreateSecret): Promise<ISecret | undefined> => {
    try {
      const result = await api.post<ICreateSecret, ISecret>(`/api/v1/secrets`, newSecret, {}, {
        snackbar: true,
      })
      if (!result) return undefined
      loadData()
      return result
    } catch (error) {
      console.error("useSecrets: Error creating secret:", error)
      throw error
    }
  }, [api, loadData])

  const updateSecret = useCallback(async (id: string, updatedSecret: Partial<ISecret>): Promise<ISecret | undefined> => {
    try {
      const result = await api.put<Partial<ISecret>, ISecret>(`/api/v1/secrets/${id}`, updatedSecret, {}, {
        snackbar: true,
      })
      if (!result) return undefined
      loadData()
      return result
    } catch (error) {
      console.error("useSecrets: Error updating secret:", error)
      throw error
    }
  }, [api, loadData])

  const deleteSecret = useCallback(async (id: string): Promise<boolean> => {
    await api.delete(`/api/v1/secrets/${id}`, {}, {
      snackbar: true,
    })
    loadData()
    return true
  }, [api, loadData])

  return {
    data,
    secret,
    loadData,
    loadSecret,
    setSecret,
    createSecret,
    updateSecret,
    deleteSecret,
  }
}

export default useSecrets
