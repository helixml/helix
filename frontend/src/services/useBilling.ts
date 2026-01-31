import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const userWalletQueryKey = (orgId?: string) => [
  "user",
  "wallet",
  orgId
];

export function useGetWallet(orgId?: string, enabled?: boolean) {
  const api = useApi()
  const apiClient = api.getApiClient()  

  return useQuery({
    queryKey: userWalletQueryKey(orgId),
    queryFn: async () => {
      const response = await apiClient.v1WalletList({
        org_id: orgId
      })
      return response.data
    },
    refetchInterval: 30000, // 30 seconds
    enabled: enabled ?? true,
  })
}