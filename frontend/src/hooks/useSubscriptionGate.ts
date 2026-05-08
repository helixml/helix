import { useCallback } from 'react'
import useRouter from './useRouter'
import useAccount from './useAccount'
import { useGetConfig } from '../services/userService'
import { useGetWallet } from '../services/useBilling'
import { useSettingsDialog } from '../contexts/settingsDialog'

interface WalletWithCancelField {
  subscription_status?: string
  subscription_cancel_at_period_end?: boolean
}

/**
 * Hook that determines whether the subscription paywall should be shown.
 * Returns paywallActive=true when require_active_subscription is enabled
 * in server config AND the current user/org does not have an active subscription.
 *
 * Also returns isPastDue and isCancelling for warning banners.
 */
export function useSubscriptionGate() {
  const router = useRouter()
  const account = useAccount()
  const settingsDialog = useSettingsDialog()
  const { data: serverConfig, isLoading: isLoadingConfig } = useGetConfig()

  const orgId = router.params.org_id || account.organizationTools.organization?.id
  const requireSub = serverConfig?.require_active_subscription ?? false

  const { data: wallet, isLoading: isLoadingWallet } = useGetWallet(
    orgId,
    !isLoadingConfig && requireSub,
  )

  const extWallet = wallet as WalletWithCancelField | undefined
  const isSubscriptionActive = extWallet?.subscription_status === 'active'
  const isLoading = isLoadingConfig || (requireSub && isLoadingWallet)
  const paywallActive = requireSub && !isSubscriptionActive && !isLoading

  const isPastDue = requireSub && extWallet?.subscription_status === 'past_due' && !isLoading
  const isCancelling = requireSub && isSubscriptionActive && !!extWallet?.subscription_cancel_at_period_end && !isLoading

  const navigateToBilling = useCallback(() => {
    if (orgId) {
      router.navigate('org_billing', { org_id: orgId })
    } else {
      settingsDialog.openDialog('account')
    }
  }, [orgId, router, settingsDialog])

  return {
    paywallActive,
    isPastDue,
    isCancelling,
    isLoading,
    navigateToBilling,
  }
}

export default useSubscriptionGate
