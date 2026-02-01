import React, { FC } from 'react'
import {
  List,
  ListItem,
  ListItemText,
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
    <List sx={{ '& .MuiListItem-root': { px: { xs: 1, sm: 2 } } }}>
      {codeRepos.map((repo) => (
        <ListItem
          key={repo.id}
          divider
          sx={{
            cursor: 'pointer',
            '&:hover': {
              backgroundColor: 'rgba(0, 0, 0, 0.04)',
            },
            flexDirection: { xs: 'column', sm: 'row' },
            alignItems: { xs: 'flex-start', sm: 'center' },
            gap: { xs: 1, sm: 0 },
            py: { xs: 1.5, sm: 1 },
          }}
          onClick={() => handleNavigateToRepo(repo.id)}
        >
          <ListItemText
            sx={{
              pr: { xs: 0, sm: 14 },
              width: '100%',
              '& .MuiListItemText-secondary': {
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                whiteSpace: 'nowrap',
              },
            }}
            primary={
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography
                  variant="body2"
                  sx={{
                    fontWeight: 600,
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    whiteSpace: 'nowrap',
                  }}
                >
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
          <Box
            sx={{
              display: 'flex',
              gap: 0.5,
              alignItems: 'center',
              alignSelf: { xs: 'flex-end', sm: 'center' },
              position: { xs: 'relative', sm: 'absolute' },
              right: { xs: 'auto', sm: 16 },
            }}
          >
            {primaryRepoId === repo.id ? (
              <Chip
                icon={<StarIcon />}
                label="Primary"
                color="secondary"
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
                size="small"
              >
                <StarBorderIcon fontSize="small" />
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
              size="small"
            >
              <DeleteIcon fontSize="small" />
            </IconButton>
          </Box>
        </ListItem>
      ))}
    </List>
  )
}

export default ProjectRepositoriesList
