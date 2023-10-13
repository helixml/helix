import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import bluebird from 'bluebird'
import Keycloak from 'keycloak-js'
import ReconnectingWebSocket from 'reconnecting-websocket'
import { useQueryParams } from 'hookrouter'
import useApi from '../hooks/useApi'
import useSnackbar from '../hooks/useSnackbar'
import useLoading from '../hooks/useLoading'
import {extractErrorMessage} from '../hooks/useErrorCallback'

import {
  IUser,
  IJob,
  IModule,
  IBalanceTransfer,
  IFileStoreItem,
  IFileStoreConfig,
} from '../types'

const REALM = 'lilypad'
const KEYCLOAK_URL = '/auth/'
const CLIENT_ID = 'frontend'

export interface IAccountContext {
  initialized: boolean,
  credits: number,
  user?: IUser,
  jobs: IJob[],
  modules: IModule[],
  files: IFileStoreItem[],
  filestoreConfig: IFileStoreConfig,
  transactions: IBalanceTransfer[],
  onLogin: () => void,
  onLogout: () => void,
  onSetFilestorePath: (path: string) => void,
}

export const AccountContext = createContext<IAccountContext>({
  initialized: false,
  credits: 0,
  jobs: [],
  modules: [],
  files: [],
  filestoreConfig: {
    folders: [],
  },
  transactions: [],
  onLogin: () => {},
  onLogout: () => {},
  onSetFilestorePath: () => {},
})

export const useAccountContext = (): IAccountContext => {
  const api = useApi()
  const snackbar = useSnackbar()
  const loading = useLoading()
  const [ queryParams, setQueryParams ] = useQueryParams()
  const [ initialized, setInitialized ] = useState(false)
  const [ user, setUser ] = useState<IUser>()
  const [ credits, setCredits ] = useState(0)
  const [ transactions, setTransactions ] = useState<IBalanceTransfer[]>([])
  const [ files, setFiles ] = useState<IFileStoreItem[]>([])
  const [ filestoreConfig, setFilestoreConfig ] = useState<IFileStoreConfig>({
    folders: [],
  })
  const [ jobs, setJobs ] = useState<IJob[]>([])
  const [ modules, setModules ] = useState<IModule[]>([])

  const keycloak = useMemo(() => {
    return new Keycloak({
      realm: REALM,
      url: KEYCLOAK_URL,
      clientId: CLIENT_ID,
    })
  }, [])

  const onSetFilestorePath = useCallback((path: string) => {
    const update: any = {}
    if(path) {
      update.path = path
    }
    setQueryParams(update)
  }, [
    setQueryParams,
  ])

  const loadModules = useCallback(async () => {
    const result = await api.get<IModule[]>('/api/v1/modules')
    if(!result) return
    setModules(result)
  }, [])

  const loadJobs = useCallback(async () => {
    const result = await api.get<IJob[]>('/api/v1/jobs')
    if(!result) return
    setJobs(result)
  }, [])

  const loadTransactions = useCallback(async () => {
    const result = await api.get<IBalanceTransfer[]>('/api/v1/transactions')
    if(!result) return
    setTransactions(result)
  }, [])

  const loadStatus = useCallback(async () => {
    const statusResult = await api.get('/api/v1/status')
    if(!statusResult) return
    setCredits(statusResult.credits)
  }, [])

  const loadFilestoreConfig = useCallback(async () => {
    const configResult = await api.get('/api/v1/filestore/config')
    if(!configResult) return
    setFilestoreConfig(configResult)
  }, [])

  const loadFiles = useCallback(async (path: string) => {
    const filesResult = await api.get('/api/v1/filestore/list', {
      params: {
        path,
      }
    })
    if(!filesResult) return
    setFiles(filesResult || [])
  }, [])

  const loadAll = useCallback(async () => {
    await bluebird.all([
      loadModules(),
      loadFilestoreConfig(),
      loadJobs(),
      loadTransactions(),
      loadStatus(),
    ])
  }, [
    loadModules,
    loadJobs,
    loadFilestoreConfig,
    loadTransactions,
    loadStatus,
  ])

  const onLogin = useCallback(() => {
    keycloak.login()
  }, [
    keycloak,
  ])

  const onLogout = useCallback(() => {
    keycloak.logout()
  }, [
    keycloak,
  ])

  const initialize = useCallback(async () => {
    loading.setLoading(true)
    try {
      const authenticated = await keycloak.init({
        onLoad: 'check-sso',
        pkceMethod: 'S256',
      })
      if(authenticated) {
        if(!keycloak.tokenParsed?.sub) throw new Error(`no user id found from keycloak`)
        if(!keycloak.tokenParsed?.preferred_username) throw new Error(`no user email found from keycloak`)
        if(!keycloak.token) throw new Error(`no user token found from keycloak`)
        const user: IUser = {
          id: keycloak.tokenParsed?.sub,
          email: keycloak.tokenParsed?.preferred_username, 
          token: keycloak.token,
        }
        api.setToken(keycloak.token)
        setUser(user)
        setInterval(async () => {
          try {
            const updated = await keycloak.updateToken(10)
            if(updated && keycloak.token) {
              api.setToken(keycloak.token)
              setUser(Object.assign({}, user, {
                token: keycloak.token,
              }))
            }
          } catch(e) {
            keycloak.login()
          }
        }, 10 * 1000)
      }
    } catch(e) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      snackbar.error(errorMessage)
    }
    loading.setLoading(false)
    setInitialized(true)
  }, [])

  useEffect(() => {
    initialize()
  }, [])

  useEffect(() => {
    if(!user) return
    loadAll()
  }, [
    user,
  ])

  useEffect(() => {
    if(!user?.token) return
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
    const wsHostname = window.location.hostname
    const url = `${wsProtocol}//${wsHostname}/api/v1/ws?access_token=${user?.token}`
    const rws = new ReconnectingWebSocket(url)
    rws.addEventListener('message', (event) => {
      const parsedData = JSON.parse(event.data)
      console.dir(parsedData)
      if(parsedData.type === 'job' && parsedData.job) {
        const newJob: IJob = parsedData.job
        setJobs(jobs => jobs.map(existingJob => {
          if(existingJob.id === newJob.id) return newJob
          return existingJob
        }))
      }
    })
    return () => rws.close()
  }, [
    user?.token,
  ])

  useEffect(() => {
    if(!queryParams.path) return
    if(!user) return
    loadFiles(queryParams.path)
  }, [
    user,
    queryParams.path,
  ])

  const contextValue = useMemo<IAccountContext>(() => ({
    initialized,
    user,
    credits,
    jobs,
    modules,
    files,
    filestoreConfig,
    transactions,
    onLogin,
    onLogout,
    onSetFilestorePath,
  }), [
    initialized,
    user,
    credits,
    jobs,
    modules,
    files,
    filestoreConfig,
    transactions,
    onLogin,
    onLogout,
    onSetFilestorePath,
  ])

  return contextValue
}

export const AccountContextProvider: FC = ({ children }) => {
  const value = useAccountContext()
  return (
    <AccountContext.Provider value={ value }>
      { children }
    </AccountContext.Provider>
  )
}