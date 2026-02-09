import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import type { TypesOrganization } from '../api/api';

export const orgListQueryKey = () => ["orgs"];

export const orgQueryNameKey = (name: string) => [
  "org",
  name
];

export const orgUsageQueryKey = (id: string) => [
  "org",
  id,
  "usage"
];

export function getOrgByIdQueryKey(id: string) {
  return [
    "org",
    id
  ];
}

export function useGetOrgById(id: string, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()
  return useQuery({
    queryKey: getOrgByIdQueryKey(id),
    queryFn: async () => {
      const response = await apiClient.v1OrganizationsDetail(id)
      return response.data
    },
    enabled: enabled,
  })
}

export function useGetOrgByName(name: string, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: orgQueryNameKey(name),
    queryFn: async () => {
      const response = await apiClient.v1OrganizationsDetail(name)
      return response.data
    },    
    enabled: enabled,
  })
}

export function useCreateOrg() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (org: TypesOrganization) => {
      const response = await apiClient.v1OrganizationsCreate(org)
      return response.data
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: orgListQueryKey() })
      if (data.id) {
        queryClient.invalidateQueries({ queryKey: getOrgByIdQueryKey(data.id) })
      }
      if (data.name) {
        queryClient.invalidateQueries({ queryKey: orgQueryNameKey(data.name) })
      }
    },
  })
}

export function useGetOrgUsage(id: string, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: orgUsageQueryKey(id),
    queryFn: async () => {
      const response = await apiClient.v1UsageList({ org_id: id })
      return response.data
    },
  })
}