import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const orgQueryNameKey = (name: string) => [
  "org",
  name
];

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