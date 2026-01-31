import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

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