import React, { FC } from 'react'
import {
  List,
  ListItem,
  ListItemText,
  ListItemSecondaryAction,
  IconButton,
  Chip,
  Box,
  Typography,
} from '@mui/material'
import StarIcon from '@mui/icons-material/Star'
import StarBorderIcon from '@mui/icons-material/StarBorder'
import DeleteIcon from '@mui/icons-material/Delete'

import useAccount from '../../hooks/useAccount'
import type { TypesGitRepository } from '../../api/api'

interface ProjectRepositoriesListProps {
  repositories: TypesGitRepository[]
  primaryRepoId?: string
  onSetPrimaryRepo: (repoId: string) => void
  onDetachRepo: (repoId: string) => void
  setPrimaryRepoPending?: boolean
  detachRepoPending?: boolean
  onClose?: () => void
}

const ProjectRepositoriesList: FC<ProjectRepositoriesListProps> = ({
  repositories,
  primaryRepoId,
  onSetPrimaryRepo,
  onDetachRepo,
  setPrimaryRepoPending = false,
  detachRepoPending = false,
  onClose,
}) => {
  const account = useAccount()

  const handleNavigateToRepo = (repoId: string | undefined) => {
    if (!repoId) return
    if (onClose) {
      onClose()
    }
    account.orgNavigate('git-repo-detail', { repoId })
  }

  // Filter out internal repos - they're deprecated
  const codeRepos = repositories.filter(r => r.repo_type !== 'internal')

  return (
    <List>
      {codeRepos.map((repo) => (
        <ListItem
          key={repo.id}
          divider
          sx={{
            cursor: 'pointer',
            '&:hover': {
              backgroundColor: 'rgba(0, 0, 0, 0.04)',
            },
          }}
          onClick={() => handleNavigateToRepo(repo.id)}
        >
          <ListItemText
            primary={
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography variant="body2" sx={{ fontWeight: 600 }}>
                  {repo.name}
                </Typography>
              </Box>
            }
            secondary={
              repo.is_external
                ? repo.external_url || 'External repository (no URL configured)'
                : repo.description || 'Helix-hosted repository'
            }
          />
          <ListItemSecondaryAction>
            <Box sx={{ display: 'flex', gap: 1, alignItems: 'center' }}>
              {primaryRepoId === repo.id ? (
                <Chip
                  icon={<StarIcon />}
                  label="Primary"
                  color="primary"
                  size="small"
                />
              ) : (
                <IconButton
                  onClick={(e) => {
                    e.stopPropagation()
                    if (repo.id) onSetPrimaryRepo(repo.id)
                  }}
                  disabled={setPrimaryRepoPending}
                  title="Set as primary"
                >
                  <StarBorderIcon />
                </IconButton>
              )}
              <IconButton
                onClick={(e) => {
                  e.stopPropagation()
                  if (repo.id) onDetachRepo(repo.id)
                }}
                disabled={detachRepoPending}
                title="Detach from project"
                color="error"
              >
                <DeleteIcon />
              </IconButton>
            </Box>
          </ListItemSecondaryAction>
        </ListItem>
      ))}
    </List>
  )
}

export default ProjectRepositoriesList
