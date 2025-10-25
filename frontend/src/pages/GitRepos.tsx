import React, { FC } from 'react'
import Container from '@mui/material/Container'
import { Box, Typography, Card, CardContent, CircularProgress, Alert, Button, Chip } from '@mui/material'
import { GitBranch, Plus, ExternalLink } from 'lucide-react'

import Page from '../components/system/Page'
import LaunchpadCTAButton from '../components/widgets/LaunchpadCTAButton'

import useAccount from '../hooks/useAccount'
import { useGitRepositories } from '../services/gitRepositoryService'
import type { ServicesGitRepository } from '../api/api'

const GitRepos: FC = () => {
  const account = useAccount()
  const { data: repositories, isLoading, error } = useGitRepositories(account.user?.id)

  // Show logged out state if user is not authenticated
  if (!account.user) {
    return (
      <Page
        breadcrumbTitle="Git Repositories"
        orgBreadcrumbs={true}
      >
        <Container maxWidth="xl" sx={{ mb: 4 }}>
          <Card sx={{ textAlign: 'center', py: 8 }}>
            <CardContent>
              <GitBranch size={64} style={{ color: 'gray', marginBottom: 24 }} />
              <Typography variant="h4" component="h1" gutterBottom>
                Git Repositories
              </Typography>
              <Typography variant="h6" color="text.secondary" gutterBottom>
                Manage your git repositories for AI agent development
              </Typography>
              <Typography variant="body1" color="text.secondary" sx={{ mb: 4, maxWidth: 600, mx: 'auto' }}>
                Please log in to view and manage your git repositories.
              </Typography>
              <LaunchpadCTAButton />
            </CardContent>
          </Card>
        </Container>
      </Page>
    )
  }

  return (
    <Page
      breadcrumbTitle="Git Repositories"
      orgBreadcrumbs={true}
    >
      <Container maxWidth="xl" sx={{ mb: 4 }}>
        <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 3 }}>
          <Typography variant="h4" component="h1">
            Git Repositories
          </Typography>
          <Button
            variant="contained"
            startIcon={<Plus size={18} />}
            onClick={() => {
              // TODO: Implement create repository dialog
              console.log('Create repository')
            }}
          >
            New Repository
          </Button>
        </Box>

        {error && (
          <Alert severity="error" sx={{ mb: 2 }}>
            {error instanceof Error ? error.message : 'Failed to load repositories'}
          </Alert>
        )}

        {isLoading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
            <CircularProgress />
          </Box>
        ) : repositories && repositories.length > 0 ? (
          <Box sx={{ display: 'grid', gap: 2, gridTemplateColumns: 'repeat(auto-fill, minmax(350px, 1fr))' }}>
            {repositories.map((repo: ServicesGitRepository) => (
              <Card key={repo.id} sx={{
                '&:hover': {
                  boxShadow: 3,
                  transform: 'translateY(-2px)',
                  transition: 'all 0.2s'
                }
              }}>
                <CardContent>
                  <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', mb: 2 }}>
                    <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                      <GitBranch size={20} />
                      <Typography variant="h6" component="h2">
                        {repo.name || repo.id}
                      </Typography>
                    </Box>
                    <Chip
                      label={repo.repo_type || 'unknown'}
                      size="small"
                      color="primary"
                      variant="outlined"
                    />
                  </Box>

                  {repo.description && (
                    <Typography variant="body2" color="text.secondary" sx={{ mb: 2 }}>
                      {repo.description}
                    </Typography>
                  )}

                  <Box sx={{ display: 'flex', gap: 1, flexWrap: 'wrap' }}>
                    {repo.clone_url && (
                      <Button
                        size="small"
                        variant="outlined"
                        startIcon={<ExternalLink size={14} />}
                        onClick={() => {
                          // TODO: Show clone instructions
                          console.log('Clone:', repo.clone_url)
                        }}
                      >
                        Clone
                      </Button>
                    )}
                    <Button
                      size="small"
                      variant="text"
                      onClick={() => {
                        // TODO: Navigate to repository details
                        console.log('View repo:', repo.id)
                      }}
                    >
                      Details
                    </Button>
                  </Box>

                  {repo.created_at && (
                    <Typography variant="caption" color="text.secondary" sx={{ mt: 2, display: 'block' }}>
                      Created {new Date(repo.created_at).toLocaleDateString()}
                    </Typography>
                  )}
                </CardContent>
              </Card>
            ))}
          </Box>
        ) : (
          <Card sx={{ textAlign: 'center', py: 8 }}>
            <CardContent>
              <GitBranch size={48} style={{ color: 'gray', marginBottom: 16 }} />
              <Typography variant="h6" gutterBottom>
                No repositories yet
              </Typography>
              <Typography variant="body2" color="text.secondary" sx={{ mb: 3 }}>
                Create your first git repository to start collaborating with AI agents.
              </Typography>
              <Button
                variant="contained"
                startIcon={<Plus size={18} />}
                onClick={() => {
                  // TODO: Implement create repository dialog
                  console.log('Create repository')
                }}
              >
                Create Repository
              </Button>
            </CardContent>
          </Card>
        )}
      </Container>
    </Page>
  )
}

export default GitRepos
