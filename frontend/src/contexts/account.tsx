import Keycloak from 'keycloak-js'
import { createContext, FC, useCallback, useEffect, useMemo, useState } from 'react'
import useApi from '../hooks/useApi'
import { extractErrorMessage } from '../hooks/useErrorCallback'
import useLoading from '../hooks/useLoading'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'

import {
  IApiKey,
  IHelixModel,
  IKeycloakUser,
  IServerConfig,
  IUserConfig
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
  tokenUrlEscaped?: string,
  loggingOut?: boolean,
  serverConfig: IServerConfig,
  userConfig: IUserConfig,
  apiKeys: IApiKey[],
  mobileMenuOpen: boolean,
  setMobileMenuOpen: (val: boolean) => void,
  showLoginWindow: boolean,
  setShowLoginWindow: (val: boolean) => void,
  onLogin: () => void,
  onLogout: () => void,
  loadApiKeys: (queryParams?: Record<string, string>) => void,
  models: IHelixModel[];
  fetchModels: () => Promise<void>;
}

export const AccountContext = createContext<IAccountContext>({
  initialized: false,
  credits: 0,
  admin: false,
  loggingOut: false,
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
  showLoginWindow: false,
  setShowLoginWindow: () => {},
  onLogin: () => {},
  onLogout: () => {},
  loadApiKeys: () => {},
  models: [],
  fetchModels: async () => {},
})

export const useAccountContext = (): IAccountContext => {
  const api = useApi()
  const snackbar = useSnackbar()
  const loading = useLoading()
  const router = useRouter()
  const [ admin, setAdmin ] = useState(false)
  const [ mobileMenuOpen, setMobileMenuOpen ] = useState(false)
  const [ showLoginWindow, setShowLoginWindow ] = useState(false)
  const [ initialized, setInitialized ] = useState(false)
  const [ user, setUser ] = useState<IKeycloakUser>()
  const [ credits, setCredits ] = useState(0)
  const [ loggingOut, setLoggingOut ] = useState(false)
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
  const [ models, setModels ] = useState<IHelixModel[]>([])

  const keycloak = useMemo(() => {
    return new Keycloak({
      realm: REALM,
      url: KEYCLOAK_URL,
      clientId: CLIENT_ID,
    })
  }, [])

  const token = useMemo(() => {
    if(user && user.token) {
      return user.token
    } else {
      return ''
    }
  }, [
    user,
  ])

  const tokenUrlEscaped = useMemo(() => {
    if (!token) return '';
    return encodeURIComponent(token);
  }, [token]);

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
  
  const loadApiKeys = useCallback(async (params: Record<string, string> = {}) => {
    const result = await api.get<IApiKey[]>('/api/v1/api_keys', {
      params,
    })
    if(!result) return
    setApiKeys(result)
  }, [])

  const loadAll = useCallback(async () => {
    await Promise.all([
      loadStatus(),
      loadServerConfig(),
    ])
  }, [
    loadStatus,
    loadServerConfig,
  ])

  const onLogin = useCallback(() => {
    keycloak.login()
  }, [
    keycloak,
  ])

  const onLogout = useCallback(() => {
    setLoggingOut(true)
    router.navigate('home')
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

        if(win.$crisp) {
          win.$crisp.push(['set', 'user:email', user?.email])
          win.$crisp.push(['set', 'user:nickname', user?.name])
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

  const fetchModels = useCallback(async () => {
    try {
      const response = await api.get('/v1/models')
      
      let modelData: IHelixModel[] = [];
      if (response && Array.isArray(response.data)) {
        modelData = response.data.map((m: any) => ({
          id: m.id,
          name: m.name || m.id,
          description: m.description || '',
          hide: m.hide || false,
          type: m.type || 'text',
        }));

        // Filter out hidden models
        modelData = modelData.filter(m => !m.hide);
      } else {
        console.error('Unexpected API response structure:', response)
      }

      setModels(modelData)
    } catch (error) {
      console.error('Error fetching models:', error)
      setModels([])
    }
  }, [api])

  useEffect(() => {
    initialize()
  }, [])

  useEffect(() => {
    fetchModels()
    if (user) {
      loadAll()
    } else {
      loadServerConfig()
    }
  }, [user])

  return {
    initialized,
    user,
    token,
    tokenUrlEscaped,
    admin,
    loggingOut,
    serverConfig,
    userConfig,
    mobileMenuOpen,
    setMobileMenuOpen,
    showLoginWindow,
    setShowLoginWindow,
    credits,
    apiKeys,
    onLogin,
    onLogout,
    loadApiKeys,
    models,
    fetchModels,
  }
}

export const AccountContextProvider: FC = ({ children }) => {
  const value = useAccountContext()
  return (
    <AccountContext.Provider value={ value }>
      { children }
    </AccountContext.Provider>
  )
}