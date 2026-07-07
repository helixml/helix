import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesProviderEndpoint, RequestParams, TypesUpdateProviderEndpoint, ContentType } from '../api/api';

export const providersQueryKey = (loadModels: boolean = false, orgId?: string, all?: boolean) => [
  "providers",
  loadModels ? "withModels" : "withoutModels",
  orgId,
  all
];

export interface ListProvidersOptions {
  loadModels?: boolean;
  orgId?: string;
  all?: boolean;
  enabled?: boolean;
}

export function useListProviders(options: ListProvidersOptions) {
  const { loadModels, orgId, all, enabled } = options;
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: providersQueryKey(loadModels, orgId, all),
    queryFn: async () => {
      const result = await apiClient.v1ProviderEndpointsList({
        with_models: loadModels,
        org_id: orgId,
        all: all,
      })
      return result.data
    },  
    enabled: enabled,
    staleTime: 3 * 1000, // 3 seconds (useful when going between pages)
  });
}

export function useCreateProviderEndpoint() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (providerEndpoint: Partial<TypesProviderEndpoint>) => {
      const result = await apiClient.v1ProviderEndpointsCreate({
        body: providerEndpoint,
        type: ContentType.Json
      } as RequestParams)
      return result.data
    },
    onSuccess: () => {
      // Invalidate all provider queries (with any combination of params)
      queryClient.invalidateQueries({ queryKey: ['providers'] })
    }
  });
}

export interface DetectedProvider {
  name: string;
  server_type: string;
  base_url: string;
  models: string[];
}

export function useDetectLocalProviders(enabled: boolean) {
  return useQuery({
    queryKey: ['providers', 'detect-local'],
    queryFn: async (): Promise<DetectedProvider[]> => {
      const res = await fetch('/api/v1/providers/detect-local');
      if (!res.ok) return [];
      const data = await res.json();
      return data.providers || [];
    },
    enabled,
    staleTime: 30_000,
    refetchInterval: 30_000,
  });
}

export interface UpdateProviderEndpointArgs {
  id: string;
  body: Partial<TypesUpdateProviderEndpoint>;
}

export function useUpdateProviderEndpoint() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ id, body }: UpdateProviderEndpointArgs) => {
      if (!id) {
        throw new Error('Cannot update provider endpoint: missing id')
      }
      const result = await apiClient.v1ProviderEndpointsUpdate(id, {
        body,
        type: ContentType.Json
      } as RequestParams)
      return result.data
    },
    onSuccess: () => {
      // Invalidate all provider queries (with any combination of params)
      queryClient.invalidateQueries({ queryKey: ['providers'] })
    }
  });
}

export interface LocalModel {
  key: string;
  type: string;
  display_name?: string;
  publisher?: string;
  architecture?: string;
  quantization?: { name: string; bits_per_weight: number };
  size_bytes: number;
  params_string?: string;
  max_context_length: number;
  format?: string;
  loaded_instances: Array<{ id: string; config?: Record<string, unknown> }>;
  capabilities?: { vision?: boolean; trained_for_tool_use?: boolean };
}

export function useLocalModels(endpointId: string | undefined, enabled: boolean) {
  return useQuery({
    queryKey: ['providers', 'local-models', endpointId],
    queryFn: async (): Promise<LocalModel[]> => {
      if (!endpointId) return [];
      const res = await fetch(`/api/v1/provider-endpoints/${endpointId}/local-models`);
      if (!res.ok) return [];
      const data = await res.json();
      return data.models || [];
    },
    enabled: enabled && !!endpointId,
    refetchInterval: 5000,
  });
}

export function useLoadLocalModel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ endpointId, model, contextLength }: { endpointId: string; model: string; contextLength?: number }) => {
      const body: Record<string, unknown> = { model };
      if (contextLength) body.context_length = contextLength;
      const res = await fetch(`/api/v1/provider-endpoints/${endpointId}/local-models/load`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(body),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['providers'] });
    },
  });
}

export function useUnloadLocalModel() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ endpointId, model }: { endpointId: string; model: string }) => {
      const res = await fetch(`/api/v1/provider-endpoints/${endpointId}/local-models/unload`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ model }),
      });
      if (!res.ok) throw new Error(await res.text());
      return res.json();
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['providers'] });
    },
  });
}

export function useDeleteProviderEndpoint() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: string) => {
      const result = await apiClient.v1ProviderEndpointsDelete(id)
      return result.data
    },
    onSuccess: () => {
      // Invalidate all provider queries (with any combination of params)
      queryClient.invalidateQueries({ queryKey: ['providers'] })
    }
  });
}