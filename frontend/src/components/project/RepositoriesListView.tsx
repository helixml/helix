import React, { FC } from 'react'
import {
  Box,
  Button,
  Card,
  CardContent,
  Typography,
  TextField,
  InputAdornment,
  Pagination,
  Chip,
} from '@mui/material'
import SearchIcon from '@mui/icons-material/Search'
import { GitBranch, Link, Brain } from 'lucide-react'

import type { ServicesGitRepository } from '../../api/api'

interface RepositoriesListViewProps {
  repositories: ServicesGitRepository[]
  ownerSlug: string
  searchQuery: string
  onSearchChange: (query: string) => void
  page: number
  onPageChange: (page: number) => void
  filteredRepositories: ServicesGitRepository[]
  paginatedRepositories: ServicesGitRepository[]
  totalPages: number
  onViewRepository: (repo: ServicesGitRepository) => void
}

const RepositoriesListView: FC<RepositoriesListViewProps> = ({
  repositories,
  ownerSlug,
  searchQuery,
  onSearchChange,
  page,
  onPageChange,
  filteredRepositories,
  paginatedRepositories,
  totalPages,
  onViewRepository,
}) => {
  return (
    <>
      {/* GitHub-style header with owner/repositories */}
      <Box sx={{ mb: 3, pb: 2 }}>
        <Box sx={{ mb: 2 }}>
          <Typography variant="h4" component="h1" sx={{ fontWeight: 400, display: 'flex', alignItems: 'center', gap: 1 }}>
            <span style={{ color: '#3b82f6', cursor: 'pointer' }}>{ownerSlug}</span>
            <span style={{ color: 'text.secondary', fontWeight: 300 }}>/</span>
            <span style={{ fontWeight: 600 }}>repositories</span>
          </Typography>
        </Box>

        {/* Search bar */}
        {repositories.length > 0 && (
          <Box>
            <TextField
              placeholder="Find a repository..."
              size="small"
              value={searchQuery}
              onChange={(e) => {
                onSearchChange(e.target.value)
                onPageChange(0)
              }}
              InputProps={{
                startAdornment: (
                  <InputAdornment position="start">
                    <SearchIcon />
                  </InputAdornment>
                ),
              }}
              sx={{ maxWidth: 400 }}
            />
            {searchQuery && (
              <Typography variant="caption" color="text.secondary" sx={{ ml: 2 }}>
                {filteredRepositories.length} of {repositories.length} repositories
              </Typography>
            )}
          </Box>
        )}
      </Box>

      {filteredRepositories.length === 0 && searchQuery ? (
        <Card sx={{ textAlign: 'center', py: 8 }}>
          <CardContent>
            <GitBranch size={48} style={{ color: 'gray', marginBottom: 16 }} />
            <Typography variant="h6" gutterBottom>
              No repositories found
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
              Try adjusting your search query
            </Typography>
            <Button
              variant="outlined"
              onClick={() => onSearchChange('')}
            >
              Clear Search
            </Button>
          </CardContent>
        </Card>
      ) : repositories.length === 0 ? (
        <Card sx={{ textAlign: 'center', py: 8 }}>
          <CardContent>
            <GitBranch size={48} style={{ color: 'gray', marginBottom: 16 }} />
            <Typography variant="h6" gutterBottom>
              No repositories yet
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
              Create your first git repository to start collaborating with AI agents and your team.
            </Typography>
          </CardContent>
        </Card>
      ) : (
        <>
        {/* GitHub-style list view */}
        <Box>
          {paginatedRepositories.map((repo: ServicesGitRepository) => (
            <Box
              key={repo.id}
              sx={{
                py: 3,
                px: 2,
                borderBottom: 1,
                borderColor: 'divider',
                '&:hover': {
                  bgcolor: 'rgba(0, 0, 0, 0.02)',
                },
                cursor: 'pointer'
              }}
              onClick={() => onViewRepository(repo)}
            >
              <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start' }}>
                <Box sx={{ flex: 1 }}>
                  {/* Repo name as owner/repo */}
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1 }}>
                    <GitBranch size={16} color="#656d76" />
                    <Typography
                      variant="h6"
                      sx={{
                        fontSize: '1.25rem',
                        fontWeight: 600,
                        color: '#3b82f6',
                        display: 'flex',
                        alignItems: 'center',
                        gap: 0.5,
                        '&:hover': {
                          textDecoration: 'underline'
                        }
                      }}
                    >
                      {ownerSlug}
                      <span style={{ color: 'text.secondary', fontWeight: 400 }}>/</span>
                      {repo.name || repo.id}
                    </Typography>

                    {/* Chips */}
                    {repo.metadata?.is_external && (
                      <Chip
                        icon={<Link size={12} />}
                        label={repo.metadata.external_type || 'External'}
                        size="small"
                        sx={{ height: 20, fontSize: '0.75rem' }}
                      />
                    )}
                    {repo.metadata?.kodit_indexing && (
                      <Chip
                        icon={<Brain size={12} />}
                        label="Code Intelligence"
                        size="small"
                        color="success"
                        sx={{ height: 20, fontSize: '0.75rem' }}
                      />
                    )}
                    <Chip
                      label={repo.repo_type || 'project'}
                      size="small"
                      variant="outlined"
                      sx={{ height: 20, fontSize: '0.75rem', borderRadius: '12px' }}
                    />
                  </Box>

                  {/* Description */}
                  {repo.description && (
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 1, ml: 3 }}>
                      {repo.description}
                    </Typography>
                  )}

                  {/* Metadata row */}
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 2, ml: 3, mt: 1 }}>
                    {repo.created_at && (
                      <Typography variant="caption" color="text.secondary">
                        Updated {new Date(repo.created_at).toLocaleDateString()}
                      </Typography>
                    )}
                  </Box>
                </Box>

              </Box>
            </Box>
          ))}
        </Box>

        {/* Pagination */}
        {totalPages > 1 && (
          <Box sx={{ display: 'flex', justifyContent: 'center', mt: 4 }}>
            <Pagination
              count={totalPages}
              page={page + 1}
              onChange={(_, newPage) => onPageChange(newPage - 1)}
              color="primary"
              showFirstButton
              showLastButton
            />
          </Box>
        )}
        </>
      )}
    </>
  )
}

export default RepositoriesListView
