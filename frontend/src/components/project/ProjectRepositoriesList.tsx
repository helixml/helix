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
import { ServerGitRepository } from '../../api/api'

interface ProjectRepositoriesListProps {
  repositories: ServerGitRepository[]
  internalRepo?: ServerGitRepository | null
  primaryRepoId?: string
  onSetPrimaryRepo: (repoId: string) => void
  onDetachRepo: (repoId: string) => void
  setPrimaryRepoPending?: boolean
  detachRepoPending?: boolean
  onClose?: () => void
}

const ProjectRepositoriesList: FC<ProjectRepositoriesListProps> = ({
  repositories,
  internalRepo,
  primaryRepoId,
  onSetPrimaryRepo,
  onDetachRepo,
  setPrimaryRepoPending = false,
  detachRepoPending = false,
  onClose,
}) => {
  const account = useAccount()

  const handleNavigateToRepo = (repoId: string) => {
    if (onClose) {
      onClose()
    }
    account.orgNavigate('git-repo-detail', { repoId })
  }

  return (
    <>
      <List>
        {repositories.map((repo) => (
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
                repo.metadata?.is_external
                  ? repo.clone_url || 'External repository (no URL configured)'
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
                      onSetPrimaryRepo(repo.id)
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
                    onDetachRepo(repo.id)
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

      {/* Internal Repository Section */}
      {internalRepo && (
        <List>
          <ListItem
            sx={{
              border: 1,
              borderColor: 'divider',
              borderRadius: 1,
              backgroundColor: 'rgba(0, 0, 0, 0.02)',
              cursor: 'pointer',
              '&:hover': {
                backgroundColor: 'rgba(0, 0, 0, 0.04)',
              },
            }}
            onClick={() => handleNavigateToRepo(internalRepo.id)}
          >
            <ListItemText
              primary={
                <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                  <Typography variant="body2" sx={{ fontWeight: 600 }}>
                    {internalRepo.name}
                  </Typography>
                  <Chip label="Project Config" size="small" variant="outlined" />
                </Box>
              }
              secondary="Stores .helix/project.json and .helix/startup.sh"
            />
          </ListItem>
        </List>
      )}
    </>
  )
}

export default ProjectRepositoriesList
