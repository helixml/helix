import React, { useEffect, createContext, useState, useCallback, useRef, ReactNode } from 'react'
import useApi from '../hooks/useApi'
import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'

import {
  IApp,
  IAppUpdate,
  IKnowledgeSource,
  SESSION_TYPE_TEXT,  
} from '../types'

// Apps context interface that mirrors the return value of the useApps hook
export interface IAppsContext {
  apps: IApp[],
  app: IApp | undefined,
  loadApps: (query?: IAppsQuery) => Promise<void>,
  loadApp: (id: string, showErrors?: boolean) => Promise<void>,
  setApp: React.Dispatch<React.SetStateAction<IApp | undefined>>,
  createAgent: (params: ICreateAgentParams) => Promise<IApp | undefined>,
  updateApp: (id: string, updatedApp: IAppUpdate) => Promise<IApp | undefined>,
  deleteApp: (id: string) => Promise<boolean | undefined>,
}

export interface IAppsQuery {
  org_id?: string,
}

// Add new interface for agent creation parameters
export interface ICreateAgentParams {
  name: string;
  description?: string;
  avatar?: string;
  image?: string;
  systemPrompt?: string;
  knowledge?: IKnowledgeSource[];
  // Models and providers
  reasoningModelProvider: string;
  reasoningModel: string;
  reasoningModelEffort: string;

  generationModelProvider: string;
  generationModel: string;
  
  smallReasoningModelProvider: string;
  smallReasoningModel: string;
  smallReasoningModelEffort: string;
  
  smallGenerationModelProvider: string;
  smallGenerationModel: string;
}

// Default context values
export const AppsContext = createContext<IAppsContext>({
  apps: [],
  app: undefined,
  loadApps: async () => {},
  loadApp: async () => {},
  setApp: () => {},
  createAgent: async () => undefined,
  updateApp: async () => undefined,
  deleteApp: async () => undefined,
})

// Hook that contains all the logic from the useApps hook
export const useAppsContext = (): IAppsContext => {
  const api = useApi()
  const account = useAccount()
  const mountedRef = useRef(true)
  const router = useRouter()
  const [ apps, setApps ] = useState<IApp[]>([])
  const [ app, setApp ] = useState<IApp>()

   // Check if org slug is set in the URL
   const orgSlug = router.params.org_id || ''

   let orgLoaded = false
 
   if (orgSlug === '') {
    orgLoaded = true
   } else if (account.organizationTools.organization?.id) {
    orgLoaded = true
   }

  const loadApps = useCallback(async () => {
    if (!orgLoaded) return
    
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
  }, [account.organizationTools.organization, orgLoaded])

  const loadApp = useCallback(async (id: string, showErrors: boolean = true) => {
    if(!id || !orgLoaded) return
    const result = await api.get<IApp>(`/api/v1/apps/${id}`, undefined, {
      snackbar: showErrors,
    })
    if(!result || !mountedRef.current) return
    setApp(result)
    setApps(prevData => prevData.map(a => a.id === id ? result : a))
  }, [api, orgLoaded])

  const createAgent = useCallback(async (params: ICreateAgentParams): Promise<IApp | undefined> => {
    try {
      // Get the first available model
      const defaultModel = account.models && account.models.length > 0 ? account.models[0].id : '';

      const result = await api.post<Partial<IApp>, IApp>(`/api/v1/apps`, {
        organization_id: account.organizationTools.organization?.id || '',
        config: {
          helix: {
            external_url: '',
            name: params.name,
            description: params.description || '',
            avatar: params.avatar || '',
            image: params.image || '',
            default_agent_type: 'helix_basic',
            assistants: [{
              name: params.name,
              description: '',
              agent_mode: false,
              reasoning_model_provider: params.reasoningModelProvider,
              reasoning_model: params.reasoningModel,
              reasoning_model_effort: params.reasoningModelEffort,
              generation_model_provider: params.generationModelProvider,
              generation_model: params.generationModel,
              small_reasoning_model_provider: params.smallReasoningModelProvider,
              small_reasoning_model: params.smallReasoningModel,
              small_reasoning_model_effort: params.smallReasoningModelEffort,
              small_generation_model_provider: params.smallGenerationModelProvider,
              small_generation_model: params.smallGenerationModel,
              avatar: '',
              image: '',
              model: defaultModel,
              type: SESSION_TYPE_TEXT,
              system_prompt: params.systemPrompt || '',
              apis: [],
              gptscripts: [],
              tools: [],
              rag_source_id: '',
              lora_id: '',
              is_actionable_template: '',
              knowledge: params.knowledge || [],
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
      console.log("useApps: Create agent result:", result);
      if (!result) {
        console.log("useApps: No result returned from create agent");
        return undefined;
      }
      await loadApps();
      return result;
    } catch (error) {
      console.error("useApps: Error creating agent:", error);
      throw error; // Re-throw the error so it can be caught in the component
    }
  }, [
    api,
    loadApps,
    account.models,
    account.organizationTools.organization,
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
    createAgent,
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