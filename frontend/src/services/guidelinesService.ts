import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesGuidelinesHistory } from '../api/api';

// Query keys
export const organizationGuidelinesHistoryQueryKey = (orgId: string) => ['organization-guidelines-history', orgId];

/**
 * Hook to get organization guidelines version history
 */
export const useGetOrganizationGuidelinesHistory = (orgId: string, enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesGuidelinesHistory[]>({
    queryKey: organizationGuidelinesHistoryQueryKey(orgId),
    queryFn: async () => {
      const response = await apiClient.v1OrganizationsGuidelinesHistoryDetail(orgId);
      return response.data || [];
    },
    enabled: enabled && !!orgId,
  });
};
