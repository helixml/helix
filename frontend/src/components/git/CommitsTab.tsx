import React, { FC } from 'react'
import {
  Box,
  Typography,
  CircularProgress,
  IconButton,
  Tooltip,
  Paper,
} from '@mui/material'
import {
  GitCommit,
  Copy,
} from 'lucide-react'
import BranchSelect from './BranchSelect'

interface CommitsTabProps {
  repository: any
  commitsBranch: string
  setCommitsBranch: (branch: string) => void
  branches: string[]
  commits: any[]
  commitsLoading: boolean
  handleCopySha: (sha: string) => void
  copiedSha: string | null
}

const CommitsTab: FC<CommitsTabProps> = ({
  repository,
  commitsBranch,
  setCommitsBranch,
  branches,
  commits,
  commitsLoading,
  handleCopySha,
  copiedSha,
}) => {
  return (
    <Box>
      <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
        <Box sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 2,
          p: 2,
          borderBottom: 1,
          borderColor: 'divider',
          bgcolor: 'rgba(0, 0, 0, 0.02)'
        }}>
          <BranchSelect
            repository={repository}
            currentBranch={commitsBranch === repository?.default_branch ? '' : commitsBranch}
            setCurrentBranch={(branch) => {
              setCommitsBranch(branch === '' ? '' : branch)
            }}
            branches={branches}
          />
        </Box>
        {commitsLoading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', py: 8 }}>
            <CircularProgress />
          </Box>
        ) : commits.length === 0 ? (
          <Box sx={{ textAlign: 'center', py: 8 }}>
            <GitCommit size={48} color="#656d76" style={{ marginBottom: 16 }} />
            <Typography variant="body2" color="text.secondary">
              No commits found in this repository.
            </Typography>
          </Box>
        ) : (
          <Box>
            {commits.map((commit, index) => (
              <Box
                key={commit.sha || index}
                sx={{
                  borderBottom: index < commits.length - 1 ? '1px solid' : 'none',
                  borderColor: 'divider',
                  p: 2,
                  '&:hover': {
                    backgroundColor: 'rgba(0, 0, 0, 0.02)',
                  },
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 2 }}>
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Tooltip
                      title={commit.message || ''}
                      placement="top"
                      arrow
                    >
                      <Typography
                        variant="body2"
                        sx={{
                          color: 'text.primary',
                          fontWeight: 500,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                          mb: 0.5,
                        }}
                      >
                        {commit.message || 'No commit message'}
                      </Typography>
                    </Tooltip>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                      <Tooltip
                        title={commit.email ? `Email: ${commit.email}` : ''}
                        placement="top"
                        arrow
                      >
                        <Typography
                          variant="body2"
                          sx={{
                            color: 'text.secondary',
                            fontSize: '0.8125rem',
                            cursor: 'default',
                          }}
                        >
                          {commit.author || 'Unknown'}
                        </Typography>
                      </Tooltip>
                      {commit.timestamp && (
                        <>
                          <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: '0.8125rem' }}>
                            •
                          </Typography>
                          <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: '0.8125rem' }}>
                            {new Date(commit.timestamp).toLocaleString()}
                          </Typography>
                        </>
                      )}
                    </Box>
                  </Box>
                  {commit.sha && (
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }}>
                      <Typography
                        variant="body2"
                        sx={{
                          fontFamily: 'monospace',
                          fontSize: '0.8125rem',
                          color: 'text.secondary',
                        }}
                      >
                        {commit.sha.substring(0, 7)}
                      </Typography>
                      <Tooltip
                        title={copiedSha === commit.sha ? 'Copied!' : 'Copy full SHA'}
                        placement="top"
                        arrow
                      >
                        <IconButton
                          size="small"
                          onClick={() => handleCopySha(commit.sha!)}
                          sx={{
                            p: 0.5,
                            color: 'text.secondary',
                            '&:hover': {
                              backgroundColor: 'rgba(0, 0, 0, 0.04)',
                              color: 'text.primary',
                            },
                          }}
                        >
                          <Copy size={14} />
                        </IconButton>
                      </Tooltip>
                    </Box>
                  )}
                </Box>
              </Box>
            ))}
          </Box>
        )}
      </Paper>
    </Box>
  )
}

export default CommitsTab

