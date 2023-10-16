import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'

import {
  IFileStoreItem,
  IFileStoreConfig,
  IFileStoreBreadcrumb,
} from '../types'

export interface IFilestoreContext {
  loading: boolean,
  files: IFileStoreItem[],
  config: IFileStoreConfig,
  path: string,
  breadcrumbs: IFileStoreBreadcrumb[],
  onUpload: (path: string, files: File[]) => Promise<void>,
  onSetPath: (path: string) => void,
}

export const FilestoreContext = createContext<IFilestoreContext>({
  loading: false,
  files: [],
  config: {
    user_prefix: '',
    folders: [],
  },
  path: '',
  breadcrumbs: [],
  onUpload: async () => {},
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

  const path = useMemo(() => {
    return params.path || '/'
  }, [
    params.path,
  ])

  const breadcrumbs = useMemo(() => {
    const parts = path.split('/')
    let currentChunks: string[] = []
    return parts
      .filter(p => p ? true : false)
      .map((p: string): IFileStoreBreadcrumb => {
        currentChunks.push(p)
        const breadcrumb = {
          path: '/' + currentChunks.join('/'),
          title: p,
        }
        return breadcrumb
      })
  }, [
    path,
  ])

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
      if(filesResult) {
        setFiles(filesResult || [])
      }
    } catch(e) {}
    setLoading(false)
  }, [])

  const onUpload = useCallback(async (path: string, files: File[]) => {
    setLoading(true)
    try {
      const formData = new FormData()
      files.forEach((file) => {
        formData.append("files", file)
      })
      await api.post('/api/v1/filestore/upload', formData, {
        params: {
          path,
        }
      })
    } catch(e) {}
    setLoading(false)
  }, [])


  useEffect(() => {
    if(!account.user) return
    loadFiles(params.path || '/')
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
    loading,
    files,
    config,
    path,
    breadcrumbs,
    onUpload,
    onSetPath,
  }), [
    loading,
    files,
    config,
    path,
    breadcrumbs,
    onUpload,
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