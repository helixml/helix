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

// Code agent runtime options for zed_external agents
export type CodeAgentRuntime = 'zed_agent' | 'qwen_code'

// Display names for code agent runtimes (maintainable for future additions)
export const CODE_AGENT_RUNTIME_DISPLAY_NAMES: Record<CodeAgentRuntime, string> = {
  'zed_agent': 'Zed Agent',
  'qwen_code': 'Qwen Code',
}

// Generate a nice display name from a model ID
export function getModelDisplayName(modelId: string): string {
  // Known model patterns with nice names
  const modelPatterns: [RegExp, string][] = [
    // Anthropic Claude models
    [/claude-opus-4-5/, 'Opus 4.5'],
    [/claude-sonnet-4-5/, 'Sonnet 4.5'],
    [/claude-haiku-4-5/, 'Haiku 4.5'],
    [/claude-opus-4/, 'Opus 4'],
    [/claude-sonnet-4/, 'Sonnet 4'],
    [/claude-haiku-4/, 'Haiku 4'],
    [/claude-3-5-sonnet/, 'Sonnet 3.5'],
    [/claude-3-5-haiku/, 'Haiku 3.5'],
    [/claude-3-opus/, 'Opus 3'],
    [/claude-3-sonnet/, 'Sonnet 3'],
    [/claude-3-haiku/, 'Haiku 3'],
    // OpenAI models
    [/gpt-5\.1/, 'GPT-5.1'],
    [/gpt-5/, 'GPT-5'],
    [/gpt-4\.5/, 'GPT-4.5'],
    [/gpt-4o-mini/, 'GPT-4o Mini'],
    [/gpt-4o/, 'GPT-4o'],
    [/gpt-4-turbo/, 'GPT-4 Turbo'],
    [/gpt-4/, 'GPT-4'],
    [/o4-mini/, 'o4-mini'],
    [/o3-mini/, 'o3-mini'],
    [/o3/, 'o3'],
    [/o1-preview/, 'o1 Preview'],
    [/o1-mini/, 'o1-mini'],
    [/o1/, 'o1'],
    // Google Gemini models
    [/gemini-3-pro/, 'Gemini 3 Pro'],
    [/gemini-2\.5-pro/, 'Gemini 2.5 Pro'],
    [/gemini-2\.5-flash/, 'Gemini 2.5 Flash'],
    [/gemini-2\.0-flash/, 'Gemini 2.0 Flash'],
    // Zhipu GLM models
    [/glm-4\.6/, 'GLM 4.6'],
    [/glm-4\.5/, 'GLM 4.5'],
    // Qwen models (order matters - more specific first)
    [/Qwen3-Coder-480B/, 'Qwen 3 Coder 480B'],
    [/Qwen3-Coder-30B/, 'Qwen 3 Coder 30B'],
    [/Qwen3-Coder/, 'Qwen 3 Coder'],
    [/Qwen3-235B/, 'Qwen 3 235B'],
    [/Qwen2\.5-72B/, 'Qwen 2.5 72B'],
    [/Qwen2\.5-7B/, 'Qwen 2.5 7B'],
    // Llama models
    [/Llama-4-Scout/, 'Llama 4 Scout'],
    [/Llama-4-Maverick/, 'Llama 4 Maverick'],
  ]

  for (const [pattern, displayName] of modelPatterns) {
    if (pattern.test(modelId)) {
      return displayName
    }
  }

  // Fallback: return the model ID as-is
  return modelId
}

// Generate an agent name from model and runtime
export function generateAgentName(modelId: string, runtime: CodeAgentRuntime): string {
  if (!modelId) return '-'  // Show dash when model not yet selected
  const modelName = getModelDisplayName(modelId)
  const runtimeName = CODE_AGENT_RUNTIME_DISPLAY_NAMES[runtime]
  return `${modelName} in ${runtimeName}`
}

// Add new interface for agent creation parameters
export interface ICreateAgentParams {
  name: string;
  description?: string;
  avatar?: string;
  image?: string;
  systemPrompt?: string;
  knowledge?: IKnowledgeSource[];
  agentType?: string; // Agent type: 'helix_basic', 'helix_agent', or 'zed_external'

  // Code agent runtime for zed_external agents
  codeAgentRuntime?: CodeAgentRuntime;

  // Default model for basic chat mode (non-agent mode)
  model?: string;

  // Models and providers for agent mode
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
      // Use the model from params, or fall back to generation_model if not provided
      const effectiveModel = params.model || params.generationModel || '';

      const result = await api.post<Partial<IApp>, IApp>(`/api/v1/apps`, {
        organization_id: account.organizationTools.organization?.id || '',
        config: {
          helix: {
            external_url: '',
            name: params.name,
            description: params.description || '',
            avatar: params.avatar || '',
            image: params.image || '',
            default_agent_type: params.agentType || 'helix_basic',
            assistants: [{
              name: params.name,
              description: '',
              agent_mode: false,
              agent_type: params.agentType || 'helix_basic',
              code_agent_runtime: params.codeAgentRuntime || 'zed_agent',
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
              model: effectiveModel,
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