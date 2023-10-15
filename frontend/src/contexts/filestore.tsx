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
  filestoreConfig: IFileStoreConfig,
  filesLoading: boolean,
  onSetFilestorePath: (path: string) => void,
}

export const FilestoreContext = createContext<IFilestoreContext>({
  files: [],
  filesLoading: false,
  filestoreConfig: {
    user_prefix: '',
    folders: [],
  },
  onSetFilestorePath: () => {},
})

export const useFilestoreContext = (): IFilestoreContext => {
  const api = useApi()
  const account = useAccount()
  const {
    name,
    meta,
    params,
    navigate,
  } = useRouter()
  const [ initialized, setInitialized ] = useState(false)
  const [ files, setFiles ] = useState<IFileStoreItem[]>([])
  const [ filesLoading, setFilesLoading ] = useState(false)
  const [ filestoreConfig, setFilestoreConfig ] = useState<IFileStoreConfig>({
    user_prefix: '',
    folders: [],
  })

  const onSetFilestorePath = useCallback((path: string) => {
    const update: any = {}
    if(path) {
      update.path = path
    }
    navigate('files', update)
  }, [
    navigate,
  ])

  const loadFilestoreConfig = useCallback(async () => {
    const configResult = await api.get('/api/v1/filestore/config')
    if(!configResult) return
    setFilestoreConfig(configResult)
  }, [])

  const loadFiles = useCallback(async (path: string) => {
    setFilesLoading(true)
    try {
      const filesResult = await api.get('/api/v1/filestore/list', {
        params: {
          path,
        }
      })
      if(!filesResult) return
      setFiles(filesResult || [])
    } catch(e) {}
    setFilesLoading(false)
  }, [])


  useEffect(() => {
    if(!params.path) return
    if(!user) return
    loadFiles(route.params.path)
  }, [
    user,
    route.params.path,
  ])

  useEffect(() => {
    if(!account.user) return
    loadFilestoreConfig()
  }, [
    account.user,
  ])

  const contextValue = useMemo<IFilestoreContext>(() => ({
    files,
    filesLoading,
    filestoreConfig,
    onSetFilestorePath,
  }), [
    initialized,
    files,
    filesLoading,
    filestoreConfig,
    onSetFilestorePath,
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