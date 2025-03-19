import React, { FC, useEffect, createContext, useMemo, useState, useCallback, useRef } from 'react'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'
import { throttle } from 'lodash'

import {
  IApp,
  IAppUpdate,
  IAppConfig,
  IAppSource,
  IGithubStatus,
  APP_SOURCE_GITHUB,
  APP_SOURCE_HELIX,
  SESSION_TYPE_TEXT,
} from '../types'

// Apps context interface that mirrors the return value of the useApps hook
export interface IAppsContext {
  apps: IApp[],
  app: IApp | undefined,
  githubStatus: IGithubStatus | undefined,
  helixApps: IApp[],
  githubApps: IApp[],
  loadApps: (query?: IAppsQuery) => Promise<void>,
  loadApp: (id: string, showErrors?: boolean) => Promise<void>,
  setApp: React.Dispatch<React.SetStateAction<IApp | undefined>>,
  loadGithubStatus: (pageURL: string) => Promise<void>,
  loadGithubRepos: () => Promise<void>,
  createGithubApp: (repo: string) => Promise<IApp | null>,
  createEmptyHelixApp: () => Promise<IApp | undefined>,
  createOrgApp: () => Promise<IApp | undefined>,
  createApp: (app_source: IAppSource, config: IAppConfig) => Promise<IApp | undefined>,
  updateApp: (id: string, updatedApp: IAppUpdate) => Promise<IApp | undefined>,
  deleteApp: (id: string) => Promise<boolean | undefined>,
  githubRepos: string[],
  githubReposLoading: boolean,
  connectError: string,
  connectLoading: boolean,
}

export interface IAppsQuery {
  org_id?: string,
}

// Default context values
export const AppsContext = createContext<IAppsContext>({
  apps: [],
  app: undefined,
  githubStatus: undefined,
  helixApps: [],
  githubApps: [],
  loadApps: async () => {},
  loadApp: async () => {},
  setApp: () => {},
  loadGithubStatus: async () => {},
  loadGithubRepos: async () => {},
  createGithubApp: async () => null,
  createEmptyHelixApp: async () => undefined,
  createOrgApp: async () => undefined,
  createApp: async () => undefined,
  updateApp: async () => undefined,
  deleteApp: async () => undefined,
  githubRepos: [],
  githubReposLoading: false,
  connectError: '',
  connectLoading: false,
})

// Hook that contains all the logic from the useApps hook
export const useAppsContext = (): IAppsContext => {
  const api = useApi()
  const account = useAccount()
  const mountedRef = useRef(true)
  
  const [ apps, setApps ] = useState<IApp[]>([])
  const [ app, setApp ] = useState<IApp>()
  const [ githubRepos, setGithubRepos ] = useState<string[]>([])
  const [ githubStatus, setGithubStatus ] = useState<IGithubStatus>()
  const [ githubReposLoading, setGithubReposLoading ] = useState(false)
  const [ connectError, setConnectError ] = useState('')
  const [ connectLoading, setConectLoading ] = useState(false)

  const loadApps = useCallback(async () => {
    
    // Determine the organization_id parameter value
    let organizationIdParam = account.organizationTools.organization?.id || ''

    const result = await api.get<IApp[]>(`/api/v1/apps`, {
      params: {
        organization_id: organizationIdParam,
      }
    }, {
      snackbar: true,
    })

    setApps(result || [])
  }, [account.organizationTools.organization])

  const helixApps = useMemo(() => {
    return apps.filter(app => app.app_source == APP_SOURCE_HELIX)
  }, [
    apps,
  ])

  const githubApps = useMemo(() => {
    return apps.filter(app => app.app_source == APP_SOURCE_GITHUB)
  }, [
    apps,
  ])

  const loadApp = useCallback(async (id: string, showErrors: boolean = true) => {
    if(!id) return
    const result = await api.get<IApp>(`/api/v1/apps/${id}`, undefined, {
      snackbar: showErrors,
    })
    if(!result || !mountedRef.current) return
    setApp(result)
    setApps(prevData => prevData.map(a => a.id === id ? result : a))
  }, [api])

  const loadGithubStatus = useCallback(async (pageURL: string) => {
    const result = await api.get<IGithubStatus>(`/api/v1/github/status`, {
      params: {
        pageURL,
      }
    })
    if(!result) return
    setGithubStatus(result)
  }, [])

  const loadGithubRepos = useCallback(async () => {
    if(!githubStatus?.has_token) {
      return
    }
    setGithubReposLoading(true)
    const repos = await api.get<string[]>(`/api/v1/github/repos`)
    if(!mountedRef.current) return
    setGithubRepos(repos || [])
    setGithubReposLoading(false)
  }, [
    githubStatus,
  ])

  const createGithubApp = useCallback(async (
    repo: string,
  ): Promise<IApp | null> => {
    setConnectError('')
    setConectLoading(true)
    const result = await api.post<Partial<IApp>, IApp>(`/api/v1/apps`, {
      app_source: APP_SOURCE_GITHUB,
      organization_id: account.organizationTools.organization?.id || '',
      config: {
        helix: {
          external_url: '',
          name: '',
          description: '',
          avatar: '',
          image: '',
          assistants: [],
        },
        github: {
          repo,
          hash: ''
        },
        secrets: {},
        allowed_domains: [],
      },
    }, {}, {
      snackbar: true,
      errorCapture: (e) => {
        setConnectError(e)
      }
    })
    setConectLoading(false)
    return result
  }, [
    account.organizationTools.organization,
  ])

  const createApp = useCallback(async (
    app_source: IAppSource,
    config: IAppConfig,
  ): Promise<IApp | undefined> => {
    console.log("useApps: Creating new app with source:", app_source);
    console.log("useApps: App config:", config);
    try {
      const result = await api.post<Partial<IApp>, IApp>(`/api/v1/apps`, {
        app_source,
        config,
      }, {}, {
        snackbar: true, // We'll handle snackbar messages in the component
      });
      console.log("useApps: Create result:", result);
      if (!result) {
        console.log("useApps: No result returned from create");
        return undefined;
      }
      await loadApps();
      return result;
    } catch (error) {
      console.error("useApps: Error creating app:", error);
      throw error; // Re-throw the error so it can be caught in the component
    }
  }, [api, loadApps])

  // helper function to create a new empty helix app without any config
  // this is so we can get a UUID from the server before we start to mess with the app form
  const createEmptyHelixApp = useCallback(async (): Promise<IApp | undefined> => {
    console.log("useApps: Creating new empty app");
    try {
      // Get the first available model
      const defaultModel = account.models && account.models.length > 0 ? account.models[0].id : '';

      const result = await api.post<Partial<IApp>, IApp>(`/api/v1/apps`, {
        app_source: 'helix',
        organization_id: account.organizationTools.organization?.id || '',
        config: {
          helix: {
            external_url: '',
            name: '',
            description: '',
            avatar: '',
            image: '',
            assistants: [{
              name: 'Default Assistant',
              description: '',
              avatar: '',
              image: '',
              model: defaultModel,
              type: SESSION_TYPE_TEXT,
              system_prompt: '',
              apis: [],
              gptscripts: [],
              tools: [],
              rag_source_id: '',
              lora_id: '',
              is_actionable_template: '',
            }],
          },
          secrets: {},
          allowed_domains: [],
        }
      }, {
        params: {
          create: true,
        }
      }, {
        snackbar: true, // We'll handle snackbar messages in the component
      });
      console.log("useApps: Create empty app result:", result);
      if (!result) {
        console.log("useApps: No result returned from create empty app");
        return undefined;
      }
      await loadApps();
      return result;
    } catch (error) {
      console.error("useApps: Error creating app:", error);
      throw error; // Re-throw the error so it can be caught in the component
    }
  }, [
    api,
    loadApps,
    account.models,
    account.organizationTools.organization,
  ])

  // this is aware of the current org that we are in
  const createOrgApp = useCallback(async (): Promise<IApp | undefined> => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return
    }
    const newApp = await createEmptyHelixApp()
    if(!newApp) return undefined
    account.orgNavigate('app', {
      app_id: newApp.id,
    })
    return newApp
  }, [
    api,
    createEmptyHelixApp,
    account.user,
    account.orgNavigate,
  ])

  const updateApp = useCallback(async (id: string, updatedApp: IAppUpdate): Promise<IApp | undefined> => {
    try {
      const url = `/api/v1/apps/${id}`;
      console.log("useApps: Request URL:", url);
      const result = await api.put<IAppUpdate, IApp>(url, updatedApp, {}, {
        snackbar: true,
      });      
      if (!result) {
        console.log("useApps: No result returned from update");
        return undefined;
      }
      loadApps();
      return result;
    } catch (error) {
      console.error("useApps: Error updating app:", error);
      if (error instanceof Error) {
        console.error("Error message:", error.message);
        console.error("Error stack:", error.stack);
      }
      throw error;
    }
  }, [api, loadApps]);

  const deleteApp = useCallback(async (id: string): Promise<boolean | undefined> => {
    await api.delete(`/api/v1/apps/${id}`, {}, {
      snackbar: true,
    })
    await loadApps()
    return true
  }, [
    api,
    loadApps,
  ])

  // Load initial data when user is available (just like in the sessions context)
  useEffect(() => {
    if(!account.user) return
    loadApps()
  }, [
    account.user,
    account.organizationTools.organization,
  ])

  return {
    apps,
    app,
    githubStatus,
    helixApps,
    githubApps,
    loadApps,
    loadApp,
    setApp,
    loadGithubStatus,
    loadGithubRepos,
    createGithubApp,
    createEmptyHelixApp,
    createOrgApp,
    createApp,
    updateApp,
    deleteApp,
    githubRepos,
    githubReposLoading,
    connectError,
    connectLoading,
  }
}

// Provider component that wraps children with the context
export const AppsContextProvider: FC = ({ children }) => {
  const value = useAppsContext()
  return (
    <AppsContext.Provider value={ value }>
      { children }
    </AppsContext.Provider>
  )
} 