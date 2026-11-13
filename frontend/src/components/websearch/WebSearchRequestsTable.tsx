import { FC, MouseEvent, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Chip from '@mui/material/Chip'
import IconButton from '@mui/material/IconButton'
import Paper from '@mui/material/Paper'
import Stack from '@mui/material/Stack'
import Tab from '@mui/material/Tab'
import TablePagination from '@mui/material/TablePagination'
import Tabs from '@mui/material/Tabs'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import Alert from '@mui/material/Alert'
import { Check, Copy } from 'lucide-react'

import SimpleTable, { ITableField } from '../widgets/SimpleTable'
import { UsageSection } from '../usage/UsageOverview'
import { TypesWebSearchEndpoint, TypesWebSearchRequest, TypesWebSearchSource } from '../../api/api'
import { useWebSearchRequests } from '../../services/webSearchUsageService'
import { copyTextToClipboard } from '../../utils/clipboard'
import { APP_MONO_FONT_FAMILY } from '../../styles/typography'
import { formatNumber } from '../../utils/usageFormat'
import { ENDPOINTS, ENDPOINT_LABELS, formatDollars, formatDuration } from './webSearchUsage'

const PAGE_SIZE = 50

// Status, cost and duration come before the identifiers so they stay on
// screen when the table is narrower than all its columns.
const FIELDS: ITableField[] = [
  { name: 'time', title: 'Time' },
  { name: 'endpoint', title: 'Endpoint' },
  { name: 'query', title: 'Query' },
  { name: 'status', title: 'Status' },
  { name: 'cost', title: 'Cost', numeric: true },
  { name: 'duration', title: 'Duration', numeric: true },
  { name: 'results', title: 'Results', numeric: true },
  { name: 'searchType', title: 'Search type' },
  { name: 'source', title: 'Source' },
  { name: 'apiKey', title: 'API key' },
  { name: 'requestId', title: 'Request ID' },
]

const CopyIdButton: FC<{ text: string }> = ({ text }) => {
  const [copied, setCopied] = useState(false)
  const timer = useRef<ReturnType<typeof setTimeout> | null>(null)
  useEffect(() => () => { if (timer.current) clearTimeout(timer.current) }, [])
  const handleCopy = async (e: MouseEvent) => {
    e.stopPropagation()
    await copyTextToClipboard(text)
    setCopied(true)
    if (timer.current) clearTimeout(timer.current)
    timer.current = setTimeout(() => setCopied(false), 1200)
  }
  return (
    <Tooltip title={copied ? 'Copied' : 'Copy request ID'}>
      <IconButton
        aria-label="Copy request ID"
        size="small"
        onClick={handleCopy}
        sx={{ color: 'text.secondary', '&:hover': { color: 'text.primary' } }}
      >
        {copied ? <Check size={14} /> : <Copy size={14} />}
      </IconButton>
    </Tooltip>
  )
}

const StatusBadge: FC<{ status?: number; errorTag?: string }> = ({ status, errorTag }) => {
  const ok = (status ?? 0) > 0 && (status ?? 0) < 400
  const chip = (
    <Chip
      size="small"
      variant="outlined"
      label={status ?? '—'}
      color={ok ? 'success' : status === 402 ? 'warning' : 'error'}
    />
  )
  if (ok || !errorTag) return chip
  return <Tooltip title={errorTag}><span>{chip}</span></Tooltip>
}

const secondary = (text: string) => (
  <Typography variant="body2" color="text.secondary" noWrap>{text}</Typography>
)

const WebSearchRequestsTable: FC<{ orgId: string; from: string; to: string }> = ({ orgId, from, to }) => {
  const [endpoint, setEndpoint] = useState<TypesWebSearchEndpoint | 'all'>('all')
  const [page, setPage] = useState(0)

  useEffect(() => { setPage(0) }, [from, to])

  const requests = useWebSearchRequests(orgId, {
    from,
    to,
    endpoint: endpoint === 'all' ? undefined : endpoint,
    limit: PAGE_SIZE,
    offset: page * PAGE_SIZE,
  })

  const total = requests.data?.total ?? 0

  const tableData = useMemo(() => (requests.data?.requests ?? []).map((req: TypesWebSearchRequest) => {
    const created = req.created ? new Date(req.created) : undefined
    return {
      id: req.id,
      _data: req,
      time: created ? (
        <Tooltip title={`${created.toISOString().replace('T', ' ').slice(0, 19)} UTC`}>
          <Typography variant="body2" noWrap>
            {created.toLocaleString(undefined, { month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', second: '2-digit' })}
          </Typography>
        </Tooltip>
      ) : secondary('—'),
      endpoint: secondary(req.endpoint ? ENDPOINT_LABELS[req.endpoint] : '—'),
      query: (
        <Tooltip title={req.query || ''} placement="top-start">
          <Typography variant="body2" noWrap sx={{ maxWidth: 240 }}>{req.query || '—'}</Typography>
        </Tooltip>
      ),
      requestId: req.id ? (
        <Stack direction="row" alignItems="center" spacing={0.25}>
          <Tooltip title={req.id}>
            <Typography variant="body2" color="text.secondary" noWrap sx={{ fontFamily: APP_MONO_FONT_FAMILY, maxWidth: 96 }}>
              {req.id}
            </Typography>
          </Tooltip>
          <CopyIdButton text={req.id} />
        </Stack>
      ) : secondary('—'),
      source: (
        <Chip
          size="small"
          variant="outlined"
          label={req.source === TypesWebSearchSource.WebSearchSourceAgent ? 'Agent' : 'API'}
        />
      ),
      apiKey: (
        <Tooltip title={req.apiKeyName || ''}>
          <Typography variant="body2" color="text.secondary" noWrap sx={{ maxWidth: 140 }}>{req.apiKeyName || '—'}</Typography>
        </Tooltip>
      ),
      status: <StatusBadge status={req.status} errorTag={req.errorTag} />,
      duration: secondary(formatDuration(req.durationMs)),
      cost: secondary(formatDollars(req.costDollars, 4)),
      searchType: secondary(req.searchType || '—'),
      results: secondary(formatNumber(req.results)),
    }
  }), [requests.data])

  return (
    <Paper variant="outlined" sx={{ p: 2, overflow: 'hidden' }}>
      <UsageSection
        title="Request history"
        action={<Typography variant="caption" color="text.secondary">Local time; hover a time for UTC</Typography>}
      >
        <Tabs
          value={endpoint}
          onChange={(_, next: TypesWebSearchEndpoint | 'all') => { setEndpoint(next); setPage(0) }}
          variant="scrollable"
          allowScrollButtonsMobile
          sx={{ mb: 1, minHeight: 40, '& .MuiTab-root': { minHeight: 40, textTransform: 'none' } }}
        >
          <Tab value="all" label="All" />
          {ENDPOINTS.map(e => <Tab key={e} value={e} label={ENDPOINT_LABELS[e]} />)}
        </Tabs>
        {requests.isError ? (
          <Alert severity="error">Failed to load web search requests.</Alert>
        ) : (
          <>
            <Box sx={{ overflowX: 'auto' }}>
              <SimpleTable
                authenticated
                compact
                fields={FIELDS}
                data={tableData}
                loading={requests.isLoading}
              />
            </Box>
            <TablePagination
              component="div"
              count={total}
              page={page}
              onPageChange={(_, next) => setPage(next)}
              rowsPerPage={PAGE_SIZE}
              rowsPerPageOptions={[PAGE_SIZE]}
            />
          </>
        )}
      </UsageSection>
    </Paper>
  )
}

export default WebSearchRequestsTable
