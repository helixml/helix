import { useQuery } from '@tanstack/react-query'
import useApi from '../hooks/useApi';

export const TOP_UP_AMOUNTS = [5, 10, 20, 50, 100] as const
export const DEFAULT_TOP_UP_AMOUNT = TOP_UP_AMOUNTS[0]

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
