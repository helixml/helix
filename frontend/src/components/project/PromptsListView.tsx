/**
 * PromptsListView - Aggregated prompt history view across all projects
 *
 * Features:
 * - Shows pinned prompts, templates, and recent prompts
 * - Search across all prompts
 * - Copy prompts, pin/unpin, manage templates
 * - Shows which project/task each prompt is from
 */

import React, { FC, useState, useEffect, useCallback, useMemo } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import {
  Box,
  Typography,
  TextField,
  InputAdornment,
  IconButton,
  Paper,
  List,
  ListItem,
  ListItemButton,
  ListItemText,
  ListItemIcon,
  Collapse,
  Divider,
  Tooltip,
  Chip,
  alpha,
  CircularProgress,
  Alert,
  Button,
} from '@mui/material'
import SearchIcon from '@mui/icons-material/Search'
import PushPinIcon from '@mui/icons-material/PushPin'
import PushPinOutlinedIcon from '@mui/icons-material/PushPinOutlined'
import DescriptionIcon from '@mui/icons-material/Description'
import DescriptionOutlinedIcon from '@mui/icons-material/DescriptionOutlined'
import HistoryIcon from '@mui/icons-material/History'
import ExpandLessIcon from '@mui/icons-material/ExpandLess'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import CheckIcon from '@mui/icons-material/Check'
import CloseIcon from '@mui/icons-material/Close'
import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { TypesPromptHistoryEntry } from '../../api/api'

// Query keys
const PINNED_PROMPTS_KEY = ['prompt-history', 'pinned']
const TEMPLATES_KEY = ['prompt-history', 'templates']
const SEARCH_PROMPTS_KEY = ['prompt-history', 'search']

interface PromptItemProps {
  entry: TypesPromptHistoryEntry
  onCopy: () => void
  onPin: () => void
  onToggleTemplate: () => void
  isPinned: boolean
  isTemplate: boolean
}

const PromptItem: FC<PromptItemProps> = ({
  entry,
  onCopy,
  onPin,
  onToggleTemplate,
  isPinned,
  isTemplate,
}) => {
  const [copied, setCopied] = useState(false)

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    navigator.clipboard.writeText(entry.content || '')
    setCopied(true)
    onCopy()
    setTimeout(() => setCopied(false), 1500)
  }

  const formatTime = (timestamp: string | undefined): string => {
    if (!timestamp) return ''
    const date = new Date(timestamp)
    const diffMs = Date.now() - date.getTime()
    const diffMins = Math.floor(diffMs / 60000)
    const diffHours = Math.floor(diffMins / 60)
    const diffDays = Math.floor(diffHours / 24)

    if (diffMins < 1) return 'just now'
    if (diffMins < 60) return `${diffMins}m ago`
    if (diffHours < 24) return `${diffHours}h ago`
    if (diffDays < 7) return `${diffDays}d ago`
    return date.toLocaleDateString()
  }

  const truncate = (text: string | undefined, maxLen: number = 120): string => {
    if (!text) return ''
    const firstLine = text.split('\n')[0]
    if (firstLine.length <= maxLen) return firstLine
    return firstLine.substring(0, maxLen - 3) + '...'
  }

  return (
    <ListItem
      disablePadding
      secondaryAction={
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.5 }}>
          {/* Copy button */}
          <Tooltip title={copied ? 'Copied!' : 'Copy'}>
            <IconButton
              size="small"
              onClick={handleCopy}
              sx={{ color: copied ? 'success.main' : 'text.secondary' }}
            >
              {copied ? <CheckIcon sx={{ fontSize: 18 }} /> : <ContentCopyIcon sx={{ fontSize: 18 }} />}
            </IconButton>
          </Tooltip>
          {/* Template toggle */}
          <Tooltip title={isTemplate ? 'Remove from templates' : 'Save as template'}>
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                onToggleTemplate()
              }}
              sx={{ color: isTemplate ? 'info.main' : 'text.secondary' }}
            >
              {isTemplate ? (
                <DescriptionIcon sx={{ fontSize: 18 }} />
              ) : (
                <DescriptionOutlinedIcon sx={{ fontSize: 18 }} />
              )}
            </IconButton>
          </Tooltip>
          {/* Pin toggle */}
          <Tooltip title={isPinned ? 'Unpin' : 'Pin'}>
            <IconButton
              size="small"
              onClick={(e) => {
                e.stopPropagation()
                onPin()
              }}
              sx={{ color: isPinned ? 'warning.main' : 'text.secondary' }}
            >
              {isPinned ? (
                <PushPinIcon sx={{ fontSize: 18 }} />
              ) : (
                <PushPinOutlinedIcon sx={{ fontSize: 18 }} />
              )}
            </IconButton>
          </Tooltip>
        </Box>
      }
      sx={{
        '& .MuiListItemSecondaryAction-root': {
          opacity: 0,
          transition: 'opacity 0.15s',
        },
        '&:hover .MuiListItemSecondaryAction-root': {
          opacity: 1,
        },
      }}
    >
      <ListItemButton
        sx={{
          py: 1.5,
          px: 2,
          borderLeft: isPinned ? '3px solid' : '3px solid transparent',
          borderColor: isPinned ? 'warning.main' : 'transparent',
          bgcolor: isPinned
            ? (theme) => alpha(theme.palette.warning.main, 0.04)
            : isTemplate
              ? (theme) => alpha(theme.palette.info.main, 0.04)
              : 'transparent',
        }}
      >
        <ListItemText
          primary={truncate(entry.content)}
          secondary={
            <Box component="span" sx={{ display: 'flex', alignItems: 'center', gap: 1, mt: 0.5 }}>
              <span>{formatTime(entry.created_at)}</span>
              {entry.usage_count !== undefined && entry.usage_count > 1 && (
                <Chip
                  label={`Used ${entry.usage_count}x`}
                  size="small"
                  sx={{ height: 18, fontSize: '0.7rem' }}
                />
              )}
              {entry.tags && (
                <Box sx={{ display: 'flex', gap: 0.5 }}>
                  {JSON.parse(entry.tags).map((tag: string) => (
                    <Chip
                      key={tag}
                      label={tag}
                      size="small"
                      variant="outlined"
                      sx={{ height: 18, fontSize: '0.65rem' }}
                    />
                  ))}
                </Box>
              )}
            </Box>
          }
          primaryTypographyProps={{
            fontSize: '0.875rem',
            sx: { pr: 12 },
          }}
          secondaryTypographyProps={{
            fontSize: '0.75rem',
          }}
        />
      </ListItemButton>
    </ListItem>
  )
}

const PromptsListView: FC = () => {
  const api = useApi()
  const snackbar = useSnackbar()
  const queryClient = useQueryClient()
  const apiClient = api.getApiClient()

  const [searchQuery, setSearchQuery] = useState('')
  const [debouncedSearch, setDebouncedSearch] = useState('')
  const [expandedSections, setExpandedSections] = useState({
    pinned: true,
    templates: true,
    recent: true,
  })

  // Debounce search query
  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedSearch(searchQuery)
    }, 300)
    return () => clearTimeout(timer)
  }, [searchQuery])

  // Fetch pinned prompts
  const { data: pinnedPrompts = [], isLoading: pinnedLoading } = useQuery({
    queryKey: PINNED_PROMPTS_KEY,
    queryFn: async () => {
      const response = await apiClient.v1PromptHistoryPinnedList({})
      return response.data || []
    },
  })

  // Fetch templates
  const { data: templates = [], isLoading: templatesLoading } = useQuery({
    queryKey: TEMPLATES_KEY,
    queryFn: async () => {
      const response = await apiClient.v1PromptHistoryTemplatesList()
      return response.data || []
    },
  })

  // Search prompts
  const { data: searchResults, isLoading: searchLoading } = useQuery({
    queryKey: [...SEARCH_PROMPTS_KEY, debouncedSearch],
    queryFn: async () => {
      if (!debouncedSearch.trim()) return null
      const response = await apiClient.v1PromptHistorySearchList({ q: debouncedSearch, limit: 50 })
      return response.data || []
    },
    enabled: !!debouncedSearch.trim(),
  })

  // Recent prompts - use search with empty string or fetch recent via templates endpoint
  // Since there's no "list all" endpoint, we'll search with a space to get recent ones
  const { data: recentPrompts = [], isLoading: recentLoading } = useQuery({
    queryKey: ['prompt-history', 'recent'],
    queryFn: async () => {
      // Get recent prompts by searching with a common character
      // This is a workaround since there's no "list all recent" endpoint
      const response = await apiClient.v1PromptHistorySearchList({ q: ' ', limit: 20 })
      return response.data || []
    },
  })

  // Mutation for updating pin status
  const updatePinMutation = useMutation({
    mutationFn: async ({ id, pinned }: { id: string; pinned: boolean }) => {
      await apiClient.v1PromptHistoryPinUpdate(id, { pinned })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: PINNED_PROMPTS_KEY })
      queryClient.invalidateQueries({ queryKey: TEMPLATES_KEY })
      queryClient.invalidateQueries({ queryKey: SEARCH_PROMPTS_KEY })
    },
  })

  // Mutation for updating template status
  const updateTemplateMutation = useMutation({
    mutationFn: async ({ id, isTemplate }: { id: string; isTemplate: boolean }) => {
      await apiClient.v1PromptHistoryTemplateUpdate(id, { is_template: isTemplate })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: PINNED_PROMPTS_KEY })
      queryClient.invalidateQueries({ queryKey: TEMPLATES_KEY })
      queryClient.invalidateQueries({ queryKey: SEARCH_PROMPTS_KEY })
    },
  })

  const handleCopy = useCallback(() => {
    snackbar.success('Prompt copied to clipboard')
  }, [snackbar])

  const handlePin = useCallback(async (id: string, currentPinned: boolean) => {
    try {
      await updatePinMutation.mutateAsync({ id, pinned: !currentPinned })
      snackbar.success(currentPinned ? 'Prompt unpinned' : 'Prompt pinned')
    } catch (error) {
      snackbar.error('Failed to update pin status')
    }
  }, [updatePinMutation, snackbar])

  const handleToggleTemplate = useCallback(async (id: string, currentTemplate: boolean) => {
    try {
      await updateTemplateMutation.mutateAsync({ id, isTemplate: !currentTemplate })
      snackbar.success(currentTemplate ? 'Removed from templates' : 'Saved as template')
    } catch (error) {
      snackbar.error('Failed to update template status')
    }
  }, [updateTemplateMutation, snackbar])

  const toggleSection = (section: keyof typeof expandedSections) => {
    setExpandedSections((prev) => ({
      ...prev,
      [section]: !prev[section],
    }))
  }

  const isLoading = pinnedLoading || templatesLoading || recentLoading

  // Create ID sets for quick lookup
  const pinnedIds = useMemo(() => new Set(pinnedPrompts.map(p => p.id)), [pinnedPrompts])
  const templateIds = useMemo(() => new Set(templates.map(t => t.id)), [templates])

  const renderSection = (
    title: string,
    icon: React.ReactNode,
    items: TypesPromptHistoryEntry[],
    sectionKey: keyof typeof expandedSections,
    color: string = 'text.secondary'
  ) => {
    if (items.length === 0) return null

    return (
      <Paper variant="outlined" sx={{ mb: 2, borderRadius: 2 }}>
        <ListItem
          component="div"
          onClick={() => toggleSection(sectionKey)}
          sx={{
            py: 1.5,
            px: 2,
            cursor: 'pointer',
            bgcolor: 'background.default',
            '&:hover': { bgcolor: 'action.hover' },
          }}
        >
          <ListItemIcon sx={{ minWidth: 36, color }}>
            {icon}
          </ListItemIcon>
          <ListItemText
            primary={
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography variant="subtitle1" sx={{ color, fontWeight: 600 }}>
                  {title}
                </Typography>
                <Chip
                  label={items.length}
                  size="small"
                  sx={{ height: 20, fontSize: '0.75rem' }}
                />
              </Box>
            }
          />
          {expandedSections[sectionKey] ? <ExpandLessIcon /> : <ExpandMoreIcon />}
        </ListItem>
        <Collapse in={expandedSections[sectionKey]} timeout="auto" unmountOnExit>
          <Divider />
          <List disablePadding>
            {items.map((entry) => (
              <PromptItem
                key={entry.id}
                entry={entry}
                onCopy={handleCopy}
                onPin={() => handlePin(entry.id || '', pinnedIds.has(entry.id))}
                onToggleTemplate={() => handleToggleTemplate(entry.id || '', templateIds.has(entry.id))}
                isPinned={pinnedIds.has(entry.id)}
                isTemplate={templateIds.has(entry.id)}
              />
            ))}
          </List>
        </Collapse>
      </Paper>
    )
  }

  return (
    <Box>
      {/* Header */}
      <Box sx={{ mb: 3 }}>
        <Typography variant="h5" sx={{ fontWeight: 600, mb: 1 }}>
          Prompt Library
        </Typography>
        <Typography variant="body2" color="text.secondary">
          Browse and manage your prompts across all projects. Pin frequently used prompts or save them as templates.
        </Typography>
      </Box>

      {/* Search */}
      <Paper variant="outlined" sx={{ p: 2, mb: 3, borderRadius: 2 }}>
        <TextField
          size="small"
          fullWidth
          placeholder="Search prompts..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                {searchLoading ? (
                  <CircularProgress size={18} />
                ) : (
                  <SearchIcon sx={{ fontSize: 20, color: 'text.secondary' }} />
                )}
              </InputAdornment>
            ),
            endAdornment: searchQuery && (
              <InputAdornment position="end">
                <IconButton size="small" onClick={() => setSearchQuery('')}>
                  <CloseIcon sx={{ fontSize: 18 }} />
                </IconButton>
              </InputAdornment>
            ),
          }}
        />
      </Paper>

      {/* Content */}
      {isLoading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
          <CircularProgress />
        </Box>
      ) : searchResults !== null && searchResults !== undefined ? (
        // Search results
        <Box>
          {searchResults.length === 0 ? (
            <Alert severity="info">
              No prompts found for "{debouncedSearch}". Try a different search term.
            </Alert>
          ) : (
            <Paper variant="outlined" sx={{ borderRadius: 2 }}>
              <Box sx={{ px: 2, py: 1.5, bgcolor: 'background.default', borderBottom: 1, borderColor: 'divider' }}>
                <Typography variant="subtitle1" sx={{ fontWeight: 600 }}>
                  Search Results ({searchResults.length})
                </Typography>
              </Box>
              <List disablePadding>
                {searchResults.map((entry) => (
                  <PromptItem
                    key={entry.id}
                    entry={entry}
                    onCopy={handleCopy}
                    onPin={() => handlePin(entry.id || '', pinnedIds.has(entry.id))}
                    onToggleTemplate={() => handleToggleTemplate(entry.id || '', templateIds.has(entry.id))}
                    isPinned={pinnedIds.has(entry.id)}
                    isTemplate={templateIds.has(entry.id)}
                  />
                ))}
              </List>
            </Paper>
          )}
        </Box>
      ) : (
        // Normal view with sections
        <Box>
          {renderSection(
            'Pinned Prompts',
            <PushPinIcon />,
            pinnedPrompts,
            'pinned',
            '#ed6c02' // warning.main
          )}
          {renderSection(
            'Templates',
            <DescriptionIcon />,
            templates,
            'templates',
            '#0288d1' // info.main
          )}
          {renderSection(
            'Recent Prompts',
            <HistoryIcon />,
            recentPrompts.filter(p => !pinnedIds.has(p.id) && !templateIds.has(p.id)),
            'recent',
            'text.secondary'
          )}
          {pinnedPrompts.length === 0 && templates.length === 0 && recentPrompts.length === 0 && (
            <Paper variant="outlined" sx={{ p: 4, textAlign: 'center', borderRadius: 2 }}>
              <HistoryIcon sx={{ fontSize: 48, color: 'text.disabled', mb: 2 }} />
              <Typography variant="h6" color="text.secondary" gutterBottom>
                No prompts yet
              </Typography>
              <Typography variant="body2" color="text.secondary">
                Send messages to AI agents in your projects to build your prompt library.
              </Typography>
            </Paper>
          )}
        </Box>
      )}
    </Box>
  )
}

export default PromptsListView
