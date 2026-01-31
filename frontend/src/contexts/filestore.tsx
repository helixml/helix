import React, { useEffect, createContext, useMemo, useState, useCallback, ReactNode } from 'react'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import { getRelativePath } from '../utils/filestore'

import {
  IFileStoreItem,
  IFileStoreConfig,
  IFileStoreBreadcrumb,
} from '../types'

export interface IFilestoreUploadProgress {
  percent: number,
  totalBytes: number,
  uploadedBytes: number,
}

export interface IFilestoreContext {
  loading: boolean,
  readonly: boolean,
  uploadProgress?: IFilestoreUploadProgress,
  files: IFileStoreItem[],
  config: IFileStoreConfig,
  path: string,
  breadcrumbs: IFileStoreBreadcrumb[],
  setPath: (path: string) => void,
  loadFiles: (path: string) => Promise<void>,
  // same as loadFiles but just returns the files
  // rather than setting the files in the state
  getFiles: (path: string) => Promise<IFileStoreItem[]>,
  upload: (path: string, files: File[]) => Promise<boolean>,
  createFolder: (path: string) => Promise<boolean>,
  rename: (path: string, newName: string) => Promise<boolean>,
  del: (path: string) => Promise<boolean>,
}

export const FilestoreContext = createContext<IFilestoreContext>({
  loading: false,
  readonly: false,
  files: [],
  config: {
    user_prefix: '',
    folders: [],
  },
  path: '',
  breadcrumbs: [],
  setPath: () => {},
  loadFiles: async () => {},
  getFiles: async () => [],
  upload: async () => true,
  createFolder: async () => true,
  rename: async () => true,
  del: async () => true,
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
  const [ uploadProgress, setUploadProgress ] = useState<IFilestoreUploadProgress>()
  const [ config, setConfig ] = useState<IFileStoreConfig>({
    user_prefix: '',
    folders: [],
  })

  const path = useMemo(() => {
    return params.path || '/'
  }, [
    params.path,
  ])

  const readonly = useMemo(() => {
    const pathParts = path.split('/')
    // this means we are in the root folder which is writable
    if(pathParts.length < 2) return false
    const rootFolder = config.folders.find(folder => folder.name == pathParts[1])
    // we are in a custom folder which is writable
    if(!rootFolder) return false
    // we let the folder itself decide
    return rootFolder.readonly
  }, [
    path,
    config,
  ])

  const breadcrumbs = useMemo(() => {
    const parts = path.split('/')
    let currentChunks: string[] = []
    const folders = parts
      .filter(p => p ? true : false)
      .map((p: string): IFileStoreBreadcrumb => {
        currentChunks.push(p)
        const breadcrumb = {
          path: '/' + currentChunks.join('/'),
          title: p,
        }
        return breadcrumb
      })

    const root = {
      path: '/',
      title: 'files'
    }

    return [root].concat(folders)
  }, [
    path,
  ])

  const setPath = useCallback((path: string) => {
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

  const getFiles = useCallback(async (path: string): Promise<IFileStoreItem[]> => {
    try {
      const filesResult = await api.get<IFileStoreItem[]>('/api/v1/filestore/list', {
        params: {
          path,
        }
      })
      return filesResult || []
    } catch(e) {
      console.error(e)
      return []
    }
  }, [])

  const loadFiles = useCallback(async (path: string, withLoading = false) => {
    if(withLoading) setLoading(true)
    const files = await getFiles(path)
    setFiles(files)
    if(withLoading) setLoading(false)
  }, [
    getFiles,
  ])

  const upload = useCallback(async (path: string, files: File[]) => {
    let result = false
    
    console.log("xxx upload called", path, files);
    
    setUploadProgress({
      percent: 0,
      totalBytes: 0,
      uploadedBytes: 0,
    })
    
    try {
      const formData = new FormData()
      files.forEach((file) => {
        formData.append("files", file)
      })
      
      // Remove user prefix from path if config is available and has user_prefix
      const uploadPath = config && config.user_prefix ? getRelativePath(config, { path } as IFileStoreItem) : path
      
      try {
        await api.post('/api/v1/filestore/upload', formData, {
          params: {
            path: uploadPath,
          },
          onUploadProgress: (progressEvent) => {
            const percent = progressEvent.total && progressEvent.total > 0 ?
              Math.round((progressEvent.loaded * 100) / progressEvent.total) :
              0
            setUploadProgress({
              percent,
              totalBytes: progressEvent.total || 0,
              uploadedBytes: progressEvent.loaded || 0,
            })
          }
        })
      } catch (error: any) {
        const errorMessage = error.response?.data?.error || 'Failed to upload files';
        console.error(errorMessage);
        throw error;
      }
      result = true
    } catch(e) {}
    setUploadProgress(undefined)
    return result
  }, [
    loadFiles,
    config,
  ])

  const rename = useCallback(async (oldName: string, newName: string) => {
    const result = await api.put('/api/v1/filestore/rename', null, {
      params: {
        path: [ path, oldName ].join('/'),
        new_path: [ path, newName ].join('/'),
      },
    }, {
      loading: true,
    })
    return result ? true : false
  }, [
    path,
  ])

  const createFolder = useCallback(async (name: string) => {
    const result = await api.post('/api/v1/filestore/folder', null, {
      params: {
        path: [ path, name ].join('/'),
      },
    }, {
      loading: true,
    })
    return result ? true : false
  }, [
    path,
  ])

  const del = useCallback(async (name: string) => {
    const result = await api.delete('/api/v1/filestore/delete', {
      params: {
        path: [ path, name ].join('/'),
      },
    }, {
      loading: true,
    })
    return result ? true : false
  }, [
    path,
  ])

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

  return {
    loading,
    readonly,
    uploadProgress,
    files,
    config,
    path,
    breadcrumbs,
    setPath,
    loadFiles,
    getFiles,
    upload,
    createFolder,
    rename,
    del,
  }
}

export const FilestoreContextProvider = ({ children }: { children: ReactNode }) => {
  const value = useFilestoreContext()
  return (
    <FilestoreContext.Provider value={ value }>
      { children }
    </FilestoreContext.Provider>
  )
}