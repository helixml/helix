/**
 * UnifiedSearchBar - A prominent search bar for searching across all Helix entities
 *
 * Features:
 * - Search across projects, tasks, sessions, prompts, code, and knowledge
 * - Tabbed results interface like Google search
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
  ListItemButton,
  ListItemIcon,
  ListItemText,
  Chip,
  CircularProgress,
  Popper,
  ClickAwayListener,
  alpha,
  Fade,
  Tabs,
  Tab,
} from '@mui/material'
import SearchIcon from '@mui/icons-material/Search'
import FolderIcon from '@mui/icons-material/Folder'
import TaskIcon from '@mui/icons-material/Task'
import ChatIcon from '@mui/icons-material/Chat'
import FormatQuoteIcon from '@mui/icons-material/FormatQuote'
import CodeIcon from '@mui/icons-material/Code'
import MenuBookIcon from '@mui/icons-material/MenuBook'
import SourceIcon from '@mui/icons-material/Source'
import SmartToyIcon from '@mui/icons-material/SmartToy'
import CloseIcon from '@mui/icons-material/Close'
import KeyboardIcon from '@mui/icons-material/Keyboard'
import { useUnifiedSearch, groupResultsByType, getSearchResultTypeLabel } from '../../services/searchService'
import { TypesUnifiedSearchResult } from '../../api/api'
import useAccount from '../../hooks/useAccount'

interface UnifiedSearchBarProps {
  placeholder?: string
  autoFocus?: boolean
  compact?: boolean
  onResultClick?: (result: TypesUnifiedSearchResult) => void
}

// Tab order for the search results
const TAB_ORDER = ['all', 'session', 'agent', 'prompt', 'code', 'knowledge', 'repository', 'project', 'task'] as const
type TabType = typeof TAB_ORDER[number]

const UnifiedSearchBar: FC<UnifiedSearchBarProps> = ({
  placeholder = 'Search projects, tasks, sessions...',
  autoFocus = false,
  compact = false,
  onResultClick,
}) => {
  const [query, setQuery] = useState('')
  const [open, setOpen] = useState(false)
  const [focused, setFocused] = useState(false)
  const [activeTab, setActiveTab] = useState<TabType>('all')
  const inputRef = useRef<HTMLInputElement>(null)
  const anchorRef = useRef<HTMLDivElement>(null)
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
    limit: 10,
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

  // Reset tab when query changes
  useEffect(() => {
    setActiveTab('all')
  }, [debouncedQuery])

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
    } else {
      // Navigate based on type
      if (result.type === 'project') {
        account.orgNavigate('project-specs', { id: result.id })
      } else if (result.type === 'task') {
        // Navigate to project kanban with task highlighted
        const projectId = result.metadata?.projectId
        if (projectId) {
          account.orgNavigate('project-specs', { id: projectId, highlight: result.id })
        }
      } else if (result.type === 'session') {
        account.orgNavigate('session', { session_id: result.id })
      } else if (result.type === 'code') {
        // For code results, navigate to the repository with file selected
        const repoId = result.metadata?.repoId
        const filePath = result.metadata?.filePath
        if (repoId) {
          account.orgNavigate('git-repo-detail', {
            repoId: repoId,
            file: filePath || undefined,
          })
        }
      } else if (result.type === 'knowledge') {
        // For knowledge, navigate to the app if available
        const appId = result.metadata?.appId
        if (appId) {
          account.orgNavigate('app', { app_id: appId })
        }
      } else if (result.type === 'repository') {
        account.orgNavigate('git-repo-detail', { repoId: result.id })
      } else if (result.type === 'agent') {
        account.orgNavigate('app', { app_id: result.id })
      } else if (result.type === 'prompt') {
        // For prompts, navigate to the task in the project kanban
        const taskId = result.metadata?.taskId
        const projectId = result.metadata?.projectId
        if (taskId && projectId) {
          account.orgNavigate('project-specs', { id: projectId, highlight: taskId })
        }
      }
    }
    setOpen(false)
    setQuery('')
  }, [onResultClick, account])

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
      case 'knowledge':
        return <MenuBookIcon sx={{ color: '#9c27b0' }} />
      case 'repository':
        return <SourceIcon sx={{ color: '#00bcd4' }} />
      case 'agent':
        return <SmartToyIcon sx={{ color: '#ff5722' }} />
      default:
        return <SearchIcon />
    }
  }

  const groupedResults = data?.results ? groupResultsByType(data.results) : {}

  // Get available tabs (only show tabs that have results)
  const availableTabs = TAB_ORDER.filter(tab => {
    if (tab === 'all') return true
    return groupedResults[tab] && groupedResults[tab].length > 0
  })

  // Get results for the current tab
  const getFilteredResults = (): TypesUnifiedSearchResult[] => {
    if (!data?.results) return []
    if (activeTab === 'all') return data.results
    return groupedResults[activeTab] || []
  }

  const filteredResults = getFilteredResults()

  // Get tab label with count
  const getTabLabel = (tab: TabType): string => {
    if (tab === 'all') return 'All'
    const count = groupedResults[tab]?.length || 0
    const label = getSearchResultTypeLabel(tab)
    return count > 0 ? `${label}s (${count})` : `${label}s`
  }

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
                borderWidth: 1,
              },
              '&:hover fieldset': {
                borderColor: 'primary.main',
                borderWidth: 1,
              },
              '&.Mui-focused fieldset': {
                borderColor: 'primary.main',
                borderWidth: 1,
                boxShadow: '0 0 0 1px rgba(25, 118, 210, 0.5)',
              },
            },
          }}
        />

        <Popper
          open={open}
          anchorEl={anchorRef.current}
          placement="bottom-start"
          transition
          style={{
            width: compact ? 'min(400px, 90vw)' : anchorRef.current?.offsetWidth,
            minWidth: compact ? 300 : undefined,
            zIndex: 1300
          }}
        >
          {({ TransitionProps }) => (
            <Fade {...TransitionProps} timeout={200}>
              <Paper
                elevation={8}
                sx={{
                  mt: 1,
                  maxHeight: 500,
                  overflow: 'hidden',
                  borderRadius: 2,
                  border: '1px solid',
                  borderColor: 'divider',
                  display: 'flex',
                  flexDirection: 'column',
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
                  <>
                    {/* Tabs - Google-style */}
                    <Box sx={{ borderBottom: 1, borderColor: 'divider', bgcolor: 'background.default' }}>
                      <Tabs
                        value={activeTab}
                        onChange={(_, newValue) => setActiveTab(newValue)}
                        variant="scrollable"
                        scrollButtons="auto"
                        sx={{
                          minHeight: 36,
                          '& .MuiTab-root': {
                            minHeight: 36,
                            py: 0.5,
                            px: 1.5,
                            fontSize: '0.75rem',
                            textTransform: 'none',
                            minWidth: 'auto',
                          },
                        }}
                      >
                        {availableTabs.map((tab) => (
                          <Tab
                            key={tab}
                            value={tab}
                            label={getTabLabel(tab)}
                            icon={tab !== 'all' ? getIcon(tab) : undefined}
                            iconPosition="start"
                            sx={{
                              '& .MuiSvgIcon-root': {
                                fontSize: 16,
                                mr: 0.5,
                              },
                            }}
                          />
                        ))}
                      </Tabs>
                    </Box>

                    {/* Results list */}
                    <List
                      dense
                      disablePadding
                      sx={{
                        overflow: 'auto',
                        maxHeight: 400,
                      }}
                    >
                      {filteredResults.length === 0 ? (
                        <Box sx={{ p: 2, textAlign: 'center' }}>
                          <Typography variant="body2" color="text.secondary">
                            No {activeTab === 'all' ? '' : getSearchResultTypeLabel(activeTab).toLowerCase()} results
                          </Typography>
                        </Box>
                      ) : (
                        filteredResults.map((result: TypesUnifiedSearchResult) => (
                          <ListItemButton
                            key={`${result.type}-${result.id}`}
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
                              secondary={
                                <Box component="span" sx={{ display: 'flex', alignItems: 'center', gap: 1, flexWrap: 'wrap' }}>
                                  {activeTab === 'all' && (
                                    <Chip
                                      label={getSearchResultTypeLabel(result.type || 'unknown')}
                                      size="small"
                                      sx={{ height: 16, fontSize: '0.6rem' }}
                                    />
                                  )}
                                  {/* Show relationship context */}
                                  {result.metadata?.projectName && (
                                    <Chip
                                      label={result.metadata.projectName}
                                      size="small"
                                      variant="outlined"
                                      sx={{ height: 16, fontSize: '0.6rem', borderColor: 'secondary.main', color: 'secondary.main' }}
                                    />
                                  )}
                                  {result.metadata?.taskName && (
                                    <Chip
                                      label={result.metadata.taskName}
                                      size="small"
                                      variant="outlined"
                                      sx={{ height: 16, fontSize: '0.6rem', borderColor: 'primary.main', color: 'primary.main' }}
                                    />
                                  )}
                                  <Typography
                                    component="span"
                                    variant="body2"
                                    sx={{ fontSize: '0.75rem', color: 'text.secondary', flex: 1, minWidth: 0 }}
                                    noWrap
                                  >
                                    {result.description}
                                  </Typography>
                                </Box>
                              }
                              primaryTypographyProps={{
                                noWrap: true,
                                fontWeight: 500,
                              }}
                            />
                            {result.metadata?.status && (
                              <Chip
                                label={result.metadata.status}
                                size="small"
                                sx={{ height: 20, fontSize: '0.65rem', ml: 1 }}
                              />
                            )}
                            {result.metadata?.sourceType && (
                              <Chip
                                label={result.metadata.sourceType}
                                size="small"
                                variant="outlined"
                                sx={{ height: 20, fontSize: '0.65rem', ml: 1 }}
                              />
                            )}
                          </ListItemButton>
                        ))
                      )}
                    </List>

                    {/* Total count */}
                    <Box
                      sx={{
                        px: 2,
                        py: 0.5,
                        borderTop: 1,
                        borderColor: 'divider',
                        bgcolor: 'action.hover',
                      }}
                    >
                      <Typography variant="caption" color="text.secondary">
                        {data.total} result{data.total !== 1 ? 's' : ''} for "{data.query}"
                      </Typography>
                    </Box>
                  </>
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
