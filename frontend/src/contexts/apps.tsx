import React, { useEffect, createContext, useMemo, useState, useCallback, useRef, ReactNode } from 'react'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'

import {
  IApp,
  IAppUpdate,
  IAppConfig,  
  SESSION_TYPE_TEXT,
} from '../types'

// Apps context interface that mirrors the return value of the useApps hook
export interface IAppsContext {
  apps: IApp[],
  app: IApp | undefined,
  loadApps: (query?: IAppsQuery) => Promise<void>,
  loadApp: (id: string, showErrors?: boolean) => Promise<void>,
  setApp: React.Dispatch<React.SetStateAction<IApp | undefined>>,
  createEmptyHelixApp: () => Promise<IApp | undefined>,
  createOrgApp: () => Promise<IApp | undefined>,
  createApp: (config: IAppConfig) => Promise<IApp | undefined>,
  updateApp: (id: string, updatedApp: IAppUpdate) => Promise<IApp | undefined>,
  deleteApp: (id: string) => Promise<boolean | undefined>,
}

export interface IAppsQuery {
  org_id?: string,
}

// Default context values
export const AppsContext = createContext<IAppsContext>({
  apps: [],
  app: undefined,
  loadApps: async () => {},
  loadApp: async () => {},
  setApp: () => {},
  createEmptyHelixApp: async () => undefined,
  createOrgApp: async () => undefined,
  createApp: async () => undefined,
  updateApp: async () => undefined,
  deleteApp: async () => undefined,
})

// Hook that contains all the logic from the useApps hook
export const useAppsContext = (): IAppsContext => {
  const api = useApi()
  const account = useAccount()
  const mountedRef = useRef(true)
  
  const [ apps, setApps ] = useState<IApp[]>([])
  const [ app, setApp ] = useState<IApp>()  

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

  const loadApp = useCallback(async (id: string, showErrors: boolean = true) => {
    if(!id) return
    const result = await api.get<IApp>(`/api/v1/apps/${id}`, undefined, {
      snackbar: showErrors,
    })
    if(!result || !mountedRef.current) return
    setApp(result)
    setApps(prevData => prevData.map(a => a.id === id ? result : a))
  }, [api])

  const createApp = useCallback(async (
    config: IAppConfig,
  ): Promise<IApp | undefined> => {
    try {
      const result = await api.post<Partial<IApp>, IApp>(`/api/v1/apps`, {
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
    loadApps,
    loadApp,
    setApp,
    createEmptyHelixApp,
    createOrgApp,
    createApp,
    updateApp,
    deleteApp,
  }
}

// Provider component that wraps children with the context
export const AppsContextProvider = ({ children }: { children: ReactNode }) => {
  const value = useAppsContext()
  return (
    <AppsContext.Provider value={ value }>
      { children }
    </AppsContext.Provider>
  )
} 