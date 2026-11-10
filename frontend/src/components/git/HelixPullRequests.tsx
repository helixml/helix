import React, { FC, useMemo, useState } from 'react'
import {
  Alert,
  Box,
  Button,
  Chip,
  CircularProgress,
  IconButton,
  Paper,
  Tab,
  Tabs,
  Tooltip,
  Typography,
} from '@mui/material'
import { ArrowLeft, GitMerge, GitPullRequest, GitPullRequestClosed, ListTodo, RefreshCw } from 'lucide-react'

import type { TypesGitPullRequest, TypesGitRepository, TypesInternalPRFile } from '../../api/api'
import {
  useHelixPullRequest,
  useHelixPullRequests,
  useMergeHelixPullRequest,
  useRequestHelixPullRequestRebase,
  useSetHelixPullRequestState,
} from '../../services/gitRepositoryService'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import { FileDiff } from '../../hooks/useLiveFileDiff'
import InteractionMarkdown from '../session/Markdown'
import DiffFileList from '../tasks/DiffFileList'
import DiffContent from '../tasks/DiffContent'

type PRStateLabel = 'open' | 'merged' | 'closed'

const prStateLabel = (pr?: TypesGitPullRequest): PRStateLabel => {
  if (pr?.merged) return 'merged'
  return pr?.state === 'open' ? 'open' : 'closed'
}

const PRStateChip: FC<{ pr?: TypesGitPullRequest }> = ({ pr }) => {
  const label = prStateLabel(pr)
  const Icon = label === 'merged' ? GitMerge : label === 'open' ? GitPullRequest : GitPullRequestClosed
  return (
    <Chip
      icon={<Icon size={14} />}
      label={label === 'merged' ? 'Merged' : label === 'open' ? 'Open' : 'Closed'}
      size="small"
      color={label === 'merged' ? 'secondary' : label === 'open' ? 'success' : 'default'}
      sx={{ height: 22 }}
    />
  )
}

const errorMessage = (err: unknown): string => {
  const e = err as { response?: { data?: { error?: string } | string }; message?: string }
  const data = e?.response?.data
  if (typeof data === 'string') return data
  return data?.error || e?.message || 'Request failed'
}

const toFileDiff = (f: TypesInternalPRFile): FileDiff => ({
  path: f.filename || '',
  old_path: f.previous_filename,
  status: f.status === 'removed' ? 'deleted' : (f.status as FileDiff['status']) || 'modified',
  additions: f.additions || 0,
  deletions: f.deletions || 0,
  diff: f.patch,
  is_binary: !f.patch && !f.additions && !f.deletions,
})

const formatDate = (d?: string) => (d ? new Date(d).toLocaleString() : '')

interface HelixPullRequestsProps {
  repository: TypesGitRepository
}

// Pull requests on a Helix-hosted repository: a list, or one pull request
// when the `pr` URL parameter is set.
const HelixPullRequests: FC<HelixPullRequestsProps> = ({ repository }) => {
  const router = useRouter()
  const number = parseInt(router.params.pr || '', 10)
  if (number > 0) {
    return (
      <HelixPullRequestDetail
        repositoryId={repository.id || ''}
        number={number}
        onBack={() => router.removeParams(['pr'])}
      />
    )
  }
  return (
    <HelixPullRequestList
      repositoryId={repository.id || ''}
      onOpen={(n) => router.mergeParams({ pr: String(n) })}
    />
  )
}

const HelixPullRequestList: FC<{ repositoryId: string; onOpen: (n: number) => void }> = ({ repositoryId, onOpen }) => {
  const [state, setState] = useState<'open' | 'closed'>('open')
  const { data: pulls = [], isLoading } = useHelixPullRequests(repositoryId, state)

  return (
    <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
      <Tabs value={state} onChange={(_, v) => setState(v)} sx={{ px: 2, borderBottom: 1, borderColor: 'divider' }}>
        <Tab value="open" label="Open" />
        <Tab value="closed" label="Closed" />
      </Tabs>
      {isLoading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
          <CircularProgress />
        </Box>
      ) : pulls.length === 0 ? (
        <Box sx={{ textAlign: 'center', py: 8 }}>
          <GitPullRequest size={48} color="#656d76" style={{ marginBottom: 16 }} />
          <Typography variant="body2" color="text.secondary">
            {state === 'open' ? 'No open pull requests.' : 'No closed pull requests.'}
          </Typography>
        </Box>
      ) : (
        pulls.map((pr, i) => (
          <Box
            key={pr.id}
            component="button"
            type="button"
            onClick={() => onOpen(pr.number || 0)}
            sx={{
              display: 'block',
              width: '100%',
              textAlign: 'left',
              border: 0,
              background: 'transparent',
              color: 'inherit',
              font: 'inherit',
              cursor: 'pointer',
              p: 2,
              borderBottom: i < pulls.length - 1 ? '1px solid' : 'none',
              borderColor: 'divider',
              '&:hover': { backgroundColor: 'action.hover' },
            }}
          >
            <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5, flexWrap: 'wrap' }}>
              <Typography variant="body2" sx={{ fontWeight: 600, fontFamily: 'monospace' }}>
                #{pr.number}
              </Typography>
              <Typography variant="body2" sx={{ fontWeight: 500 }}>
                {pr.title}
              </Typography>
              <PRStateChip pr={pr} />
            </Box>
            <Typography variant="body2" color="text.secondary" sx={{ fontSize: '0.8125rem' }}>
              {pr.author_login} • {formatDate(pr.created_at)} •{' '}
              <Box component="span" sx={{ fontFamily: 'monospace' }}>
                {pr.head_branch} → {pr.base_branch}
              </Box>
            </Typography>
          </Box>
        ))
      )}
    </Paper>
  )
}

const HelixPullRequestDetail: FC<{ repositoryId: string; number: number; onBack: () => void }> = ({
  repositoryId,
  number,
  onBack,
}) => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const { data: detail, isLoading, error } = useHelixPullRequest(repositoryId, number)
  const merge = useMergeHelixPullRequest(repositoryId)
  const setState = useSetHelixPullRequestState(repositoryId)
  const rebase = useRequestHelixPullRequestRebase(repositoryId)
  const [tab, setTab] = useState<'conversation' | 'files'>('files')
  const [selected, setSelected] = useState<string | null>(null)

  const files = useMemo(() => (detail?.files || []).map(toFileDiff), [detail?.files])
  const selectedFile = files.find((f) => f.path === selected) || files[0] || null

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
        <CircularProgress />
      </Box>
    )
  }
  if (error || !detail?.pull_request) {
    return <Alert severity="error">Could not load pull request #{number}: {errorMessage(error)}</Alert>
  }

  const pr = detail.pull_request
  const state = prStateLabel(pr)
  const conflicted = state === 'open' && detail.mergeable === false

  const run = async (action: () => Promise<unknown>, success: string) => {
    try {
      await action()
      snackbar.success(success)
    } catch (err) {
      snackbar.error(errorMessage(err))
    }
  }

  return (
    <Box>
      <Box sx={{ display: 'flex', alignItems: 'flex-start', gap: 1, mb: 2 }}>
        <Tooltip title="All pull requests">
          <IconButton aria-label="All pull requests" onClick={onBack} sx={{ width: 30, height: 30, mt: 0.25 }}>
            <ArrowLeft size={18} />
          </IconButton>
        </Tooltip>
        <Box sx={{ flex: 1, minWidth: 0 }}>
          <Typography variant="h6" sx={{ fontWeight: 500 }}>
            {pr.title}{' '}
            <Box component="span" sx={{ color: 'text.secondary', fontWeight: 400 }}>
              #{pr.number}
            </Box>
          </Typography>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5, flexWrap: 'wrap' }}>
            <PRStateChip pr={pr} />
            <Typography variant="body2" color="text.secondary">
              {state === 'merged' ? 'Merged' : `${pr.author_login} ${state === 'open' ? 'wants to merge' : 'proposed merging'}`}{' '}
              <Box component="span" sx={{ fontFamily: 'monospace' }}>
                {pr.head_branch}
              </Box>{' '}
              into{' '}
              <Box component="span" sx={{ fontFamily: 'monospace' }}>
                {pr.base_branch}
              </Box>
              {pr.merged && pr.merged_at
                ? ` on ${formatDate(pr.merged_at)}`
                : state === 'closed' && pr.closed_at
                  ? ` • closed ${formatDate(pr.closed_at)}`
                  : ''}
            </Typography>
            {detail.spec_task_id && detail.spec_task_project_id && (
              <Button
                size="small"
                startIcon={<ListTodo size={16} />}
                onClick={() =>
                  account.orgNavigate('project-task-detail', {
                    id: detail.spec_task_project_id,
                    taskId: detail.spec_task_id,
                  })
                }
                sx={{ textTransform: 'none', minWidth: 0, maxWidth: 420, py: 0 }}
              >
                <Box component="span" sx={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
                  {detail.spec_task_name || detail.spec_task_id}
                </Box>
              </Button>
            )}
          </Box>
        </Box>
        {state === 'open' && (
          <Box sx={{ display: 'flex', gap: 1, flexShrink: 0 }}>
            <Button
              variant="outlined"
              color="inherit"
              disabled={setState.isPending}
              onClick={() => run(() => setState.mutateAsync({ number, state: 'closed' }), 'Pull request closed')}
            >
              Close
            </Button>
            <Tooltip title={conflicted ? `Conflicts with ${pr.base_branch}` : ''}>
              <span>
                <Button
                  variant="contained"
                  color="secondary"
                  startIcon={merge.isPending ? <CircularProgress size={16} /> : <GitMerge size={18} />}
                  disabled={conflicted || merge.isPending}
                  onClick={() => run(() => merge.mutateAsync(number), 'Pull request merged')}
                >
                  Merge
                </Button>
              </span>
            </Tooltip>
          </Box>
        )}
        {state === 'closed' && (
          <Button
            variant="outlined"
            disabled={setState.isPending}
            onClick={() => run(() => setState.mutateAsync({ number, state: 'open' }), 'Pull request reopened')}
          >
            Reopen
          </Button>
        )}
      </Box>

      {conflicted && (
        <Alert
          severity="warning"
          sx={{ mb: 2 }}
          action={
            detail.spec_task_id ? (
              <Button
                color="inherit"
                size="small"
                startIcon={<RefreshCw size={16} />}
                disabled={rebase.isPending}
                onClick={() => run(() => rebase.mutateAsync(number), 'Asked the agent to rebase')}
              >
                Ask agent to rebase
              </Button>
            ) : undefined
          }
        >
          This branch has conflicts with {pr.base_branch}. Rebase {pr.head_branch} onto {pr.base_branch} to merge it.
        </Alert>
      )}

      <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
        <Tabs value={tab} onChange={(_, v) => setTab(v)} sx={{ px: 2, borderBottom: 1, borderColor: 'divider' }}>
          <Tab value="conversation" label="Description" />
          <Tab value="files" label={`Files changed (${files.length})`} />
        </Tabs>
        {tab === 'conversation' ? (
          <Box sx={{ p: 2 }}>
            {pr.body ? (
              <InteractionMarkdown text={pr.body} />
            ) : (
              <Typography variant="body2" color="text.secondary">
                No description.
              </Typography>
            )}
          </Box>
        ) : files.length === 0 ? (
          <Box sx={{ p: 4, textAlign: 'center' }}>
            <Typography variant="body2" color="text.secondary">
              No changes.
            </Typography>
          </Box>
        ) : (
          <Box sx={{ display: 'flex', minHeight: 400, maxHeight: '70vh' }}>
            <Box sx={{ width: 300, flexShrink: 0, borderRight: 1, borderColor: 'divider', overflow: 'auto' }}>
              <DiffFileList files={files} selectedFile={selectedFile?.path || null} onSelectFile={setSelected} />
            </Box>
            <Box sx={{ flex: 1, minWidth: 0, overflow: 'auto' }}>
              <DiffContent file={selectedFile} />
            </Box>
          </Box>
        )}
      </Paper>
    </Box>
  )
}

export default HelixPullRequests
