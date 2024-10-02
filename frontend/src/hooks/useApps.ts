import React, { FC, useState, useCallback, useMemo } from 'react'
import useApi from '../hooks/useApi'

import {
  IApp,
  IAppUpdate,
  IAppConfig,
  IAppSource,
  IGithubStatus,
  APP_SOURCE_GITHUB,
  APP_SOURCE_HELIX,
} from '../types'

// import {
//   APPS,
// } from '../fixtures'

export const useApps = () => {
  const api = useApi()
  
  const [ data, setData ] = useState<IApp[]>([])
  const [ app, setApp ] = useState<IApp>()
  const [ githubRepos, setGithubRepos ] = useState<string[]>([])
  const [ githubStatus, setGithubStatus ] = useState<IGithubStatus>()
  const [ githubReposLoading, setGithubReposLoading ] = useState(false)
  const [ connectError, setConnectError ] = useState('')
  const [ connectLoading, setConectLoading ] = useState(false)

  const helixApps = useMemo(() => {
    return data.filter(app => app.app_source == APP_SOURCE_HELIX)
  }, [
    data,
  ])

  const githubApps = useMemo(() => {
    return data.filter(app => app.app_source == APP_SOURCE_GITHUB)
  }, [
    data,
  ])

  const loadData = useCallback(async () => {
    const result = await api.get<IApp[]>(`/api/v1/apps`, undefined, {
      snackbar: true,
    })
    if(!result) return
    setData(result)
    // setData(APPS)
  }, [])

  const loadApp = useCallback(async (id: string) => {
    if(!id) return
    const result = await api.get<IApp>(`/api/v1/apps/${id}`, undefined, {
      snackbar: true,
    })
    if(!result) return
    setApp(result)
    setData(prevData => prevData.map(a => a.id === id ? result : a))
    // setApp(APPS[0])
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
      loadData();
      return result;
    } catch (error) {
      console.error("useApps: Error creating app:", error);
      throw error; // Re-throw the error so it can be caught in the component
    }
  }, [api, loadData])

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
    loadData()
    return true
  }, [
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
    createApp,
    updateApp,
    deleteApp,
    githubRepos,
    githubReposLoading,
    connectError,
    connectLoading,
  }
}

export default useApps