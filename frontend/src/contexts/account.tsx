import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import bluebird from 'bluebird'
import Keycloak from 'keycloak-js'
import useApi from '../hooks/useApi'
import useSnackbar from '../hooks/useSnackbar'
import useLoading from '../hooks/useLoading'
import {extractErrorMessage} from '../hooks/useErrorCallback'

import {
  IUser,
} from '../types'

const REALM = 'lilypad'
const KEYCLOAK_URL = '/auth/'
const CLIENT_ID = 'frontend'

export interface IAccountContext {
  initialized: boolean,
  credits: number,
  user?: IUser,
  onLogin: () => void,
  onLogout: () => void,
}

export const AccountContext = createContext<IAccountContext>({
  initialized: false,
  credits: 0,
  onLogin: () => {},
  onLogout: () => {},
})

export const useAccountContext = (): IAccountContext => {
  const api = useApi()
  const snackbar = useSnackbar()
  const loading = useLoading()
  const [ initialized, setInitialized ] = useState(false)
  const [ user, setUser ] = useState<IUser>()
  const [ credits, setCredits ] = useState(0)
  const [ transactions, setTransactions ] = useState([])
  const [ jobs, setJobs ] = useState([])
  const [ modules, setModules ] = useState([])

  const keycloak = useMemo(() => {
    return new Keycloak({
      realm: REALM,
      url: KEYCLOAK_URL,
      clientId: CLIENT_ID,
    })
  }, [])

  const loadModules = useCallback(async () => {
    const result = await api.get('/api/v1/modules')
    if(!result) return
    setModules(result as any)
  }, [])

  const loadJobs = useCallback(async () => {
    const result = await api.get('/api/v1/jobs')
    if(!result) return
    setJobs(result as any)
  }, [])

  const loadTransactions = useCallback(async () => {
    const result = await api.get('/api/v1/transactions')
    if(!result) return
    setTransactions(result as any)
  }, [])

  const loadStatus = useCallback(async () => {
    const statusResult = await api.get('/api/v1/status')
    if(!statusResult) return
    setCredits(statusResult.credits)
  }, [])

  const loadAll = useCallback(async () => {
    await bluebird.all([
      loadModules(),
      loadJobs(),
      loadTransactions(),
      loadStatus(),
    ])
  }, [
    loadModules,
    loadJobs,
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

  const contextValue = useMemo<IAccountContext>(() => ({
    initialized,
    user,
    credits,
    onLogin,
    onLogout,
  }), [
    initialized,
    user,
    credits,
    onLogin,
    onLogout,
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