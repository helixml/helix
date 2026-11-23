import { FC } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import CircularProgress from '@mui/material/CircularProgress'
import Stack from '@mui/material/Stack'
import Typography from '@mui/material/Typography'
import { alpha } from '@mui/material/styles'
import { ChevronRight } from 'lucide-react'

import useRouter from '../../hooks/useRouter'
import { useGetQuota } from '../../services/quotaService'
import { useGetWallet } from '../../services/useBilling'
import { useGetConfig } from '../../services/userService'
import { formatCost } from '../../utils/usageFormat'

interface UsageLimitRow {
  label: string
  used: number
  max: number
}

// A max of -1 means the quota isn't enforced; 0 means none are allowed.
export const limitState = (used: number, max: number) => {
  if (max < 0) return { limited: false, share: 0, tone: 'normal' as const }
  const share = max === 0 ? 1 : used / max
  return { limited: true, share, tone: share >= 1 ? 'full' as const : share >= 0.8 ? 'near' as const : 'normal' as const }
}

const LimitRow: FC<{ row: UsageLimitRow; first: boolean }> = ({ row, first }) => {
  const state = limitState(row.used, row.max)
  const color = state.tone === 'full' ? 'error.main' : state.tone === 'near' ? 'warning.main' : 'primary.main'
  return (
    <Stack spacing={1} sx={{ px: 2, py: 1.5, borderTop: first ? 'none' : '1px solid', borderColor: 'divider' }}>
      <Stack direction="row" justifyContent="space-between" alignItems="baseline" spacing={2}>
        <Typography variant="body1" sx={{ fontWeight: 500 }}>{row.label}</Typography>
        <Typography variant="body2" color={state.tone === 'normal' ? 'text.secondary' : color} sx={{ fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap' }}>
          {state.limited ? `${row.used.toLocaleString()} of ${row.max.toLocaleString()}` : `${row.used.toLocaleString()} used · no limit`}
        </Typography>
      </Stack>
      {state.limited && (
        <Box
          role="meter"
          aria-label={row.label}
          aria-valuenow={row.used}
          aria-valuemin={0}
          aria-valuemax={row.max}
          sx={{ height: 6, borderRadius: 999, bgcolor: theme => alpha(theme.palette.text.primary, 0.08), overflow: 'hidden' }}
        >
          <Box sx={{ width: `${Math.min(state.share, 1) * 100}%`, minWidth: row.used > 0 ? 6 : 0, height: '100%', borderRadius: 999, bgcolor: color }} />
        </Box>
      )}
    </Stack>
  )
}

const Card: FC<{ children: React.ReactNode }> = ({ children }) => (
  <Box sx={{ bgcolor: 'background.paper', border: '1px solid', borderColor: 'divider', borderRadius: 3, overflow: 'hidden' }}>
    {children}
  </Box>
)

const SectionTitle: FC<{ children: React.ReactNode }> = ({ children }) => (
  <Typography variant="body2" component="h3" sx={{ fontWeight: 600, color: 'text.secondary', px: 0.5, mb: 1 }}>{children}</Typography>
)

// UsageLimits is the Limits tab: the credit balance usage draws down, and the
// org's resource quotas with how much of each is in use.
const UsageLimits: FC<{ orgId: string }> = ({ orgId }) => {
  const router = useRouter()
  const { data: serverConfig, isLoading: configLoading } = useGetConfig()
  const billing = Boolean(serverConfig?.billing_enabled)
  const wallet = useGetWallet(orgId, !configLoading && billing)
  const quota = useGetQuota(orgId)

  if (configLoading || quota.isLoading || (billing && wallet.isLoading)) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
        <CircularProgress />
      </Box>
    )
  }

  const q = quota.data
  const rows: UsageLimitRow[] = q ? [
    { label: 'Concurrent desktops', used: q.active_concurrent_desktops ?? 0, max: q.max_concurrent_desktops ?? 0 },
    { label: 'Desktop sandboxes', used: q.active_desktop_sandboxes ?? 0, max: q.max_desktop_sandboxes ?? 0 },
    { label: 'Headless sandboxes', used: q.active_headless_sandboxes ?? 0, max: q.max_headless_sandboxes ?? 0 },
    { label: 'Projects', used: q.projects ?? 0, max: q.max_projects ?? 0 },
    { label: 'Repositories', used: q.repositories ?? 0, max: q.max_repositories ?? 0 },
    { label: 'Spec tasks', used: q.spec_tasks ?? 0, max: q.max_spec_tasks ?? 0 },
  ] : []
  // Limited quotas first, fullest first: those are the ones that can stop work.
  const ordered = [...rows].sort((a, b) => {
    const sa = limitState(a.used, a.max)
    const sb = limitState(b.used, b.max)
    if (sa.limited !== sb.limited) return sa.limited ? -1 : 1
    return sb.share - sa.share
  })
  const balance = wallet.data?.balance ?? 0
  const periodEnd = wallet.data?.subscription_current_period_end

  return (
    <Box sx={{ display: 'grid', gap: 3, gridTemplateColumns: { xs: 'minmax(0, 1fr)', lg: billing ? '380px minmax(0, 720px)' : 'minmax(0, 720px)' }, alignItems: 'start' }}>
      {billing && (
        <Box>
          <SectionTitle>Credits</SectionTitle>
          <Card>
            <Box sx={{ p: { xs: 2, sm: 2.5 } }}>
              <Typography variant="body2" color="text.secondary">Balance</Typography>
              <Typography sx={{ fontSize: '2.25rem', fontWeight: 700, lineHeight: 1.15, letterSpacing: '-0.02em', fontVariantNumeric: 'tabular-nums' }}>
                {wallet.isError ? '—' : formatCost(balance)}
              </Typography>
              <Typography variant="body2" color="text.secondary">
                {wallet.isError
                  ? 'Could not load the balance.'
                  : wallet.data?.subscription_status
                    ? `Plan ${wallet.data.subscription_status}${periodEnd ? ` · renews ${new Date(periodEnd * 1000).toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}` : ''}`
                    : 'LLM tokens and sandbox compute draw from this balance.'}
              </Typography>
            </Box>
            <Box sx={{ borderTop: '1px solid', borderColor: 'divider', px: 1, py: 0.5 }}>
              <Button
                fullWidth
                color="inherit"
                endIcon={<ChevronRight size={16} />}
                onClick={() => router.navigate('org_billing', { org_id: orgId })}
                sx={{ justifyContent: 'space-between', textTransform: 'none', color: 'text.secondary', '&:hover': { color: 'text.primary' } }}
              >
                Billing and top-ups
              </Button>
            </Box>
          </Card>
        </Box>
      )}
      <Box>
        <SectionTitle>Resource limits</SectionTitle>
        <Card>
          {quota.isError || !ordered.length ? (
            <Typography variant="body2" color="text.secondary" sx={{ p: 2 }}>Could not load the organization's limits.</Typography>
          ) : (
            ordered.map((row, index) => <LimitRow key={row.label} row={row} first={index === 0} />)
          )}
        </Card>
      </Box>
    </Box>
  )
}

export default UsageLimits
