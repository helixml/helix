import { FC, useMemo, useState } from 'react'
import { isAxiosError } from 'axios'
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import CircularProgress from '@mui/material/CircularProgress'
import Container from '@mui/material/Container'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import ToggleButton from '@mui/material/ToggleButton'
import ToggleButtonGroup from '@mui/material/ToggleButtonGroup'
import Typography from '@mui/material/Typography'

import Page from '../system/Page'
import MarkdownCodeBlock from '../session/MarkdownCodeBlock'
import SimpleTable, { ITableField } from '../widgets/SimpleTable'
import ShadcnAreaChart from '../usage/ShadcnAreaChart'
import {
  OverviewStat,
  OverviewStatStrip,
  USAGE_RANGE_DAYS,
  UsageHeadline,
  UsagePageHeader,
  UsageRangeControls,
  UsageRangeKey,
  UsageSection,
  UsageShareRow,
} from '../usage/UsageOverview'
import useRouter from '../../hooks/useRouter'
import { useWebSearchUsage } from '../../services/webSearchUsageService'
import { formatCost, formatNumber, formatPercent } from '../../utils/usageFormat'
import WebSearchRequestsTable from './WebSearchRequestsTable'
import {
  ENDPOINTS,
  ENDPOINT_LABELS,
  UsageMetric,
  buildDailyRows,
  rangeBounds,
  useEndpointColors,
} from './webSearchUsage'

const DEFAULT_RANGE: UsageRangeKey = '30d'

const BREAKDOWN_FIELDS: ITableField[] = [
  { name: 'endpoint', title: 'Endpoint' },
  { name: 'requests', title: 'Requests', numeric: true },
  { name: 'successRate', title: 'Success rate', numeric: true },
  { name: 'outOfCredits', title: 'Out of credits', numeric: true },
  { name: 'failed', title: 'Other errors', numeric: true },
  { name: 'spend', title: 'Spend', numeric: true },
]

const firstSearchSnippet = (baseUrl: string) => `curl -X POST ${baseUrl}/search \\
  -H "x-api-key: $HELIX_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{"query": "latest developments in open-source LLMs", "numResults": 5}'`

// The usage API answers a bad range with a plain-text 400 ("the range may
// span at most 90 days"); show that instead of a generic failure.
const usageErrorMessage = (error: unknown): string => {
  if (isAxiosError(error) && typeof error.response?.data === 'string' && error.response.data.trim()) {
    return `Failed to load web search usage: ${error.response.data.trim()}`
  }
  return 'Failed to load web search usage.'
}

const Swatch: FC<{ color: string }> = ({ color }) => (
  <Box sx={{ width: 10, height: 10, borderRadius: '2px', bgcolor: color, flexShrink: 0 }} />
)

const OrgWebSearchUsage: FC = () => {
  const router = useRouter()
  // The usage API resolves the URL's org slug as well as an org id.
  const orgId = router.params.org_id as string | undefined
  const baseUrl = `${window.location.origin}/api/v1/websearch`
  const colors = useEndpointColors()

  const [metric, setMetric] = useState<UsageMetric>('spend')
  const [range, setRange] = useState<UsageRangeKey | null>(DEFAULT_RANGE)
  const [{ from, to }, setBounds] = useState(() => rangeBounds(USAGE_RANGE_DAYS[DEFAULT_RANGE]))

  const usage = useWebSearchUsage(orgId, from, to)
  const totals = usage.data?.totals ?? {}
  const byEndpoint = usage.data?.byEndpoint ?? []
  const requests = totals.requests ?? 0
  const spend = totals.costDollars ?? 0

  const chartData = useMemo(
    () => buildDailyRows(usage.data?.days ?? [], from, to, metric),
    [usage.data, from, to, metric],
  )
  const chartSeries = ENDPOINTS.map(endpoint => ({ key: endpoint, label: ENDPOINT_LABELS[endpoint], color: colors[endpoint] }))
  const metricValue = (row: { requests?: number; costDollars?: number }) =>
    metric === 'spend' ? (row.costDollars ?? 0) : (row.requests ?? 0)
  const formatMetric = metric === 'spend' ? formatCost : formatNumber
  const activeDays = new Set((usage.data?.days ?? []).map(day => day.date)).size

  const breakdownRows = ENDPOINTS.map(endpoint => {
    const row = byEndpoint.find(item => item.endpoint === endpoint) ?? {}
    const count = row.requests ?? 0
    return {
      id: endpoint,
      _data: row,
      endpoint: (
        <Stack direction="row" alignItems="center" spacing={1}>
          <Swatch color={colors[endpoint]} />
          <Typography variant="body2" sx={{ fontWeight: 600 }}>{ENDPOINT_LABELS[endpoint]}</Typography>
        </Stack>
      ),
      requests: <Typography variant="body2" color="text.secondary">{formatNumber(count)}</Typography>,
      successRate: <Typography variant="body2" color="text.secondary">{formatPercent(count > 0 ? (row.succeeded ?? 0) / count : null)}</Typography>,
      outOfCredits: <Typography variant="body2" color="text.secondary">{formatNumber(row.outOfCredits)}</Typography>,
      failed: <Typography variant="body2" color="text.secondary">{formatNumber(row.failed)}</Typography>,
      spend: <Typography variant="body2" sx={{ fontWeight: 600 }}>{formatCost(row.costDollars)}</Typography>,
    }
  })

  return (
    <Page
      breadcrumbTitle="Search"
      breadcrumbParent={{
        title: 'Organizations',
        routeName: 'orgs',
        useOrgRouter: false,
      }}
      breadcrumbShowHome={true}
      orgBreadcrumbs={true}
    >
      <Container maxWidth="xl">
        <Box sx={{ mt: 3, pb: 4 }}>
          <Stack spacing={2.5}>
            <UsagePageHeader
              title="Search"
              description="Exa-compatible web search API calls by this organization's keys and agents."
              actions={(
                <UsageRangeControls
                  range={range}
                  from={from}
                  to={to}
                  onRangeChange={next => { setRange(next); setBounds(rangeBounds(USAGE_RANGE_DAYS[next])) }}
                  onFromChange={value => { setRange(null); setBounds(bounds => ({ ...bounds, from: value })) }}
                  onToChange={value => { setRange(null); setBounds(bounds => ({ ...bounds, to: value })) }}
                />
              )}
            />

            {usage.isError ? (
              <Alert severity="error">{usageErrorMessage(usage.error)}</Alert>
            ) : usage.isLoading ? (
              <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
                <CircularProgress />
              </Box>
            ) : requests === 0 ? (
              <Paper variant="outlined" sx={{ p: 3 }}>
                <Typography variant="body1" sx={{ fontWeight: 600 }}>No web search calls in this range</Typography>
                <Typography variant="body2" color="text.secondary" sx={{ mt: 0.5 }}>
                  Make a first search with an organization API key:
                </Typography>
                <MarkdownCodeBlock language="bash">{firstSearchSnippet(baseUrl)}</MarkdownCodeBlock>
              </Paper>
            ) : (
              <>
                <Box sx={{ display: 'grid', gridTemplateColumns: { xs: '1fr', lg: 'minmax(260px, 0.72fr) minmax(0, 1.6fr)' }, gap: 3 }}>
                  <Stack spacing={2.25}>
                    <UsageHeadline
                      label={metric === 'spend' ? 'Web search spend' : 'Web search requests'}
                      value={metric === 'spend' ? formatCost(spend) : formatNumber(requests)}
                      detail={metric === 'spend'
                        ? `Credits charged for ${formatNumber(requests)} requests in this window`
                        : `${formatCost(spend)} charged in this window`}
                    />
                    {ENDPOINTS.map(endpoint => {
                      const row = byEndpoint.find(item => item.endpoint === endpoint) ?? {}
                      const value = metricValue(row)
                      const total = metric === 'spend' ? spend : requests
                      const share = total > 0 ? value / total : 0
                      return (
                        <UsageShareRow
                          key={endpoint}
                          icon={<Swatch color={colors[endpoint]} />}
                          label={ENDPOINT_LABELS[endpoint]}
                          value={formatMetric(value)}
                          share={share}
                          color={colors[endpoint]}
                          caption={`${formatPercent(share)} of ${metric} · ${formatNumber(row.requests)} requests`}
                        />
                      )
                    })}
                  </Stack>

                  <Box>
                    <Stack direction="row" justifyContent="flex-end" sx={{ mb: 1 }}>
                      <ToggleButtonGroup
                        value={metric}
                        exclusive
                        size="small"
                        onChange={(_, next: UsageMetric | null) => next && setMetric(next)}
                      >
                        <ToggleButton value="spend">Spend</ToggleButton>
                        <ToggleButton value="requests">Requests</ToggleButton>
                      </ToggleButtonGroup>
                    </Stack>
                    <ShadcnAreaChart
                      title={metric === 'spend' ? 'DAILY SPEND' : 'DAILY REQUESTS'}
                      headline={metric === 'spend' ? formatCost(spend) : formatNumber(requests)}
                      data={chartData}
                      series={chartSeries}
                      valueFormatter={formatMetric}
                    />
                  </Box>
                </Box>

                <OverviewStatStrip columns={5}>
                  <OverviewStat label="Requests" value={formatNumber(requests)} detail={`${activeDays} ${activeDays === 1 ? 'day' : 'days'} with calls`} />
                  <OverviewStat
                    label="Success rate"
                    value={formatPercent((totals.succeeded ?? 0) / requests)}
                    detail={`${formatNumber(totals.succeeded)} of ${formatNumber(requests)} succeeded`}
                  />
                  <OverviewStat label="Out of credits" value={formatNumber(totals.outOfCredits)} detail="402 responses" />
                  <OverviewStat label="Other errors" value={formatNumber(totals.failed)} detail="Any other 4xx or 5xx" />
                  <OverviewStat label="Cost per request" value={formatCost(spend / requests)} detail="Average across endpoints" />
                </OverviewStatStrip>

                <UsageSection title="Endpoint breakdown">
                  <SimpleTable authenticated fields={BREAKDOWN_FIELDS} data={breakdownRows} compact />
                </UsageSection>

                {orgId && <WebSearchRequestsTable orgId={orgId} from={from} to={to} />}
              </>
            )}

            <UsageSection title="API base URL">
              <MarkdownCodeBlock language="text">{baseUrl}</MarkdownCodeBlock>
            </UsageSection>
          </Stack>
        </Box>
      </Container>
    </Page>
  )
}

export default OrgWebSearchUsage
