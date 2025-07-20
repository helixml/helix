import { useQuery } from '@tanstack/react-query';
import useApi from './useApi';
import { TypesSkillDefinition } from '../api/api';

export const useSkills = (category?: string, provider?: string) => {
  const api = useApi();
  const client = api.getApiClient();

  return useQuery({
    queryKey: ['skills', { category, provider }],
    queryFn: async () => {
      const params = new URLSearchParams();
      if (category) params.append('category', category);
      if (provider) params.append('provider', provider);

      const response = await client.v1SkillsList({
        category,
        provider,
      });

      return response.data;
    },
    staleTime: 1 * 60 * 1000, // 1 minute
    gcTime: 1 * 60 * 1000, // 1 minute
  });
};

export const useSkill = (id: string) => {
  const api = useApi();
  const client = api.getApiClient();

  return useQuery({
    queryKey: ['skill', id],
    queryFn: async () => {
      const response = await client.v1SkillsDetail(id);
      return response.data;
    },
    enabled: !!id,
    staleTime: 1 * 60 * 1000, // 1 minute
    gcTime: 1 * 60 * 1000, // 1 minute
  });
};

// Convert backend skill to frontend IAgentSkill format for compatibility
export const convertBackendSkillToFrontend = (backendSkill: TypesSkillDefinition) => {
  return {
    name: backendSkill.displayName || backendSkill.name || '',
    description: backendSkill.description || '',
    systemPrompt: backendSkill.systemPrompt || '',
    apiSkill: {
      schema: backendSkill.schema || '',
      url: backendSkill.baseUrl || '',
      requiredParameters: [],
      headers: backendSkill.headers || {},
    },
    configurable: backendSkill.configurable || false, 
    skip_unknown_keys: backendSkill.skipUnknownKeys || false,
    transform_output: backendSkill.transformOutput || false,
  };
}; 