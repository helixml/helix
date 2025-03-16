import bluebird from 'bluebird'
import { createContext, FC, useCallback, useEffect, useMemo, useState, useContext } from 'react'
import useApi from '../hooks/useApi'
import { extractErrorMessage } from '../hooks/useErrorCallback'
import useLoading from '../hooks/useLoading'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'
import useOrganizations, { IOrganizationTools, defaultOrganizationTools } from '../hooks/useOrganizations'

import {
  IApiKey,
  IHelixModel,
  IKeycloakUser,
  IServerConfig,
  IUserConfig,
  IProviderEndpoint
} from '../types'

export interface IAccountContext {
  initialized: boolean,
  credits: number,
  admin: boolean,
  organizationTools: IOrganizationTools,
  isOrgAdmin: boolean,
  isOrgMember: boolean,
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
  addAppAPIKey: (appId: string) => Promise<void>,
  models: IHelixModel[],
  hasImageModels: boolean,
  fetchModels: (provider?: string) => Promise<void>,
  providerEndpoints: IProviderEndpoint[],
  fetchProviderEndpoints: () => Promise<void>,
}

export const AccountContext = createContext<IAccountContext>({
  initialized: false,
  credits: 0,
  admin: false,
  organizationTools: defaultOrganizationTools,
  isOrgAdmin: false,
  isOrgMember: false,
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
  addAppAPIKey: async () => { },
  models: [],
  fetchModels: async () => { },
  providerEndpoints: [],
  fetchProviderEndpoints: async () => {},
  hasImageModels: false,
})

export const useAccount = () => {
  return useContext(AccountContext);
};

export const useAccountContext = (): IAccountContext => {
  const api = useApi()
  const snackbar = useSnackbar()
  const loading = useLoading()
  const router = useRouter()
  const organizationTools = useOrganizations()
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
  const [apiKeys, setApiKeys] = useState<IApiKey[]>([])
  const [models, setModels] = useState<IHelixModel[]>([])
  const [providerEndpoints, setProviderEndpoints] = useState<IProviderEndpoint[]>([])
  const [hasImageModels, setHasImageModels] = useState(false)

  const token = useMemo(() => {
    if (user && user.token) {
      return user.token
    } else {
      return ''
    }
  }, [
    user,
  ])

  const isOrgAdmin = useMemo(() => {
    if(admin) return true
    if(!organizationTools.organization) return false
    if(!user) return false
    return organizationTools.organization?.memberships?.some(
      m => m.user_id === user?.id && m.role === 'owner'
    ) ? true : false
  }, [
    admin,
    organizationTools.organization,
    user,
  ])
  
  const isOrgMember = useMemo(() => {
    if(admin) return true
    if(isOrgAdmin) return true
    if(!user) return false
    if(!organizationTools.organization) return false
    return organizationTools.organization?.memberships?.some(
      m => m.user_id === user?.id
    ) ? true : false
  }, [
    admin,
    organizationTools.organization,
    user,
    isOrgAdmin,
  ])

  const tokenUrlEscaped = useMemo(() => {
    if (!token) return '';
    return encodeURIComponent(token);
  }, [token]);

  const loadStatus = useCallback(async () => {
    try {
      const statusResult = await api.get('/api/v1/status')
      if (!statusResult) return
      setCredits(statusResult.credits)
      setAdmin(statusResult.admin)
      setUserConfig(statusResult.config)
      await organizationTools.loadOrganizations()
    } catch (error) {
      console.error('Error loading status:', error)
      // Don't propagate error - allow app to continue functioning
    }
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

  
  /**
   * Adds a new API key for the app
   */
  const addAppAPIKey = useCallback(async (appId: string) => {
    try {
      const res = await api.post('/api/v1/api_keys', {
        name: `api key ${apiKeys.length + 1}`,
        type: 'app',
        app_id: appId,
      }, {}, {
        snackbar: true,
      })
      
      if (!res) return
      
      snackbar.success('API Key added')
      
      // Reload API keys
      loadApiKeys({
        types: 'app',
        app_id: appId,
      })
    } catch (error) {
      console.error('Error adding API key:', error)
      snackbar.error('Failed to add API key')
    }
  }, [
    api,
    snackbar,
    apiKeys,
  ])

  const fetchProviderEndpoints = useCallback(async () => {
    const response = await api.get('/api/v1/provider-endpoints')
    if (!response) return
    setProviderEndpoints(response)
  }, [])

  const loadAll = useCallback(async () => {
    try {
      await bluebird.all([
        loadStatus(),
        loadServerConfig(),
        fetchProviderEndpoints(),
      ])
    } catch (error) {
      console.error('Error loading data:', error)
      // Don't crash the app on data loading errors
    }
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
        api.setToken(user.token)
        const win = (window as any)
        if (win.setUser) {
          win.setUser(user)
        }

        if (win.$crisp) {
          win.$crisp.push(['set', 'user:email', user?.email])
          win.$crisp.push(['set', 'user:nickname', user?.name])
        }

        setUser(user)

        // Set up token refresh interval - using 30 seconds instead of 10
        // to reduce server load and prevent potential race conditions
        const refreshInterval = setInterval(async () => {
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
            // Instead of immediately calling onLogin, clear interval and try one more time
            clearInterval(refreshInterval)
            // Only call onLogin if we're really unauthorized, not for network issues
            if ((e as any).response && (e as any).response.status === 401) {
              onLogin()
            }
          }
        }, 30 * 1000) // 30 seconds instead of 10
        
        // Clean up interval on component unmount
        return () => clearInterval(refreshInterval)
      }
    } catch (e) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      snackbar.error(errorMessage)
    } finally {
      loading.setLoading(false)
      setInitialized(true)
    }
  }, [])

  const fetchModels = useCallback(async (provider?: string) => {
    let loadingModels = false;
    try {
      loadingModels = true;
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
      
      // Check if there are any image models in the results
      const hasImage = modelData.some(model => model.type === 'image')
      setHasImageModels(hasImage)
    } catch (error) {
      console.error('Error fetching models:', error)
      setModels([])
    } finally {
      loadingModels = false;
    }
  }, [api])

  useEffect(() => {
    initialize()
  }, [])

  useEffect(() => {
    try {
      // Only fetch models if we haven't already done so
      if (models.length === 0) {
        fetchModels()
      }
      
      if (user) {
        loadAll()
      } else {
        loadServerConfig()
      }
    } catch (error) {
      console.error('Error in data loading useEffect:', error)
      // Ensure any loading states are cleared even on error
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
    addAppAPIKey,
    models,
    fetchModels,
    fetchProviderEndpoints,
    providerEndpoints,
    organizationTools,
    isOrgAdmin,
    isOrgMember,
    hasImageModels,
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