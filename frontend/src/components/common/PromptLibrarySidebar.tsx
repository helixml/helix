/**
 * PromptLibrarySidebar - A sidebar for browsing and managing prompt history
 *
 * Features:
 * - Pinned prompts section for quick access
 * - Recent prompts with search
 * - Click to copy/use prompt
 * - Pin/unpin management
 */

import React, { FC, useState, useEffect, useCallback } from 'react'
import {
  Box,
  Typography,
  TextField,
  InputAdornment,
  IconButton,
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
} from '@mui/material'
import SearchIcon from '@mui/icons-material/Search'
import PushPinIcon from '@mui/icons-material/PushPin'
import PushPinOutlinedIcon from '@mui/icons-material/PushPinOutlined'
import HistoryIcon from '@mui/icons-material/History'
import ExpandLessIcon from '@mui/icons-material/ExpandLess'
import ExpandMoreIcon from '@mui/icons-material/ExpandMore'
import ContentCopyIcon from '@mui/icons-material/ContentCopy'
import CheckIcon from '@mui/icons-material/Check'
import CloseIcon from '@mui/icons-material/Close'
import { PromptHistoryEntry } from '../../hooks/usePromptHistory'

interface PromptLibrarySidebarProps {
  // Library data
  pinnedPrompts: PromptHistoryEntry[]
  recentPrompts: PromptHistoryEntry[]
  // Actions
  onSelectPrompt: (content: string) => void
  onPinPrompt: (id: string, pinned: boolean) => Promise<void>
  onSearch: (query: string) => Promise<PromptHistoryEntry[]>
  // Loading states
  loading?: boolean
  // Close handler
  onClose?: () => void
}

interface PromptItemProps {
  entry: PromptHistoryEntry
  onSelect: () => void
  onPin: () => void
}

const PromptItem: FC<PromptItemProps> = ({
  entry,
  onSelect,
  onPin,
}) => {
  const [copied, setCopied] = useState(false)
  const isPinned = entry.pinned

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation()
    navigator.clipboard.writeText(entry.content)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  const formatTime = (timestamp: number): string => {
    const diffMs = Date.now() - timestamp
    const diffMins = Math.floor(diffMs / 60000)
    const diffHours = Math.floor(diffMins / 60)
    const diffDays = Math.floor(diffHours / 24)

    if (diffMins < 1) return 'just now'
    if (diffMins < 60) return `${diffMins}m ago`
    if (diffHours < 24) return `${diffHours}h ago`
    if (diffDays < 7) return `${diffDays}d ago`
    return new Date(timestamp).toLocaleDateString()
  }

  const truncate = (text: string, maxLen: number = 80): string => {
    const firstLine = text.split('\n')[0]
    if (firstLine.length <= maxLen) return firstLine
    return firstLine.substring(0, maxLen - 3) + '...'
  }

  return (
    <ListItem
      disablePadding
      secondaryAction={
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
          {/* Copy button */}
          <Tooltip title={copied ? 'Copied!' : 'Copy'}>
            <IconButton
              size="small"
              onClick={handleCopy}
              sx={{ color: copied ? 'success.main' : 'text.secondary', opacity: 0.6 }}
            >
              {copied ? <CheckIcon sx={{ fontSize: 16 }} /> : <ContentCopyIcon sx={{ fontSize: 16 }} />}
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
              sx={{ color: isPinned ? 'warning.main' : 'text.secondary', opacity: isPinned ? 0.9 : 0.6 }}
            >
              {isPinned ? (
                <PushPinIcon sx={{ fontSize: 16 }} />
              ) : (
                <PushPinOutlinedIcon sx={{ fontSize: 16 }} />
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
        onClick={onSelect}
        sx={{
          py: 1,
          borderLeft: isPinned ? '3px solid' : '3px solid transparent',
          borderColor: isPinned ? 'warning.main' : 'transparent',
          bgcolor: isPinned
            ? (theme) => alpha(theme.palette.warning.main, 0.04)
            : 'transparent',
        }}
      >
        <ListItemText
          primary={truncate(entry.content)}
          secondary={
            <Box component="span" sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
              <span>{formatTime(entry.timestamp)}</span>
              {entry.usageCount && entry.usageCount > 1 && (
                <Chip
                  label={`Used ${entry.usageCount}x`}
                  size="small"
                  sx={{ height: 16, fontSize: '0.65rem' }}
                />
              )}
            </Box>
          }
          primaryTypographyProps={{
            fontSize: '0.875rem',
            noWrap: true,
            sx: { pr: 8 },
          }}
          secondaryTypographyProps={{
            fontSize: '0.75rem',
          }}
        />
      </ListItemButton>
    </ListItem>
  )
}

const PromptLibrarySidebar: FC<PromptLibrarySidebarProps> = ({
  pinnedPrompts,
  recentPrompts,
  onSelectPrompt,
  onPinPrompt,
  onSearch,
  loading = false,
  onClose,
}) => {
  const [searchQuery, setSearchQuery] = useState('')
  const [searchResults, setSearchResults] = useState<PromptHistoryEntry[] | null>(null)
  const [searching, setSearching] = useState(false)
  const [expandedSections, setExpandedSections] = useState({
    pinned: true,
    recent: true,
  })

  // Debounced search
  useEffect(() => {
    if (!searchQuery.trim()) {
      setSearchResults(null)
      return
    }

    const timer = setTimeout(async () => {
      setSearching(true)
      try {
        const results = await onSearch(searchQuery)
        setSearchResults(results)
      } catch (error) {
        console.error('Search failed:', error)
        setSearchResults([])
      } finally {
        setSearching(false)
      }
    }, 300)

    return () => clearTimeout(timer)
  }, [searchQuery, onSearch])

  const toggleSection = (section: keyof typeof expandedSections) => {
    setExpandedSections((prev) => ({
      ...prev,
      [section]: !prev[section],
    }))
  }

  const renderSection = (
    title: string,
    icon: React.ReactNode,
    items: PromptHistoryEntry[],
    sectionKey: keyof typeof expandedSections,
    color: string = 'text.secondary'
  ) => {
    if (items.length === 0) return null

    return (
      <>
        <ListItem
          button
          onClick={() => toggleSection(sectionKey)}
          sx={{
            py: 0.75,
            bgcolor: 'background.default',
          }}
        >
          <ListItemIcon sx={{ minWidth: 32, color }}>
            {icon}
          </ListItemIcon>
          <ListItemText
            primary={
              <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
                <Typography variant="subtitle2" sx={{ color, fontWeight: 600 }}>
                  {title}
                </Typography>
                <Chip
                  label={items.length}
                  size="small"
                  sx={{ height: 18, fontSize: '0.7rem' }}
                />
              </Box>
            }
          />
          {expandedSections[sectionKey] ? <ExpandLessIcon /> : <ExpandMoreIcon />}
        </ListItem>
        <Collapse in={expandedSections[sectionKey]} timeout="auto" unmountOnExit>
          <List component="div" disablePadding>
            {items.map((entry) => (
              <PromptItem
                key={entry.id}
                entry={entry}
                onSelect={() => onSelectPrompt(entry.content)}
                onPin={() => onPinPrompt(entry.id, !entry.pinned)}
              />
            ))}
          </List>
        </Collapse>
      </>
    )
  }

  return (
    <Box
      sx={{
        display: 'flex',
        flexDirection: 'column',
        height: '100%',
        bgcolor: 'background.paper',
        borderLeft: '1px solid',
        borderColor: 'divider',
      }}
    >
      {/* Header */}
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'space-between',
          px: 2,
          py: 1.5,
          borderBottom: '1px solid',
          borderColor: 'divider',
        }}
      >
        <Typography variant="h6" sx={{ fontWeight: 600, fontSize: '1rem' }}>
          Prompt Library
        </Typography>
        {onClose && (
          <IconButton size="small" onClick={onClose}>
            <CloseIcon sx={{ fontSize: 18 }} />
          </IconButton>
        )}
      </Box>

      {/* Search */}
      <Box sx={{ px: 2, py: 1.5, borderBottom: '1px solid', borderColor: 'divider' }}>
        <TextField
          size="small"
          fullWidth
          placeholder="Search prompts..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                {searching ? (
                  <CircularProgress size={18} />
                ) : (
                  <SearchIcon sx={{ fontSize: 18, color: 'text.secondary' }} />
                )}
              </InputAdornment>
            ),
            endAdornment: searchQuery && (
              <InputAdornment position="end">
                <IconButton size="small" onClick={() => setSearchQuery('')}>
                  <CloseIcon sx={{ fontSize: 16 }} />
                </IconButton>
              </InputAdornment>
            ),
          }}
          sx={{
            '& .MuiOutlinedInput-root': {
              fontSize: '0.875rem',
            },
          }}
        />
      </Box>

      {/* Content */}
      <Box sx={{ flex: 1, overflowY: 'auto' }}>
        {loading ? (
          <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
            <CircularProgress size={32} />
          </Box>
        ) : searchResults !== null ? (
          // Search results
          <List disablePadding>
            {searchResults.length === 0 ? (
              <ListItem>
                <ListItemText
                  primary="No results found"
                  secondary="Try a different search term"
                  primaryTypographyProps={{ color: 'text.secondary', textAlign: 'center' }}
                  secondaryTypographyProps={{ textAlign: 'center' }}
                />
              </ListItem>
            ) : (
              <>
                <ListItem sx={{ py: 0.75, bgcolor: 'background.default' }}>
                  <ListItemText
                    primary={
                      <Typography variant="subtitle2" sx={{ color: 'text.secondary', fontWeight: 600 }}>
                        Search Results ({searchResults.length})
                      </Typography>
                    }
                  />
                </ListItem>
                {searchResults.map((entry) => (
                  <PromptItem
                    key={entry.id}
                    entry={entry}
                    onSelect={() => onSelectPrompt(entry.content)}
                    onPin={() => onPinPrompt(entry.id, !entry.pinned)}
                  />
                ))}
              </>
            )}
          </List>
        ) : (
          // Normal view with sections
          <List disablePadding>
            {renderSection(
              'Pinned',
              <PushPinIcon sx={{ fontSize: 18 }} />,
              pinnedPrompts,
              'pinned',
              'warning.main'
            )}
            {renderSection(
              'Recent',
              <HistoryIcon sx={{ fontSize: 18 }} />,
              recentPrompts,
              'recent',
              'text.secondary'
            )}
            {pinnedPrompts.length === 0 && recentPrompts.length === 0 && (
              <ListItem>
                <ListItemText
                  primary="No prompts yet"
                  secondary="Send messages to build your prompt library"
                  primaryTypographyProps={{ color: 'text.secondary', textAlign: 'center' }}
                  secondaryTypographyProps={{ textAlign: 'center' }}
                />
              </ListItem>
            )}
          </List>
        )}
      </Box>
    </Box>
  )
}

export default PromptLibrarySidebar
