import React, { FC, useEffect, createContext, useMemo, useState, useCallback } from 'react'
import bluebird from 'bluebird'
import Keycloak from 'keycloak-js'
import useApi from '../hooks/useApi'
import useSnackbar from '../hooks/useSnackbar'
import useLoading from '../hooks/useLoading'
import { extractErrorMessage } from '../hooks/useErrorCallback'

import {
  IKeycloakUser,
  ISession,
  IApiKey,
  IServerConfig,
  IUserConfig,
} from '../types'

const REALM = 'helix'
const KEYCLOAK_URL = '/auth/'
const CLIENT_ID = 'frontend'

export interface IAccountContext {
  initialized: boolean,
  credits: number,
  admin: boolean,
  user?: IKeycloakUser,
  token?: string,
  serverConfig: IServerConfig,
  userConfig: IUserConfig,
  apiKeys: IApiKey[],
  mobileMenuOpen: boolean,
  setMobileMenuOpen: (val: boolean) => void,
  onLogin: () => void,
  onLogout: () => void,
}

export const AccountContext = createContext<IAccountContext>({
  initialized: false,
  credits: 0,
  admin: false,
  serverConfig: {
    filestore_prefix: '',
    stripe_enabled: false,
    sentry_dsn_frontend: '',
    google_analytics_frontend: '',
    eval_user_id: '',
    tools_enabled: true,
    apps_enabled: true,
  },
  userConfig: {},
  apiKeys: [],
  mobileMenuOpen: false,
  setMobileMenuOpen: () => {},
  onLogin: () => {},
  onLogout: () => {},
})

export const useAccountContext = (): IAccountContext => {
  const api = useApi()
  const snackbar = useSnackbar()
  const loading = useLoading()
  const [ admin, setAdmin ] = useState(false)
  const [ mobileMenuOpen, setMobileMenuOpen ] = useState(false)
  const [ initialized, setInitialized ] = useState(false)
  const [ user, setUser ] = useState<IKeycloakUser>()
  const [ credits, setCredits ] = useState(0)
  const [ userConfig, setUserConfig ] = useState<IUserConfig>({})
  const [ serverConfig, setServerConfig ] = useState<IServerConfig>({
    filestore_prefix: '',
    stripe_enabled: false,
    sentry_dsn_frontend: '',
    google_analytics_frontend: '',
    eval_user_id: '',
    tools_enabled: true,
    apps_enabled: true,
  })
  const [ apiKeys, setApiKeys ] = useState<IApiKey[]>([])

  const keycloak = useMemo(() => {
    return new Keycloak({
      realm: REALM,
      url: KEYCLOAK_URL,
      clientId: CLIENT_ID,
    })
  }, [])

  const token = useMemo(() => {
    if(apiKeys && apiKeys.length > 0) {
      return apiKeys[0].key
    } else if(user && user.token) {
      return user.token
    } else {
      return ''
    }
  }, [
    user,
    apiKeys,
  ])

  console.log('--------------------------------------------')
  console.dir(token)

  const loadStatus = useCallback(async () => {
    const statusResult = await api.get('/api/v1/status')
    if(!statusResult) return
    setCredits(statusResult.credits)
    setAdmin(statusResult.admin)
    setUserConfig(statusResult.config)
  }, [])

  const loadServerConfig = useCallback(async () => {
    const configResult = await api.get('/api/v1/config')
    if(!configResult) return
    setServerConfig(configResult)
  }, [])
  
  const loadApiKeys = useCallback(async () => {
    const result = await api.get<IApiKey[]>('/api/v1/api_keys')
    if(!result) return
    setApiKeys(result)
  }, [])

  const loadAll = useCallback(async () => {
    await bluebird.all([
      loadStatus(),
      loadServerConfig(),
      loadApiKeys(),
    ])
  }, [
    loadStatus,
    loadServerConfig,
    loadApiKeys,
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
        const user: IKeycloakUser = {
          id: keycloak.tokenParsed?.sub,
          email: keycloak.tokenParsed?.preferred_username, 
          token: keycloak.token,
          name: keycloak.tokenParsed?.name,
        }
        const win = (window as any)
        if(win.setUser) {
          win.setUser(user)
        }

        win.$crisp.push(['set', 'user:email', user?.email])
        win.$crisp.push(['set', 'user:nickname', user?.name])

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
    if(!user) {
      loadServerConfig()
    } else {
      loadAll()
    }
    
  }, [
    user,
  ])

  const contextValue = useMemo<IAccountContext>(() => ({
    initialized,
    user,
    token,
    admin,
    serverConfig,
    userConfig,
    mobileMenuOpen,
    setMobileMenuOpen,
    credits,
    apiKeys,
    onLogin,
    onLogout,
  }), [
    initialized,
    user,
    token,
    admin,
    serverConfig,
    userConfig,
    mobileMenuOpen,
    setMobileMenuOpen,
    credits,
    apiKeys,
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