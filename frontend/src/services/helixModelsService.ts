import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesModel, RequestParams, ContentType } from '../api/api';

export const helixModelsQueryKey = (runtime: string = "") => [
  "helixModels",
  runtime
];

export function useListHelixModels(runtime: string = "") {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: helixModelsQueryKey(runtime),
    queryFn: async () => {
      const result = await apiClient.v1HelixModelsList({
        runtime: runtime,
      })
      return result.data
    },
    enabled: true,
    staleTime: 3 * 1000, // 3 seconds (useful when going between pages)
  });
}
