import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesQuotaResponse } from '../api/api';

export const quotaQueryKey = (orgId?: string) => ['quota', orgId];

export const useGetQuota = (orgId?: string, options?: { enabled?: boolean }) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesQuotaResponse>({
    queryKey: quotaQueryKey(orgId),
    queryFn: async () => {
      const response = await apiClient.v1QuotasList({ org_id: orgId || undefined });
      return response.data;
    },
    enabled: options?.enabled ?? true,
  });
};
