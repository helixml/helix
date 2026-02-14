import bluebird from 'bluebird'
import { createContext, FC, useCallback, useEffect, useMemo, useRef, useState, useContext, ReactNode } from 'react'
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
  // Dismiss onboarding for this page session (resets on refresh)
  dismissOnboarding: () => void,
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
  dismissOnboarding: () => {},
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
      // Use redirect: 'manual' to prevent fetch from following cross-origin redirects
      // which would fail due to CORS when redirecting to Keycloak
      const response = await fetch(`/api/v1/auth/login`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          redirect_uri: window.location.href,
        }),
        redirect: 'manual',
      });

      // With redirect: 'manual', a 302 response becomes type: 'opaqueredirect'
      // We can't read the Location header, but response.url contains the redirect target
      if (response.type === 'opaqueredirect') {
        // For opaque redirects, we need to get the URL from the server differently
        // The response.url will be empty for opaque redirects, so we make another request
        // to get the redirect URL as JSON
        const urlResponse = await fetch(`/api/v1/auth/login?get_url=true`, {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify({
            redirect_uri: window.location.href,
          }),
        });
        if (urlResponse.ok) {
          const data = await urlResponse.json();
          if (data.url) {
            window.location.href = data.url;
          }
        }
      } else if (response.redirected) {
        window.location.href = response.url;
      }
    } catch (e) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      snackbar.error(errorMessage)
    }
  }, [
    api,
    snackbar,
  ])

  const onLogout = useCallback(async () => {
    setLoggingOut(true)
    setUser(undefined)

    try {
      // Use redirect: 'manual' to prevent fetch from following cross-origin redirects
      // which would fail due to CORS when redirecting to Keycloak
      const response = await fetch(`/api/v1/auth/logout`, {
        method: 'POST',
        redirect: 'manual',
      });

      // With redirect: 'manual', a 302 response becomes type: 'opaqueredirect'
      // We can't read the Location header, so we make another request to get the redirect URL as JSON
      if (response.type === 'opaqueredirect') {
        const redirectUri = encodeURIComponent(window.location.origin);
        const urlResponse = await fetch(`/api/v1/auth/logout?get_url=true&redirect_uri=${redirectUri}`, {
          method: 'POST',
        });
        if (urlResponse.ok) {
          const data = await urlResponse.json();
          if (data.url) {
            window.location.href = data.url;
          }
        }
      } else if (response.redirected) {
        window.location.href = response.url;
      }
    } catch (e) {
      const errorMessage = extractErrorMessage(e)
      console.error(errorMessage)
      snackbar.error(errorMessage)
    }
  }, [
    snackbar,
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

        const win = (window as any)
        if (win.setUser) {
          win.setUser(user)
        }

        if (win.$crisp) {
          win.$crisp.push(['set', 'user:email', user?.email])
          if (user?.name) {
            win.$crisp.push(['set', 'user:nickname', user?.name])
          }
        }

        setUser(user)
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

  // Redirect to waitlist page immediately if user is waitlisted (before loading anything else)
  useEffect(() => {
    if (!initialized || !user) return
    if (router.name === 'waitlist') return
    if (!user.waitlisted) return

    router.navigateReplace('waitlist')
  }, [initialized, user, router.name])

  // Dismiss onboarding for this page session (in-memory, resets on refresh)
  const onboardingDismissedRef = useRef(false)
  const dismissOnboarding = useCallback(() => {
    onboardingDismissedRef.current = true
  }, [])

  // Redirect to onboarding if user hasn't completed it and has no orgs
  useEffect(() => {
    if (!initialized || !user) return
    // Don't redirect if user is waitlisted (waitlist takes priority)
    if (user.waitlisted) return
    // Don't redirect if already on onboarding page
    if (router.name === 'onboarding') return
    // Don't redirect if onboarding is already completed
    if (user.onboarding_completed) return
    // Don't redirect if user dismissed onboarding this session
    if (onboardingDismissedRef.current) return
    // Don't redirect if user already has organizations (not a fresh account)
    if (organizationTools.organizations.length > 0) return
    // Wait for organizations to be loaded before deciding
    if (organizationTools.loading) return

    router.navigateReplace('onboarding')
  }, [initialized, user, router.name, organizationTools.organizations.length, organizationTools.loading])

  return {
    initialized,
    user,
    userMeta,
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
    dismissOnboarding,
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
