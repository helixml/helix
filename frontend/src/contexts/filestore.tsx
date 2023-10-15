import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'

import {
  IFileStoreItem,
  IFileStoreConfig,
} from '../types'

export interface IFilestoreContext {
  files: IFileStoreItem[],
  config: IFileStoreConfig,
  loading: boolean,
  onSetPath: (path: string) => void,
}

export const FilestoreContext = createContext<IFilestoreContext>({
  files: [],
  loading: false,
  config: {
    user_prefix: '',
    folders: [],
  },
  onSetPath: () => {},
})

export const useFilestoreContext = (): IFilestoreContext => {
  const api = useApi()
  const account = useAccount()
  const {
    params,
    navigate,
  } = useRouter()
  const [ files, setFiles ] = useState<IFileStoreItem[]>([])
  const [ loading, setLoading ] = useState(false)
  const [ config, setConfig ] = useState<IFileStoreConfig>({
    user_prefix: '',
    folders: [],
  })

  const onSetPath = useCallback((path: string) => {
    const update: any = {}
    if(path) {
      update.path = path
    }
    navigate('files', update)
  }, [
    navigate,
  ])

  const loadConfig = useCallback(async () => {
    const configResult = await api.get('/api/v1/filestore/config')
    if(!configResult) return
    setConfig(configResult)
  }, [])

  const loadFiles = useCallback(async (path: string) => {
    setLoading(true)
    try {
      const filesResult = await api.get('/api/v1/filestore/list', {
        params: {
          path,
        }
      })
      if(!filesResult) return
      setFiles(filesResult || [])
    } catch(e) {}
    setLoading(false)
  }, [])


  useEffect(() => {
    if(!params.path) return
    if(!account.user) return
    loadFiles(params.path)
  }, [
    account.user,
    params.path,
  ])

  useEffect(() => {
    if(!account.user) return
    loadConfig()
  }, [
    account.user,
  ])

  const contextValue = useMemo<IFilestoreContext>(() => ({
    files,
    loading,
    config,
    onSetPath,
  }), [
    files,
    loading,
    config,
    onSetPath,
  ])

  return contextValue
}

export const FilestoreContextProvider: FC = ({ children }) => {
  const value = useFilestoreContext()
  return (
    <FilestoreContext.Provider value={ value }>
      { children }
    </FilestoreContext.Provider>
  )
}