import { useQuery } from '@tanstack/react-query';
import useApi from './useApi';

// Backend skill types
export interface BackendSkillDefinition {
  id: string;
  name: string;
  displayName?: string;
  description: string;
  systemPrompt?: string;
  schema?: string;
  baseUrl?: string;
  oauthProvider?: string;
  oauthScopes?: string[];
  configurable?: boolean;
  category?: string;
  enabled?: boolean;
  icon?: any;
  createdAt?: string;
  updatedAt?: string;
}

export interface BackendSkillsListResponse {
  skills: BackendSkillDefinition[];
  count: number;
}

export const useSkills = (category?: string, provider?: string) => {
  const api = useApi();

  return useQuery({
    queryKey: ['skills', { category, provider }],
    queryFn: async () => {
      const params = new URLSearchParams();
      if (category) params.append('category', category);
      if (provider) params.append('provider', provider);
      const queryString = params.toString();
      const url = `/api/v1/skills${queryString ? `?${queryString}` : ''}`;
      const response = await api.get<BackendSkillsListResponse>(url);
      return response;
    },
    staleTime: 5 * 60 * 1000, // 5 minutes
    gcTime: 10 * 60 * 1000, // 10 minutes
  });
};

export const useSkill = (id: string) => {
  const api = useApi();

  return useQuery({
    queryKey: ['skill', id],
    queryFn: async () => {
      const response = await api.get<BackendSkillDefinition>(`/api/v1/skills/${id}`);
      return response;
    },
    enabled: !!id,
    staleTime: 5 * 60 * 1000, // 5 minutes
    gcTime: 10 * 60 * 1000, // 10 minutes
  });
};

// Convert backend skill to frontend IAgentSkill format for compatibility
export const convertBackendSkillToFrontend = (backendSkill: BackendSkillDefinition) => {
  return {
    name: backendSkill.displayName || backendSkill.name || '',
    description: backendSkill.description || '',
    systemPrompt: backendSkill.systemPrompt || '',
    apiSkill: {
      schema: backendSkill.schema || '',
      url: backendSkill.baseUrl || '',
      requiredParameters: [],
    },
    configurable: backendSkill.configurable || false,
  };
}; 