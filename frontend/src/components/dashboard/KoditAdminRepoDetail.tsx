import React, { FC, useState, useCallback, useMemo } from 'react'
import {
  Box,
  Typography,
  Button,
  Chip,
  CircularProgress,
  Paper,
  Grid,
  TextField,
  Card,
  CardContent,
  Divider,
  Table,
  TableBody,
  TableCell,
  TableContainer,
  TableHead,
  TableRow,
  LinearProgress,
  Accordion,
  AccordionSummary,
  AccordionDetails,
} from '@mui/material'
import ArrowBackIcon from '@mui/icons-material/ArrowBack'
import SyncIcon from '@mui/icons-material/Sync'
import RefreshIcon from '@mui/icons-material/Refresh'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import OpenInNewIcon from '@mui/icons-material/OpenInNew'

import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import {
  useAdminKoditRepositoryDetail,
  useAdminKoditRepositoryTasks,
  useAdminSyncKoditRepository,
  useAdminRescanKoditRepository,
  useAdminKoditRepoEnrichments,
  useAdminKoditRepoSearch,
} from '../../services/koditAdminService'

const statusColor: Record<string, 'success' | 'warning' | 'error' | 'default' | 'info'> = {
  cloned: 'success',
  syncing: 'info',
  cloning: 'info',
  pending: 'default',
  failed: 'error',
  deleting: 'warning',
  idle: 'default',
  processing: 'info',
  completed: 'success',
  in_progress: 'info',
  completed_with_errors: 'warning',
}

const stateColor: Record<string, 'success' | 'warning' | 'error' | 'default' | 'info'> = {
  started: 'info',
  in_progress: 'info',
  completed: 'success',
  failed: 'error',
  skipped: 'default',
}

const formatOperation = (op: string): string => {
  // "kodit.commit.extract_snippets" -> "Extract Snippets"
  const parts = op.split('.')
  const last = parts[parts.length - 1] || op
  return last.replace(/_/g, ' ').replace(/\b\w/g, c => c.toUpperCase())
}

interface StatCardProps {
  label: string
  value: string | number
}

const StatCard: FC<StatCardProps> = ({ label, value }) => (
  <Card variant="outlined">
    <CardContent sx={{ textAlign: 'center', py: 2, '&:last-child': { pb: 2 } }}>
      <Typography variant="h4" fontWeight={600}>{value}</Typography>
      <Typography variant="caption" color="text.secondary">{label}</Typography>
    </CardContent>
  </Card>
)

interface KoditAdminRepoDetailProps {
  koditRepoId: string
  onBack?: () => void
}

const KoditAdminRepoDetail: FC<KoditAdminRepoDetailProps> = ({ koditRepoId, onBack }) => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const [searchQuery, setSearchQuery] = useState('')
  const [activeSearch, setActiveSearch] = useState('')

  const { data, isLoading, error } = useAdminKoditRepositoryDetail(koditRepoId)
  const { data: tasksData } = useAdminKoditRepositoryTasks(koditRepoId)
  const syncMutation = useAdminSyncKoditRepository()
  const rescanMutation = useAdminRescanKoditRepository()

  const attrs = data?.data?.attributes
  const helixRepoId = attrs?.helix_repo_id || ''

  // Enrichments and search use admin endpoints keyed by kodit repo ID
  // (works for both git repos and knowledge-based repos)
  const enrichmentsQuery = useAdminKoditRepoEnrichments(koditRepoId)
  const searchResults = useAdminKoditRepoSearch(koditRepoId, activeSearch, {
    enabled: !!activeSearch,
  })

  const handleBack = useCallback(() => {
    if (onBack) {
      onBack()
    }
  }, [onBack])

  const handleSync = useCallback(() => {
    syncMutation.mutate(Number(koditRepoId), {
      onSuccess: () => snackbar.success('Sync triggered'),
      onError: (err) => snackbar.error(`Sync failed: ${err.message}`),
    })
  }, [koditRepoId, syncMutation, snackbar])

  const handleRescan = useCallback(() => {
    rescanMutation.mutate(Number(koditRepoId), {
      onSuccess: () => snackbar.success('Rescan triggered'),
      onError: (err) => snackbar.error(`Rescan failed: ${err.message}`),
    })
  }, [koditRepoId, rescanMutation, snackbar])

  const handleSearch = useCallback(() => {
    setActiveSearch(searchQuery)
  }, [searchQuery])

  const taskSummary = useMemo(() => {
    const statuses = tasksData?.statuses || []
    const pendingCount = tasksData?.pending_tasks?.length || 0
    if (statuses.length === 0 && pendingCount === 0) return ''

    // Count statuses by state, using raw state values
    const counts: Record<string, number> = {}
    for (const s of statuses) {
      const state = s.state || 'unknown'
      counts[state] = (counts[state] || 0) + 1
    }

    const parts: string[] = []
    for (const [state, count] of Object.entries(counts)) {
      parts.push(`${count} ${state}`)
    }
    if (pendingCount > 0) {
      parts.push(`${pendingCount} pending in queue`)
    }
    return parts.join(', ')
  }, [tasksData])

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
        <CircularProgress />
      </Box>
    )
  }

  if (error) {
    return (
      <Box sx={{ py: 4 }}>
        <Button startIcon={<ArrowBackIcon />} onClick={handleBack} sx={{ mb: 2 }}>
          Back to list
        </Button>
        <Typography color="error">
          Failed to load repository: {(error as Error).message}
        </Typography>
      </Box>
    )
  }

  if (!attrs) return null

  const repoDisplayName = attrs.helix_repo_name || attrs.remote_url || koditRepoId

  return (
    <Box>
      {/* Header */}
      <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, mb: 3 }}>
        <Button startIcon={<ArrowBackIcon />} onClick={handleBack} size="small">
          Back
        </Button>
        <Typography variant="h6" sx={{ flex: 1 }}>{repoDisplayName}</Typography>
        <Chip
          label={attrs.status || 'unknown'}
          color={statusColor[attrs.status || ''] || 'default'}
          size="small"
        />
      </Box>

      {/* Link to Helix repo page */}
      {helixRepoId ? (
        <Button
          size="small"
          startIcon={<OpenInNewIcon />}
          onClick={() => account.orgNavigate('git-repo-detail', { repoId: helixRepoId })}
          sx={{ mb: 3, textTransform: 'none' }}
        >
          View in Repository
        </Button>
      ) : (
        <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
          No linked Helix repository
        </Typography>
      )}

      {/* Status message from Kodit tracking */}
      {attrs.status_message && (
        <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
          <Typography variant="body2" color="text.secondary">
            {attrs.status_message}
          </Typography>
        </Paper>
      )}

      {/* Summary Stats */}
      <Grid container spacing={2} sx={{ mb: 3 }}>
        <Grid item xs={6} sm={3}>
          <StatCard label="Branches" value={attrs.branch_count ?? 0} />
        </Grid>
        <Grid item xs={6} sm={3}>
          <StatCard label="Tags" value={attrs.tag_count ?? 0} />
        </Grid>
        <Grid item xs={6} sm={3}>
          <StatCard label="Commits" value={attrs.commit_count ?? 0} />
        </Grid>
        <Grid item xs={6} sm={3}>
          <StatCard label="Enrichments" value={attrs.enrichment_count ?? 0} />
        </Grid>
      </Grid>

      {/* Info */}
      <Paper variant="outlined" sx={{ p: 2, mb: 3 }}>
        <Grid container spacing={2}>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary">Kodit Repository ID</Typography>
            <Typography variant="body2">{data?.data?.id || '-'}</Typography>
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary">Default Branch</Typography>
            <Typography variant="body2">{attrs.default_branch || '-'}</Typography>
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary">Organization</Typography>
            <Typography variant="body2">{attrs.helix_org_name || '-'}</Typography>
            {attrs.helix_org_id && (
              <Typography variant="caption" color="text.secondary" sx={{ fontFamily: 'monospace' }}>
                {attrs.helix_org_id}
              </Typography>
            )}
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary">Helix Repository ID</Typography>
            <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
              {attrs.helix_repo_id || '-'}
            </Typography>
          </Grid>
          <Grid item xs={12}>
            <Typography variant="caption" color="text.secondary">Latest Commit</Typography>
            {attrs.latest_commit_sha ? (
              <Box>
                <Typography variant="body2" sx={{ fontFamily: 'monospace', fontSize: '0.75rem' }}>
                  {attrs.latest_commit_sha}
                </Typography>
                <Typography variant="body2" color="text.secondary">
                  {attrs.latest_commit_message}
                  {attrs.latest_commit_author && ` — ${attrs.latest_commit_author}`}
                  {attrs.latest_commit_date && `, ${new Date(attrs.latest_commit_date).toLocaleString()}`}
                </Typography>
              </Box>
            ) : (
              <Typography variant="body2">-</Typography>
            )}
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary">Created</Typography>
            <Typography variant="body2">
              {attrs.created_at ? new Date(attrs.created_at).toLocaleString() : '-'}
            </Typography>
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary">Updated</Typography>
            <Typography variant="body2">
              {attrs.updated_at ? new Date(attrs.updated_at).toLocaleString() : '-'}
            </Typography>
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary">Last Scanned</Typography>
            <Typography variant="body2">
              {attrs.last_scanned_at ? new Date(attrs.last_scanned_at).toLocaleString() : '-'}
            </Typography>
          </Grid>
        </Grid>
      </Paper>

      {/* Actions */}
      <Box sx={{ display: 'flex', gap: 1, mb: 3 }}>
        <Button
          variant="outlined"
          startIcon={<SyncIcon />}
          onClick={handleSync}
          disabled={syncMutation.isPending}
        >
          Sync
        </Button>
        <Button
          variant="outlined"
          startIcon={<RefreshIcon />}
          onClick={handleRescan}
          disabled={rescanMutation.isPending}
        >
          Rescan HEAD
        </Button>
      </Box>

      <Divider sx={{ mb: 3 }} />

      {/* Task Activity (collapsible) */}
      {tasksData && taskSummary && (
        <Accordion variant="outlined" sx={{ mb: 3 }}>
          <AccordionSummary expandIcon={<ExpandMoreIcon />}>
            <Box>
              <Typography variant="subtitle1">Task Activity</Typography>
              <Typography variant="body2" color="text.secondary">{taskSummary}</Typography>
            </Box>
          </AccordionSummary>
          <AccordionDetails>
            {/* Tracking statuses */}
            {tasksData.statuses && tasksData.statuses.length > 0 && (
              <TableContainer sx={{ mb: 2 }}>
                <Table size="small">
                  <TableHead>
                    <TableRow>
                      <TableCell>Operation</TableCell>
                      <TableCell>State</TableCell>
                      <TableCell>Progress</TableCell>
                      <TableCell>Updated</TableCell>
                    </TableRow>
                  </TableHead>
                  <TableBody>
                    {tasksData.statuses.map((status, i) => (
                      <TableRow key={`${status.operation}-${i}`}>
                        <TableCell>
                          <Typography variant="body2">{formatOperation(status.operation || '')}</Typography>
                          <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                            {status.operation}
                          </Typography>
                        </TableCell>
                        <TableCell>
                          <Chip
                            label={status.state}
                            size="small"
                            color={stateColor[status.state || ''] || 'default'}
                          />
                          {status.error && (
                            <Typography variant="caption" color="error" sx={{ display: 'block', mt: 0.5 }}>
                              {status.error}
                            </Typography>
                          )}
                          {status.message && !status.error && (
                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block', mt: 0.5 }}>
                              {status.message}
                            </Typography>
                          )}
                        </TableCell>
                        <TableCell sx={{ minWidth: 120 }}>
                          {(status.total ?? 0) > 0 ? (
                            <Box>
                              <LinearProgress
                                variant="determinate"
                                value={Math.round(((status.current ?? 0) / status.total!) * 100)}
                                sx={{ mb: 0.5 }}
                              />
                              <Typography variant="caption" color="text.secondary">
                                {status.current} / {status.total}
                              </Typography>
                            </Box>
                          ) : (
                            <Typography variant="caption" color="text.secondary">-</Typography>
                          )}
                        </TableCell>
                        <TableCell>
                          <Typography variant="caption">
                            {status.updated_at ? new Date(status.updated_at).toLocaleTimeString() : '-'}
                          </Typography>
                        </TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </TableContainer>
            )}

            {/* Pending queue tasks */}
            {tasksData.pending_tasks && tasksData.pending_tasks.length > 0 && (
              <Box>
                <Typography variant="body2" color="text.secondary" sx={{ mb: 1 }}>
                  Pending Tasks ({tasksData.pending_tasks.length})
                </Typography>
                <TableContainer>
                  <Table size="small">
                    <TableHead>
                      <TableRow>
                        <TableCell>Operation</TableCell>
                        <TableCell>Priority</TableCell>
                        <TableCell>Queued</TableCell>
                      </TableRow>
                    </TableHead>
                    <TableBody>
                      {tasksData.pending_tasks.map((task) => (
                        <TableRow key={task.id}>
                          <TableCell>
                            <Typography variant="body2">{formatOperation(task.operation || '')}</Typography>
                            <Typography variant="caption" color="text.secondary" sx={{ display: 'block' }}>
                              {task.operation}
                            </Typography>
                          </TableCell>
                          <TableCell>
                            <Typography variant="body2">{task.priority}</Typography>
                          </TableCell>
                          <TableCell>
                            <Typography variant="caption">
                              {task.created_at ? new Date(task.created_at).toLocaleTimeString() : '-'}
                            </Typography>
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </TableContainer>
              </Box>
            )}
          </AccordionDetails>
        </Accordion>
      )}

      {/* Search Test Panel */}
      {koditRepoId && (
        <Box sx={{ mb: 3 }}>
          <Typography variant="subtitle1" gutterBottom>Search Test</Typography>
          <Box sx={{ display: 'flex', gap: 1, mb: 2 }}>
            <TextField
              size="small"
              placeholder="Search code snippets..."
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
              sx={{ flex: 1 }}
            />
            <Button variant="contained" onClick={handleSearch} disabled={!searchQuery}>
              Search
            </Button>
          </Box>
          {searchResults.isLoading && <CircularProgress size={20} />}
          {(() => {
            const results = (searchResults.data as any)?.data || []
            if (results.length > 0) return (
              <Paper variant="outlined" sx={{ maxHeight: 400, overflow: 'auto' }}>
                {results.map((result: any, i: number) => (
                  <Box key={result.id || i} sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider' }}>
                    <Box sx={{ display: 'flex', gap: 1, mb: 1 }}>
                      <Chip label={result.type} size="small" />
                      {result.language && <Chip label={result.language} size="small" variant="outlined" />}
                    </Box>
                    <Typography
                      variant="body2"
                      component="pre"
                      sx={{
                        whiteSpace: 'pre-wrap',
                        fontFamily: 'monospace',
                        fontSize: '0.75rem',
                        maxHeight: 200,
                        overflow: 'auto',
                      }}
                    >
                      {result.content}
                    </Typography>
                  </Box>
                ))}
              </Paper>
            )
            if (searchResults.data && activeSearch) return (
              <Typography variant="body2" color="text.secondary">No results found</Typography>
            )
            return null
          })()}
        </Box>
      )}

      {/* Enrichments Preview */}
      {koditRepoId && enrichmentsQuery.data && (
        <Box>
          <Typography variant="subtitle1" gutterBottom>
            Enrichments Preview ({(enrichmentsQuery.data as any)?.data?.length || 0})
          </Typography>
          <Paper variant="outlined" sx={{ maxHeight: 400, overflow: 'auto' }}>
            {((enrichmentsQuery.data as any)?.data || []).slice(0, 20).map((enrichment: any) => (
              <Box key={enrichment.id} sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider' }}>
                <Box sx={{ display: 'flex', gap: 1, mb: 0.5 }}>
                  <Chip label={enrichment.attributes?.type || enrichment.type} size="small" />
                  {enrichment.attributes?.subtype && (
                    <Chip label={enrichment.attributes.subtype} size="small" variant="outlined" />
                  )}
                </Box>
                <Typography
                  variant="body2"
                  sx={{
                    whiteSpace: 'pre-wrap',
                    fontSize: '0.75rem',
                    maxHeight: 100,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                  }}
                >
                  {enrichment.attributes?.content?.slice(0, 300) || ''}
                  {enrichment.attributes?.content?.length > 300 ? '...' : ''}
                </Typography>
              </Box>
            ))}
          </Paper>
        </Box>
      )}
    </Box>
  )
}

export default KoditAdminRepoDetail
