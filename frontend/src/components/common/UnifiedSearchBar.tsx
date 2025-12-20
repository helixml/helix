/**
 * UnifiedSearchBar - A prominent search bar for searching across all Helix entities
 *
 * Features:
 * - Search across projects, tasks, sessions, and prompts
 * - Real-time search results dropdown
 * - Keyboard shortcuts (Cmd/Ctrl+K)
 * - Click-through to results
 */

import React, { FC, useState, useEffect, useRef, useCallback } from 'react'
import {
  Box,
  TextField,
  InputAdornment,
  Paper,
  Typography,
  List,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Chip,
  CircularProgress,
  Popper,
  ClickAwayListener,
  alpha,
  Divider,
  Fade,
} from '@mui/material'
import SearchIcon from '@mui/icons-material/Search'
import FolderIcon from '@mui/icons-material/Folder'
import TaskIcon from '@mui/icons-material/Task'
import ChatIcon from '@mui/icons-material/Chat'
import FormatQuoteIcon from '@mui/icons-material/FormatQuote'
import CodeIcon from '@mui/icons-material/Code'
import CloseIcon from '@mui/icons-material/Close'
import KeyboardIcon from '@mui/icons-material/Keyboard'
import { useUnifiedSearch, groupResultsByType, getSearchResultTypeLabel } from '../../services/searchService'
import { TypesUnifiedSearchResult } from '../../api/api'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'

interface UnifiedSearchBarProps {
  placeholder?: string
  autoFocus?: boolean
  compact?: boolean
  onResultClick?: (result: TypesUnifiedSearchResult) => void
}

const UnifiedSearchBar: FC<UnifiedSearchBarProps> = ({
  placeholder = 'Search projects, tasks, sessions...',
  autoFocus = false,
  compact = false,
  onResultClick,
}) => {
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const [focused, setFocused] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)
  const anchorRef = useRef<HTMLDivElement>(null)
  const router = useRouter()
  const account = useAccount()

  // Debounced search
  const [debouncedQuery, setDebouncedQuery] = useState('')
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(query)
    }, 200)
    return () => clearTimeout(timer)
  }, [query])

  const { data, isLoading } = useUnifiedSearch({
    query: debouncedQuery,
    limit: 5,
    enabled: debouncedQuery.length >= 2,
  })

  // Open dropdown when we have results
  useEffect(() => {
    if (data?.results && data.results.length > 0 && focused) {
      setOpen(true)
    } else if (!debouncedQuery) {
      setOpen(false)
    }
  }, [data, debouncedQuery, focused])

  // Keyboard shortcut: Cmd/Ctrl+K
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key === 'k') {
        e.preventDefault()
        inputRef.current?.focus()
        setFocused(true)
      }
      if (e.key === 'Escape' && open) {
        setOpen(false)
        inputRef.current?.blur()
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [open])

  const handleResultClick = useCallback((result: TypesUnifiedSearchResult) => {
    if (onResultClick) {
      onResultClick(result)
    } else if (result.url) {
      // Navigate based on type
      if (result.type === 'project') {
        account.orgNavigate('project-specs', { id: result.id })
      } else if (result.type === 'task') {
        account.orgNavigate('spec-task', { id: result.id })
      } else if (result.type === 'session') {
        router.navigate('session', { id: result.id })
      } else if (result.type === 'code') {
        // For code results, navigate to the repository
        const repoId = result.metadata?.repoId
        if (repoId) {
          account.orgNavigate('repository', { repository_id: repoId })
        }
      } else {
        // For prompts, navigate to the task if available
        const taskId = result.metadata?.taskId
        if (taskId) {
          account.orgNavigate('spec-task', { id: taskId })
        }
      }
    }
    setOpen(false)
    setQuery('')
  }, [onResultClick, router, account])

  const getIcon = (type: string) => {
    switch (type) {
      case 'project':
        return <FolderIcon sx={{ color: 'secondary.main' }} />
      case 'task':
        return <TaskIcon sx={{ color: 'primary.main' }} />
      case 'session':
        return <ChatIcon sx={{ color: 'info.main' }} />
      case 'prompt':
        return <FormatQuoteIcon sx={{ color: 'warning.main' }} />
      case 'code':
        return <CodeIcon sx={{ color: 'success.main' }} />
      default:
        return <SearchIcon />
    }
  }

  const groupedResults = data?.results ? groupResultsByType(data.results) : {}

  return (
    <ClickAwayListener onClickAway={() => setOpen(false)}>
      <Box
        ref={anchorRef}
        sx={{
          position: 'relative',
          width: '100%',
          maxWidth: compact ? 400 : 600,
          mx: 'auto',
        }}
      >
        <TextField
          inputRef={inputRef}
          fullWidth
          placeholder={placeholder}
          value={query}
          onChange={(e) => setQuery(e.target.value)}
          onFocus={() => {
            setFocused(true)
            if (data?.results && data.results.length > 0) {
              setOpen(true)
            }
          }}
          onBlur={() => setFocused(false)}
          autoFocus={autoFocus}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                {isLoading ? (
                  <CircularProgress size={20} />
                ) : (
                  <SearchIcon sx={{ color: 'text.secondary' }} />
                )}
              </InputAdornment>
            ),
            endAdornment: (
              <InputAdornment position="end">
                {query ? (
                  <CloseIcon
                    sx={{ cursor: 'pointer', color: 'text.secondary', fontSize: 18 }}
                    onClick={() => {
                      setQuery('')
                      setOpen(false)
                    }}
                  />
                ) : (
                  <Box
                    sx={{
                      display: 'flex',
                      alignItems: 'center',
                      gap: 0.5,
                      color: 'text.disabled',
                      fontSize: '0.75rem',
                    }}
                  >
                    <KeyboardIcon sx={{ fontSize: 14 }} />
                    <Typography variant="caption" sx={{ fontFamily: 'monospace' }}>
                      {navigator.platform.includes('Mac') ? 'Cmd' : 'Ctrl'}+K
                    </Typography>
                  </Box>
                )}
              </InputAdornment>
            ),
          }}
          sx={{
            '& .MuiOutlinedInput-root': {
              borderRadius: compact ? 1 : 2,
              bgcolor: 'background.paper',
              fontSize: compact ? '0.875rem' : '1rem',
              '& fieldset': {
                borderColor: 'divider',
              },
              '&:hover fieldset': {
                borderColor: 'primary.main',
              },
              '&.Mui-focused fieldset': {
                borderColor: 'primary.main',
                borderWidth: 2,
              },
            },
          }}
        />

        <Popper
          open={open}
          anchorEl={anchorRef.current}
          placement="bottom-start"
          transition
          style={{ width: anchorRef.current?.offsetWidth, zIndex: 1300 }}
        >
          {({ TransitionProps }) => (
            <Fade {...TransitionProps} timeout={200}>
              <Paper
                elevation={8}
                sx={{
                  mt: 1,
                  maxHeight: 400,
                  overflow: 'auto',
                  borderRadius: 2,
                  border: '1px solid',
                  borderColor: 'divider',
                }}
              >
                {!data?.results || data.results.length === 0 ? (
                  <Box sx={{ p: 2, textAlign: 'center' }}>
                    <Typography variant="body2" color="text.secondary">
                      {debouncedQuery.length < 2
                        ? 'Type at least 2 characters to search'
                        : 'No results found'}
                    </Typography>
                  </Box>
                ) : (
                  <List dense disablePadding>
                    {Object.entries(groupedResults).map(([type, results], groupIndex) => (
                      <React.Fragment key={type}>
                        {groupIndex > 0 && <Divider />}
                        <ListItem sx={{ py: 0.5, bgcolor: 'action.hover' }}>
                          <ListItemText
                            primary={
                              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                                <Typography
                                  variant="caption"
                                  sx={{ fontWeight: 600, color: 'text.secondary', textTransform: 'uppercase' }}
                                >
                                  {getSearchResultTypeLabel(type)}s
                                </Typography>
                                <Chip
                                  label={results.length}
                                  size="small"
                                  sx={{ height: 16, fontSize: '0.65rem' }}
                                />
                              </Box>
                            }
                          />
                        </ListItem>
                        {results.map((result: TypesUnifiedSearchResult) => (
                          <ListItemButton
                            key={result.id}
                            onClick={() => handleResultClick(result)}
                            sx={{
                              py: 1,
                              '&:hover': {
                                bgcolor: (theme) => alpha(theme.palette.primary.main, 0.08),
                              },
                            }}
                          >
                            <ListItemIcon sx={{ minWidth: 36 }}>
                              {getIcon(result.type || 'unknown')}
                            </ListItemIcon>
                            <ListItemText
                              primary={result.title}
                              secondary={result.description}
                              primaryTypographyProps={{
                                noWrap: true,
                                fontWeight: 500,
                              }}
                              secondaryTypographyProps={{
                                noWrap: true,
                                fontSize: '0.75rem',
                              }}
                            />
                            {result.metadata?.status && (
                              <Chip
                                label={result.metadata.status}
                                size="small"
                                sx={{ height: 20, fontSize: '0.65rem', ml: 1 }}
                              />
                            )}
                          </ListItemButton>
                        ))}
                      </React.Fragment>
                    ))}
                  </List>
                )}
              </Paper>
            </Fade>
          )}
        </Popper>
      </Box>
    </ClickAwayListener>
  )
}

export default UnifiedSearchBar
