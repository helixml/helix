import React, { FC } from 'react'
import {
  Box,
  Typography,
  CircularProgress,
  Paper,
  Chip,
} from '@mui/material'
import {
  GitPullRequest,
  ExternalLink,
} from 'lucide-react'

interface PullRequestsProps {
  repository: any
  pullRequests: any[]
  pullRequestsLoading: boolean
}

const PullRequests: FC<PullRequestsProps> = ({
  repository,
  pullRequests,
  pullRequestsLoading,
}) => {
  const handlePRClick = (url: string | undefined) => {
    if (url) {
      window.open(url, '_blank', 'noopener,noreferrer')
    }
  }

  const formatDate = (dateString: string | undefined) => {
    if (!dateString) return 'Unknown date'
    return new Date(dateString).toLocaleString()
  }

  return (
    <Box>
      <Paper variant="outlined" sx={{ borderRadius: 2, overflow: 'hidden' }}>
        {pullRequestsLoading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', alignItems: 'center', py: 8 }}>
            <CircularProgress />
          </Box>
        ) : pullRequests.length === 0 ? (
          <Box sx={{ textAlign: 'center', py: 8 }}>
            <GitPullRequest size={48} color="#656d76" style={{ marginBottom: 16 }} />
            <Typography variant="body2" color="text.secondary">
              No pull requests found in this repository.
            </Typography>
          </Box>
        ) : (
          <Box>
            {pullRequests.map((pr, index) => (
              <Box
                key={pr.id || pr.number || index}
                onClick={() => handlePRClick(pr.url)}
                sx={{
                  borderBottom: index < pullRequests.length - 1 ? '1px solid' : 'none',
                  borderColor: 'divider',
                  p: 2,
                  cursor: pr.url ? 'pointer' : 'default',
                  '&:hover': {
                    backgroundColor: pr.url ? 'rgba(0, 0, 0, 0.02)' : 'transparent',
                  },
                }}
              >
                <Box sx={{ display: 'flex', alignItems: 'flex-start', justifyContent: 'space-between', gap: 2 }}>
                  <Box sx={{ flex: 1, minWidth: 0 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 0.5, flexWrap: 'wrap' }}>
                      <Typography
                        variant="body2"
                        sx={{
                          color: 'text.primary',
                          fontWeight: 600,
                          fontFamily: 'monospace',
                        }}
                      >
                        #{pr.number}
                      </Typography>
                      <Typography
                        variant="body2"
                        sx={{
                          color: 'text.primary',
                          fontWeight: 500,
                        }}
                      >
                        {pr.title || 'Untitled Pull Request'}
                      </Typography>
                      {pr.state && (
                        <Chip
                          label={pr.state}
                          size="small"
                          color={
                            pr.state === 'active' || pr.state === 'open' ? 'success' :
                            pr.state === 'completed' || pr.state === 'merged' ? 'primary' :
                            pr.state === 'abandoned' || pr.state === 'closed' ? 'default' :
                            'default'
                          }
                          sx={{ height: 20, fontSize: '0.75rem' }}
                        />
                      )}
                    </Box>
                    {pr.description && (
                      <Typography
                        variant="body2"
                        sx={{
                          color: 'text.secondary',
                          fontSize: '0.875rem',
                          mb: 1,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          display: '-webkit-box',
                          WebkitLineClamp: 2,
                          WebkitBoxOrient: 'vertical',
                        }}
                      >
                        {pr.description}
                      </Typography>
                    )}
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                      {pr.author && (
                        <>
                          <Typography
                            variant="body2"
                            sx={{
                              color: 'text.secondary',
                              fontSize: '0.8125rem',
                            }}
                          >
                            {pr.author}
                          </Typography>
                          <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: '0.8125rem' }}>
                            •
                          </Typography>
                        </>
                      )}
                      <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: '0.8125rem' }}>
                        Created {formatDate(pr.created_at)}
                      </Typography>
                      {pr.source_branch && pr.target_branch && (
                        <>
                          <Typography variant="body2" sx={{ color: 'text.secondary', fontSize: '0.8125rem' }}>
                            •
                          </Typography>
                          <Typography
                            variant="body2"
                            sx={{
                              color: 'text.secondary',
                              fontSize: '0.8125rem',
                              fontFamily: 'monospace',
                            }}
                          >
                            {pr.source_branch} → {pr.target_branch}
                          </Typography>
                        </>
                      )}
                    </Box>
                  </Box>
                  {pr.url && (
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5, flexShrink: 0 }}>
                      <ExternalLink size={16} style={{ color: 'currentColor', opacity: 0.6 }} />
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

export default PullRequests

