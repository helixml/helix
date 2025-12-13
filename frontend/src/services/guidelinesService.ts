import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesGuidelinesHistory, TypesUserGuidelinesResponse, TypesUpdateUserGuidelinesRequest } from '../api/api';

// Query keys
export const organizationGuidelinesHistoryQueryKey = (orgId: string) => ['organization-guidelines-history', orgId];
export const userGuidelinesQueryKey = () => ['user-guidelines'];
export const userGuidelinesHistoryQueryKey = () => ['user-guidelines-history'];

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

/**
 * Hook to get the current user's personal workspace guidelines
 */
export const useGetUserGuidelines = (enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesUserGuidelinesResponse>({
    queryKey: userGuidelinesQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1UsersMeGuidelinesList();
      return response.data;
    },
    enabled,
  });
};

/**
 * Hook to update the current user's personal workspace guidelines
 */
export const useUpdateUserGuidelines = () => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation<TypesUserGuidelinesResponse, Error, TypesUpdateUserGuidelinesRequest>({
    mutationFn: async (request) => {
      const response = await apiClient.v1UsersMeGuidelinesUpdate(request);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: userGuidelinesQueryKey() });
      queryClient.invalidateQueries({ queryKey: userGuidelinesHistoryQueryKey() });
    },
  });
};

/**
 * Hook to get the current user's personal workspace guidelines history
 */
export const useGetUserGuidelinesHistory = (enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesGuidelinesHistory[]>({
    queryKey: userGuidelinesHistoryQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1UsersMeGuidelinesHistoryList();
      return response.data || [];
    },
    enabled,
  });
};
