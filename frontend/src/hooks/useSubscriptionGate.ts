import { useCallback } from 'react'
import useRouter from './useRouter'
import useAccount from './useAccount'
import { useGetConfig } from '../services/userService'
import { useGetWallet } from '../services/useBilling'

/**
 * Hook that determines whether the subscription paywall should be shown.
 * Returns paywallActive=true when require_active_subscription is enabled
 * in server config AND the current user/org does not have an active subscription.
 */
export function useSubscriptionGate() {
  const router = useRouter()
  const account = useAccount()
  const { data: serverConfig, isLoading: isLoadingConfig } = useGetConfig()

  const orgId = router.params.org_id || account.organizationTools.organization?.id
  const requireSub = serverConfig?.require_active_subscription ?? false

  const { data: wallet, isLoading: isLoadingWallet } = useGetWallet(
    orgId,
    !isLoadingConfig && requireSub,
  )

  const isSubscriptionActive = wallet?.subscription_status === 'active'
  const isLoading = isLoadingConfig || (requireSub && isLoadingWallet)
  const paywallActive = requireSub && !isSubscriptionActive && !isLoading

  const navigateToBilling = useCallback(() => {
    if (orgId) {
      router.navigate('org_billing', { org_id: orgId })
    } else {
      router.navigate('account')
    }
  }, [orgId, router])

  return {
    paywallActive,
    isLoading,
    navigateToBilling,
  }
}

export default useSubscriptionGate
