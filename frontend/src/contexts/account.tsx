import bluebird from 'bluebird'
import { createContext, FC, useCallback, useEffect, useMemo, useState, useContext } from 'react'
import useApi from '../hooks/useApi'
import { useOrganizations } from '../hooks/useOrganisations'
import { extractErrorMessage } from '../hooks/useErrorCallback'
import useLoading from '../hooks/useLoading'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'

import {
  IApiKey,
  IHelixModel,
  IKeycloakUser,
  IServerConfig,
  IUserConfig,
  IProviderEndpoint
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
  models: IHelixModel[],
  fetchModels: (provider?: string) => Promise<void>,
  providerEndpoints: IProviderEndpoint[],
  fetchProviderEndpoints: () => Promise<void>,
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
  setMobileMenuOpen: () => { },
  showLoginWindow: false,
  setShowLoginWindow: () => { },
  onLogin: () => { },
  onLogout: () => { },
  loadApiKeys: () => { },
  models: [],
  fetchModels: async () => { },
  providerEndpoints: [],
  fetchProviderEndpoints: async () => { },
})

export const useAccount = () => {
  return useContext(AccountContext);
};

export const useAccountContext = (): IAccountContext => {
  const api = useApi()
  const organizations = useOrganizations()
  const snackbar = useSnackbar()
  const loading = useLoading()
  const router = useRouter()
  const [admin, setAdmin] = useState(false)
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const [showLoginWindow, setShowLoginWindow] = useState(false)
  const [initialized, setInitialized] = useState(false)
  const [user, setUser] = useState<IKeycloakUser>()
  const [credits, setCredits] = useState(0)
  const [loggingOut, setLoggingOut] = useState(false)
  const [userConfig, setUserConfig] = useState<IUserConfig>({})
  const [serverConfig, setServerConfig] = useState<IServerConfig>({
    filestore_prefix: '',
    stripe_enabled: false,
    sentry_dsn_frontend: '',
    google_analytics_frontend: '',
    eval_user_id: '',
    tools_enabled: true,
    apps_enabled: true,
  })
  const [apiKeys, setApiKeys] = useState<IApiKey[]>([])
  const [models, setModels] = useState<IHelixModel[]>([])
  const [providerEndpoints, setProviderEndpoints] = useState<IProviderEndpoint[]>([])
  const [latestVersion, setLatestVersion] = useState<string>()

  const token = useMemo(() => {
    if (user && user.token) {
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
    if (!statusResult) return
    setCredits(statusResult.credits)
    setAdmin(statusResult.admin)
    setUserConfig(statusResult.config)
  }, [])

  const loadServerConfig = useCallback(async () => {
    const configResult = await api.get('/api/v1/config')
    if (!configResult) return
    setServerConfig(configResult)
  }, [])

  const loadApiKeys = useCallback(async (params: Record<string, string> = {}) => {
    const result = await api.get<IApiKey[]>('/api/v1/api_keys', {
      params,
    })
    if (!result) return
    setApiKeys(result)
  }, [])

  const fetchProviderEndpoints = useCallback(async () => {
    const response = await api.get('/api/v1/provider-endpoints')
    if (!response) return
    setProviderEndpoints(response)
  }, [])

  const loadAll = useCallback(async () => {
    await bluebird.all([
      loadStatus(),
      loadServerConfig(),
      fetchProviderEndpoints(),
      organizations.loadData(),
    ])
  }, [
    loadStatus,
    loadServerConfig,
    fetchProviderEndpoints,
  ])

  const onLogin = useCallback(async () => {
    try {
      fetch(`/api/v1/auth/login`, {
        method: 'POST',
        body: JSON.stringify({
          redirect_uri: window.location.href,
        }),
      })
        .then(response => {
          console.log(response);
          if (response.redirected) {
            window.location.href = response.url;
          }
        });
    } catch (e) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      snackbar.error(errorMessage)
    }
  }, [
    api,
  ])

  const onLogout = useCallback(() => {
    setLoggingOut(true)
    try {
      fetch(`/api/v1/auth/logout`, {
        method: 'POST',
      })
        .then(response => {
          console.log(response);
          if (response.redirected) {
            window.location.href = response.url;
          }
        });
    } catch (e) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      snackbar.error(errorMessage)
    }
  }, [
    api,
  ])

  const initialize = useCallback(async () => {
    loading.setLoading(true)
    try {
      const client = api.getApiClient()
      const authenticated = await client.v1AuthAuthenticatedList()
      if (authenticated.data.authenticated) {
        const userResponse = await client.v1AuthUserList()
        const user = userResponse.data as IKeycloakUser
        const win = (window as any)
        if (win.setUser) {
          win.setUser(user)
        }

        if (win.$crisp) {
          win.$crisp.push(['set', 'user:email', user?.email])
          win.$crisp.push(['set', 'user:nickname', user?.name])
        }

        setUser(user)
        if (user.token) {
          api.setToken(user.token)
        }

        // Set up token refresh interval
        setInterval(async () => {
          try {
            const innerClient = api.getApiClient()
            await innerClient.v1AuthRefreshCreate()
            const userResponse = await innerClient.v1AuthUserList()
            const user = userResponse.data as IKeycloakUser
            setUser(Object.assign({}, user, {
              token: user.token,
              is_admin: admin,
            }))
            if (user.token) {
              api.setToken(user.token)
            }
          } catch (e) {
            console.error('Error refreshing token:', e)
            onLogin()
          }
        }, 10 * 1000)
      }
    } catch (e) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      snackbar.error(errorMessage)
    }
    loading.setLoading(false)
    setInitialized(true)
  }, [])

  const fetchModels = useCallback(async (provider?: string) => {
    try {
      const url = provider ? `/v1/models?provider=${encodeURIComponent(provider)}` : '/v1/models'
      const response = await api.get(url)

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
    fetchProviderEndpoints,
    providerEndpoints,
  }
}

export const AccountContextProvider: FC = ({ children }) => {
  const value = useAccountContext()
  return (
    <AccountContext.Provider value={value}>
      {children}
    </AccountContext.Provider>
  )
}