import React, { FC, useState, useCallback } from 'react'
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
} from '@mui/material'
import ArrowBackIcon from '@mui/icons-material/ArrowBack'
import SyncIcon from '@mui/icons-material/Sync'
import RefreshIcon from '@mui/icons-material/Refresh'

import useRouter from '../../hooks/useRouter'
import useSnackbar from '../../hooks/useSnackbar'
import {
  useAdminKoditRepositoryDetail,
  useAdminSyncKoditRepository,
  useAdminRescanKoditRepository,
} from '../../services/koditAdminService'
import {
  useKoditSearch,
  useKoditEnrichments,
} from '../../services/koditService'

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
}

const KoditAdminRepoDetail: FC<KoditAdminRepoDetailProps> = ({ koditRepoId }) => {
  const router = useRouter()
  const snackbar = useSnackbar()
  const [searchQuery, setSearchQuery] = useState('')
  const [activeSearch, setActiveSearch] = useState('')

  const { data, isLoading, error } = useAdminKoditRepositoryDetail(koditRepoId)
  const syncMutation = useAdminSyncKoditRepository()
  const rescanMutation = useAdminRescanKoditRepository()

  const attrs = data?.data?.attributes
  const helixRepoId = attrs?.helix_repo_id || ''

  // Enrichments for this repo (if we have a Helix repo ID to cross-reference)
  const enrichmentsQuery = useKoditEnrichments(helixRepoId, undefined, {
    enabled: !!helixRepoId,
    refetchInterval: false,
  })

  // Search (if we have a Helix repo ID)
  const searchResults = useKoditSearch(helixRepoId, activeSearch, {
    enabled: !!helixRepoId && !!activeSearch,
  })

  const handleBack = useCallback(() => {
    router.setParams({ tab: 'kodit' })
    router.removeParams(['repo_id'])
  }, [router])

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

      {/* Remote URL */}
      <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
        {attrs.remote_url}
      </Typography>

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
            <Typography variant="caption" color="text.secondary">Default Branch</Typography>
            <Typography variant="body2">{attrs.default_branch || '-'}</Typography>
          </Grid>
          <Grid item xs={6}>
            <Typography variant="caption" color="text.secondary">Helix Repository</Typography>
            <Typography variant="body2">{attrs.helix_repo_name || '-'}</Typography>
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

      {/* Search Test Panel */}
      {helixRepoId && (
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
          {searchResults.data && Array.isArray(searchResults.data) && searchResults.data.length > 0 && (
            <Paper variant="outlined" sx={{ maxHeight: 400, overflow: 'auto' }}>
              {searchResults.data.map((result: any, i: number) => (
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
          )}
          {searchResults.data && Array.isArray(searchResults.data) && searchResults.data.length === 0 && activeSearch && (
            <Typography variant="body2" color="text.secondary">No results found</Typography>
          )}
        </Box>
      )}

      {/* Enrichments Preview */}
      {helixRepoId && enrichmentsQuery.data && (
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
