import bluebird from 'bluebird'
import { createContext, FC, useCallback, useEffect, useMemo, useState, useContext, ReactNode } from 'react'
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
  IProviderEndpoint,
} from '../types'

export interface IAccountContext {
  initialized: boolean,
  credits: number,
  admin: boolean,
  organizationTools: IOrganizationTools,
  isOrgAdmin: boolean,
  isOrgMember: boolean,
  user?: IKeycloakUser,
  userMeta?: { slug: string },  // User metadata including slug for GitHub-style URLs
  token?: string,
  tokenUrlEscaped?: string,
  loggingOut?: boolean,
  serverConfig: IServerConfig,
  userConfig: IUserConfig,
  appApiKeys: IApiKey[],
  mobileMenuOpen: boolean,
  setMobileMenuOpen: (val: boolean) => void,
  showLoginWindow: boolean,
  setShowLoginWindow: (val: boolean) => void,
  onLogin: () => void,
  onLogout: () => void,
  loadApiKeys: (queryParams?: Record<string, string>) => void,
  addAppAPIKey: (appId: string) => Promise<void>,
  loadAppApiKeys: (appId: string) => Promise<void>,
  models: IHelixModel[],
  hasImageModels: boolean,
  // an org aware navigate function that will prepend `org_` to the route name
  // and include the org_id in the params if we are currently looking at an org
  orgNavigate: (routeName: string, params?: Record<string, string | undefined>, queryParams?: Record<string, string>) => void,
  // Token expiry info for debugging
  tokenExpiryMinutes: number | null,
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
  appApiKeys: [],
  mobileMenuOpen: false,
  setMobileMenuOpen: () => { },
  showLoginWindow: false,
  setShowLoginWindow: () => { },
  onLogin: () => { },
  onLogout: () => { },
  loadApiKeys: () => { },
  addAppAPIKey: async () => { },
  loadAppApiKeys: async () => { },
  models: [],
  hasImageModels: false,
  orgNavigate: () => {},
  tokenExpiryMinutes: null,
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
  const [ userMeta, setUserMeta ] = useState<{ slug: string }>()
  const [ tokenExpiryMinutes, setTokenExpiryMinutes ] = useState<number | null>(null)
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
  const [appApiKeys, setAppApiKeys] = useState<IApiKey[]>([])
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
      if (statusResult.slug) {
        setUserMeta({ slug: statusResult.slug })
      }
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
    // This function is kept for backward compatibility but now relies on React Query
    // The actual loading is handled by the useGetUserAPIKeys hook
  }, [])

  const loadAppApiKeys = useCallback(async (appId: string) => {
    const result = await api.get<IApiKey[]>('/api/v1/api_keys', {
      params: {
        types: 'app',
        app_id: appId,
      },
    })
    if (!result) return
    setAppApiKeys(result)
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

      await loadAppApiKeys(appId)
    } catch (error) {
      console.error('Error adding API key:', error)
      snackbar.error('Failed to add API key')
    }
  }, [
    api,
    snackbar,
    apiKeys,
    loadAppApiKeys,
  ])

  const loadAll = useCallback(async () => {
    try {
      await bluebird.all([
        loadStatus(),
        loadServerConfig(),
      ])
    } catch (error) {
      console.error('Error loading data:', error)
      // Don't crash the app on data loading errors
    }
  }, [
    loadStatus,
    loadServerConfig,
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

    // Check for logout reason and show snackbar
    const logoutReason = localStorage.getItem('logout_reason')
    if (logoutReason) {
      snackbar.error(logoutReason)
      localStorage.removeItem('logout_reason')
    }

    try {
      const client = api.getApiClient()
      const authenticated = await client.v1AuthAuthenticatedList()
      if (authenticated.data.authenticated) {
        const userResponse = await client.v1AuthUserList()
        const user = userResponse.data as IKeycloakUser
        api.setToken(user.token)

        // Log token expiry for debugging
        try {
          if (user.token) {
            const tokenParts = user.token.split('.')
            if (tokenParts.length === 3) {
              const payload = JSON.parse(atob(tokenParts[1]))
              const expiry = new Date(payload.exp * 1000)
              const now = new Date()
              const minutesUntilExpiry = (expiry.getTime() - now.getTime()) / 1000 / 60
              console.log('[AUTH] Access token expiry:', {
                expiresAt: expiry.toISOString(),
                minutesUntilExpiry: Math.round(minutesUntilExpiry),
                exp: payload.exp,
                tokenType: 'access_token'
              })
            }
          }
          if (user.refresh_token) {
            const tokenParts = user.refresh_token.split('.')
            if (tokenParts.length === 3) {
              const payload = JSON.parse(atob(tokenParts[1]))
              const expiry = new Date(payload.exp * 1000)
              const now = new Date()
              const minutesUntilExpiry = (expiry.getTime() - now.getTime()) / 1000 / 60
              console.log('[AUTH] Refresh token expiry:', {
                expiresAt: expiry.toISOString(),
                minutesUntilExpiry: Math.round(minutesUntilExpiry),
                exp: payload.exp,
                tokenType: 'refresh_token'
              })
            }
          }
        } catch (e) {
          console.warn('[AUTH] Failed to decode token expiry:', e)
        }
        const win = (window as any)
        if (win.setUser) {
          win.setUser(user)
        }

        if (win.$crisp) {
          win.$crisp.push(['set', 'user:email', user?.email])
          win.$crisp.push(['set', 'user:nickname', user?.name])
        }

        setUser(user)

        // Check if token expires soon and refresh immediately if needed
        const checkAndRefreshToken = async () => {
          try {
            if (user.token) {
              const payload = JSON.parse(atob(user.token.split('.')[1]))
              const expiry = new Date(payload.exp * 1000)
              const now = new Date()
              const minutesUntilExpiry = (expiry.getTime() - now.getTime()) / 1000 / 60

              // If token expires in less than 4 minutes, refresh immediately!
              if (minutesUntilExpiry < 4) {
                console.log(`[AUTH] Token expires in ${Math.round(minutesUntilExpiry)} minutes - refreshing immediately!`)
                const innerClient = api.getApiClient()
                await innerClient.v1AuthRefreshCreate()
                const userResponse = await innerClient.v1AuthUserList()
                const refreshedUser = userResponse.data as IKeycloakUser

                setUser(Object.assign({}, refreshedUser, {
                  token: refreshedUser.token,
                  is_admin: admin,
                }))
                if (refreshedUser.token) {
                  api.setToken(refreshedUser.token)
                  console.log('[AUTH] Emergency refresh completed')
                }
              }
            }
          } catch (e) {
            console.error('[AUTH] Emergency refresh failed:', e)
          }
        }

        // Do immediate check/refresh if needed
        await checkAndRefreshToken()

        // Set up token refresh interval - using 4 minutes to stay well within
        // 15 minute implicit flow token expiry (accessTokenLifespanForImplicitFlow)
        const refreshInterval = setInterval(async () => {
          try {
            console.log('[AUTH] Token refresh starting...')
            const innerClient = api.getApiClient()
            await innerClient.v1AuthRefreshCreate()
            const userResponse = await innerClient.v1AuthUserList()
            const user = userResponse.data as IKeycloakUser

            // Log new token expiry after refresh
            try {
              if (user.token) {
                const payload = JSON.parse(atob(user.token.split('.')[1]))
                const expiry = new Date(payload.exp * 1000)
                console.log('[AUTH] Token refreshed! New access token expires:', expiry.toISOString())
              }
              if (user.refresh_token) {
                const payload = JSON.parse(atob(user.refresh_token.split('.')[1]))
                const expiry = new Date(payload.exp * 1000)
                console.log('[AUTH] Token refreshed! New refresh token expires:', expiry.toISOString())
              }
            } catch (e) {
              console.warn('[AUTH] Could not decode refreshed token expiry:', e)
            }

            setUser(Object.assign({}, user, {
              token: user.token,
              is_admin: admin,
            }))
            if (user.token) {
              api.setToken(user.token)
              console.log('[AUTH] Updated axios headers with new token')
            }
          } catch (e) {
            console.error('Error refreshing token:', e)

            // Try to get token expiry info for better error message
            let expiryInfo = ''
            try {
              const currentToken = api.getApiClient().securityData?.token
              if (currentToken) {
                const payload = JSON.parse(atob(currentToken.split('.')[1]))
                const expiry = new Date(payload.exp * 1000)
                expiryInfo = ` (token expired at ${expiry.toISOString()})`
              }
            } catch {}

            // Instead of immediately calling onLogin, clear interval and try one more time
            clearInterval(refreshInterval)
            // Only call onLogin if we're really unauthorized, not for network issues
            if ((e as any).response && (e as any).response.status === 401) {
              const reason = `Token refresh failed${expiryInfo} - session expired or server restarted`
              localStorage.setItem('logout_reason', reason)
              console.log('[AUTH] Logging out:', reason, e)
              onLogin()
            }
          }
        }, 120 * 1000) // 2 minutes (tokens expire in 5min, so refresh every 2min to be safe)

        // Clean up interval on component unmount
        return () => clearInterval(refreshInterval)
      }
    } catch (e) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)

      // Don't show snackbars for auth errors (401/403) to avoid scary red error messages
      // when tokens expire naturally. The auth error detection logic matches useApi.ts
      const isAuthError = (error: any): boolean => {
        // Check status code
        if (error.response?.status === 401 || error.response?.status === 403) {
          return true
        }

        // Check error message for common auth failure patterns
        const errorMsg = errorMessage.toLowerCase()
        const authErrorPatterns = [
          'unauthorized',
          'token expired',
          'token invalid',
          'authentication failed',
          'access denied',
          'forbidden',
          'not authenticated',
          'invalid token',
          'expired token'
        ]

        return authErrorPatterns.some(pattern => errorMsg.includes(pattern))
      }

      if (!isAuthError(e)) {
        snackbar.error(errorMessage)
      }
    } finally {
      loading.setLoading(false)
      setInitialized(true)
    }
  }, [])

  const orgNavigate = (routeName: string, params: Record<string, string | undefined> = {}, queryParams?: Record<string, string>) => {
    // Current menu type for triggering animations
    const currentResourceType = router.params.resource_type || 'chat'
    const isOrgRoute = routeName.startsWith('org_')
    const targetIsOrgRoute = isOrgRoute || params.org_id

    // Determine if we're transitioning between org and non-org routes or vice versa
    const isOrgTransition = (router.meta.menu === 'orgs' && !targetIsOrgRoute) ||
                           (router.meta.menu !== 'orgs' && targetIsOrgRoute)

    // Get the target route name and params
    let targetRouteName = routeName
    let targetParams = {...params}

    if(organizationTools.organization || params.org_id) {
      const useOrgID = params.org_id || organizationTools.organization?.name
      // Only prepend org_ if not already present
      if (!routeName.startsWith('org_')) {
        targetRouteName = `org_${routeName}`
      }
      targetParams = {
        ...params,
        org_id: useOrgID,
      }
    }

    // Add query params if provided
    const finalParams = queryParams ? { ...targetParams, ...queryParams } : targetParams

    // Navigate first, then trigger animations after a very small delay
    // This ensures components are mounted before animations run
    router.navigate(targetRouteName, finalParams)


  }

  useEffect(() => {
    initialize()
  }, [])

  useEffect(() => {
    try {
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
    userMeta,
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
    appApiKeys,
    onLogin,
    onLogout,
    loadApiKeys,
    addAppAPIKey,
    loadAppApiKeys,
    models,
    organizationTools,
    isOrgAdmin,
    isOrgMember,
    hasImageModels,
    orgNavigate,
  }
}

export const AccountContextProvider = ({ children }: { children: ReactNode }) => {
  const value = useAccountContext()
  return (
    <AccountContext.Provider value={value}>
      {children}
    </AccountContext.Provider>
  )
}
