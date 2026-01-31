import React, { FC, useMemo, useState } from 'react'
import {
  Box,
  List,
  ListItem,
  ListItemText,
  ListItemIcon,
  ListItemButton,
  Typography,
  Chip,
  TextField,
  InputAdornment,
  CircularProgress,
  Alert,
} from '@mui/material'
import {
  Lock,
  LockOpen,
  Search,
  Folder,
} from '@mui/icons-material'
import { TypesRepositoryInfo } from '../../api/api'

interface RepositoryBrowserProps {
  repositories: TypesRepositoryInfo[] | undefined
  isLoading: boolean
  error: Error | null
  onSelect: (repo: TypesRepositoryInfo) => void
}

/**
 * RepositoryBrowser component displays a searchable list of repositories
 * and allows the user to select one.
 */
const RepositoryBrowser: FC<RepositoryBrowserProps> = ({
  repositories,
  isLoading,
  error,
  onSelect,
}) => {
  const [searchQuery, setSearchQuery] = useState('')

  // Filter repositories based on search query
  const filteredRepos = useMemo(() => {
    if (!repositories) return []
    if (!searchQuery.trim()) return repositories

    const query = searchQuery.toLowerCase()
    return repositories.filter(repo =>
      repo.full_name?.toLowerCase().includes(query) ||
      repo.name?.toLowerCase().includes(query) ||
      repo.description?.toLowerCase().includes(query)
    )
  }, [repositories, searchQuery])

  if (isLoading) {
    return (
      <Box sx={{ display: 'flex', justifyContent: 'center', p: 4 }}>
        <CircularProgress size={32} />
      </Box>
    )
  }

  if (error) {
    return (
      <Alert severity="error" sx={{ m: 2 }}>
        Failed to load repositories: {error.message}
      </Alert>
    )
  }

  if (!repositories || repositories.length === 0) {
    return (
      <Alert severity="info" sx={{ m: 2 }}>
        No repositories found. Make sure you have access to at least one repository.
      </Alert>
    )
  }

  return (
    <Box>
      <TextField
        fullWidth
        size="small"
        placeholder="Search repositories..."
        value={searchQuery}
        onChange={(e) => setSearchQuery(e.target.value)}
        InputProps={{
          startAdornment: (
            <InputAdornment position="start">
              <Search fontSize="small" />
            </InputAdornment>
          ),
        }}
        sx={{ mb: 2 }}
      />

      <Typography variant="caption" color="text.secondary" sx={{ mb: 1, display: 'block' }}>
        {filteredRepos.length} {filteredRepos.length === 1 ? 'repository' : 'repositories'} found
      </Typography>

      <List sx={{ maxHeight: 400, overflow: 'auto', bgcolor: 'background.paper', borderRadius: 1 }}>
        {filteredRepos.map((repo) => (
          <ListItem key={repo.full_name || repo.name} disablePadding>
            <ListItemButton onClick={() => onSelect(repo)} sx={{ py: 1.5 }}>
              <ListItemIcon sx={{ minWidth: 40 }}>
                <Folder fontSize="small" />
              </ListItemIcon>
              <ListItemText
                primary={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                    <Typography variant="body2" fontWeight={500}>
                      {repo.full_name || repo.name}
                    </Typography>
                    {repo.private ? (
                      <Lock fontSize="inherit" color="action" sx={{ fontSize: 14 }} />
                    ) : (
                      <LockOpen fontSize="inherit" color="action" sx={{ fontSize: 14 }} />
                    )}
                  </Box>
                }
                secondary={
                  <Box sx={{ display: 'flex', flexDirection: 'column', gap: 0.5 }}>
                    {repo.description && (
                      <Typography variant="caption" color="text.secondary" noWrap sx={{ maxWidth: 400 }}>
                        {repo.description}
                      </Typography>
                    )}
                    {repo.default_branch && (
                      <Chip
                        label={repo.default_branch}
                        size="small"
                        variant="outlined"
                        sx={{ alignSelf: 'flex-start', height: 18, fontSize: 10 }}
                      />
                    )}
                  </Box>
                }
              />
            </ListItemButton>
          </ListItem>
        ))}
      </List>
    </Box>
  )
}

export default RepositoryBrowser
