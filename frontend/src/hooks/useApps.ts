import React, { FC, useState, useCallback, useMemo, useRef, useEffect } from 'react'
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

export const useApps = () => {
  const api = useApi()
  const account = useAccount()
  const mountedRef = useRef(true)
  const [isLoading, setIsLoading] = useState(false)
  
  const [ data, setData ] = useState<IApp[]>([])
  const [ app, setApp ] = useState<IApp>()
  const [ githubRepos, setGithubRepos ] = useState<string[]>([])
  const [ githubStatus, setGithubStatus ] = useState<IGithubStatus>()
  const [ githubReposLoading, setGithubReposLoading ] = useState(false)
  const [ connectError, setConnectError ] = useState('')
  const [ connectLoading, setConectLoading ] = useState(false)

  // Create a ref for the throttled function
  const throttledLoadRef = useRef<any>(null)
  
  // Initialize the throttled function once
  useEffect(() => {
    throttledLoadRef.current = throttle(async (force = false) => {
      if (isLoading) return
      setIsLoading(true)
      
      try {
        let result = await api.get<IApp[]>(`/api/v1/apps`, undefined, {
          snackbar: true,
        })
        if(!result) result = []
        if(!mountedRef.current) return
        setData(result)
      } finally {
        if(mountedRef.current) {
          setIsLoading(false)
        }
      }
    }, 1000, { leading: true, trailing: false })

    // Cleanup
    return () => {
      if (throttledLoadRef.current?.cancel) {
        throttledLoadRef.current.cancel()
      }
    }
  }, []) // Empty deps array - only create once

  // Expose loadData as a wrapper around the throttled function
  const loadData = useCallback(async (force = false) => {
    if (throttledLoadRef.current) {
      await throttledLoadRef.current(force)
    }
  }, [])

  const helixApps = useMemo(() => {
    const sourceData: IApp[] = data || []
    if(!sourceData) return []
    if(!sourceData.filter) return []
    return sourceData.filter(app => app.app_source == APP_SOURCE_HELIX)
  }, [
    data,
  ])

  const githubApps = useMemo(() => {
    const sourceData: IApp[] = data || []
    if(!sourceData) return []
    if(!sourceData.filter) return []
    return sourceData.filter(app => app.app_source == APP_SOURCE_GITHUB)
  }, [
    data,
  ])

  const loadApp = useCallback(async (id: string, showErrors: boolean = true) => {
    if(!id) return
    const result = await api.get<IApp>(`/api/v1/apps/${id}`, undefined, {
      snackbar: showErrors,
    })
    if(!result || !mountedRef.current) return
    setApp(result)
    setData(prevData => prevData.map(a => a.id === id ? result : a))
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
  }, [])

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
      await loadData();
      return result;
    } catch (error) {
      console.error("useApps: Error creating app:", error);
      throw error; // Re-throw the error so it can be caught in the component
    }
  }, [api, loadData])

  // helper function to create a new empty helix app without any config
  // this is so we can get a UUID from the server before we start to mess with the app form
  const createEmptyHelixApp = useCallback(async (): Promise<IApp | undefined> => {
    console.log("useApps: Creating new empty app");
    try {
      // Get the first available model
      const defaultModel = account.models && account.models.length > 0 ? account.models[0].id : '';

      const result = await api.post<Partial<IApp>, IApp>(`/api/v1/apps`, {
        app_source: 'helix',
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
      await loadData();
      return result;
    } catch (error) {
      console.error("useApps: Error creating app:", error);
      throw error; // Re-throw the error so it can be caught in the component
    }
  }, [api, account.models])

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
      loadData();
      return result;
    } catch (error) {
      console.error("useApps: Error updating app:", error);
      if (error instanceof Error) {
        console.error("Error message:", error.message);
        console.error("Error stack:", error.stack);
      }
      throw error;
    }
  }, [api, loadData]);

  const deleteApp = useCallback(async (id: string): Promise<boolean | undefined> => {
    await api.delete(`/api/v1/apps/${id}`, {}, {
      snackbar: true,
    })
    await loadData()
    return true
  }, [
    api,
    loadData,
  ])

  return {
    data,
    app,
    githubStatus,
    helixApps,
    githubApps,
    loadData,
    loadApp,
    setApp,
    loadGithubStatus,
    loadGithubRepos,
    createGithubApp,
    createEmptyHelixApp,
    createApp,
    updateApp,
    deleteApp,
    githubRepos,
    githubReposLoading,
    connectError,
    connectLoading,
    isLoading,
  }
}

export default useApps