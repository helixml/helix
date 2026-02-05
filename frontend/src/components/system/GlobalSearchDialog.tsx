import React, { FC, useState, useMemo, useCallback, useEffect, useRef } from 'react'
import {
  Dialog,
  Box,
  Typography,
  CircularProgress,
  InputBase,
  Chip,
} from '@mui/material'
import { Search, X, FolderKanban, Bot, MessageSquare, FileText, BookOpen, GitBranch, ListTodo } from 'lucide-react'
import useLightTheme from '../../hooks/useLightTheme'
import useResourceSearch from '../../hooks/useResourceSearch'
import { TypesResource, TypesResourceSearchResult } from '../../api/api'

interface GlobalSearchDialogProps {
  open: boolean
  onClose: () => void
  organizationId: string
}

const SEARCHABLE_RESOURCE_TYPES: { type: TypesResource; label: string; icon: React.ReactNode }[] = [
  { type: TypesResource.ResourceProject, label: 'Projects', icon: <FolderKanban size={14} /> },
  { type: TypesResource.ResourceApplication, label: 'Agents', icon: <Bot size={14} /> },
  { type: TypesResource.ResourceSession, label: 'Sessions', icon: <MessageSquare size={14} /> },
  { type: TypesResource.ResourcePrompt, label: 'Prompts', icon: <FileText size={14} /> },
  { type: TypesResource.ResourceKnowledge, label: 'Knowledge', icon: <BookOpen size={14} /> },
  { type: TypesResource.ResourceGitRepository, label: 'Repositories', icon: <GitBranch size={14} /> },
  { type: TypesResource.ResourceSpecTask, label: 'Tasks', icon: <ListTodo size={14} /> },
]

const getIconForType = (type: TypesResource): React.ReactNode => {
  const found = SEARCHABLE_RESOURCE_TYPES.find(r => r.type === type)
  return found?.icon || <FileText size={16} />
}

const getLabelForType = (type: TypesResource): string => {
  const found = SEARCHABLE_RESOURCE_TYPES.find(r => r.type === type)
  return found?.label || type
}

const GlobalSearchDialog: FC<GlobalSearchDialogProps> = ({
  open,
  onClose,
  organizationId,
}) => {
  const lightTheme = useLightTheme()
  const inputRef = useRef<HTMLInputElement>(null)
  const [query, setQuery] = useState('')
  const [selectedTypes, setSelectedTypes] = useState<TypesResource[]>([])
  const [debouncedQuery, setDebouncedQuery] = useState('')

  useEffect(() => {
    const timer = setTimeout(() => {
      setDebouncedQuery(query)
    }, 300)
    return () => clearTimeout(timer)
  }, [query])

  useEffect(() => {
    if (open && inputRef.current) {
      setTimeout(() => inputRef.current?.focus(), 100)
    }
    if (!open) {
      setQuery('')
      setSelectedTypes([])
      setDebouncedQuery('')
    }
  }, [open])

  const { data, isLoading } = useResourceSearch({
    query: debouncedQuery,
    types: selectedTypes.length > 0 ? selectedTypes : SEARCHABLE_RESOURCE_TYPES.map(r => r.type),
    limit: 50,
    orgId: organizationId,
    enabled: debouncedQuery.length > 0,
  })

  const toggleType = useCallback((type: TypesResource) => {
    setSelectedTypes(prev => 
      prev.includes(type) 
        ? prev.filter(t => t !== type)
        : [...prev, type]
    )
  }, [])

  const groupedResults = useMemo(() => {
    if (!data?.results) return new Map<TypesResource, TypesResourceSearchResult[]>()
    
    const grouped = new Map<TypesResource, TypesResourceSearchResult[]>()
    for (const result of data.results) {
      if (!result.type) continue
      const existing = grouped.get(result.type) || []
      grouped.set(result.type, [...existing, result])
    }
    return grouped
  }, [data?.results])

  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    if (e.key === 'Escape') {
      onClose()
    }
  }, [onClose])

  return (
    <Dialog
      open={open}
      onClose={onClose}
      maxWidth={false}
      fullWidth
      onKeyDown={handleKeyDown}
      slotProps={{
        backdrop: {
          sx: {
            backgroundColor: 'rgba(0, 0, 0, 0.7)',
            backdropFilter: 'blur(12px)',
          }
        }
      }}
      PaperProps={{
        sx: {
          position: 'fixed',
          top: '5%',
          left: '10%',
          right: '10%',
          bottom: '5%',
          width: 'auto',
          maxWidth: 'none',
          maxHeight: 'none',
          height: 'auto',
          m: 0,
          borderRadius: '16px',
          backgroundColor: lightTheme.isDark ? 'rgba(18, 18, 22, 0.95)' : 'rgba(255, 255, 255, 0.95)',
          backdropFilter: 'blur(20px)',
          border: lightTheme.border,
          boxShadow: lightTheme.isDark 
            ? '0 25px 50px -12px rgba(0, 0, 0, 0.5), 0 0 0 1px rgba(255, 255, 255, 0.05)'
            : '0 25px 50px -12px rgba(0, 0, 0, 0.25)',
          overflow: 'hidden',
        }
      }}
    >
      <Box sx={{ 
        display: 'flex', 
        flexDirection: 'column', 
        height: '100%',
        minHeight: 0,
      }}>
        {/* Header with search input */}
        <Box sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 2,
          px: 4,
          py: 3,
          borderBottom: lightTheme.border,
        }}>
          <Box sx={{ flex: 1, display: 'flex', alignItems: 'center', gap: 2 }}>
            <InputBase
              inputRef={inputRef}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search across your workspace..."
              sx={{
                flex: 1,
                fontSize: '1.25rem',
                fontWeight: 400,
                color: lightTheme.textColor,
                '& input': {
                  p: 0,
                  '&::placeholder': {
                    color: lightTheme.textColorFaded,
                    opacity: 0.7,
                  },
                },
              }}
            />
          </Box>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 2 }}>
            {isLoading && <CircularProgress size={20} sx={{ color: lightTheme.textColorFaded }} />}
            <Search size={22} color={lightTheme.textColorFaded} />
          </Box>
        </Box>

        {/* Resource type pills */}
        <Box sx={{
          display: 'flex',
          flexWrap: 'wrap',
          gap: 1,
          px: 4,
          py: 2.5,
          borderBottom: lightTheme.border,
          backgroundColor: lightTheme.isDark ? 'rgba(255, 255, 255, 0.02)' : 'rgba(0, 0, 0, 0.02)',
        }}>
          {SEARCHABLE_RESOURCE_TYPES.map(({ type, label, icon }) => {
            const isSelected = selectedTypes.includes(type)
            return (
              <Chip
                key={type}
                label={
                  <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.75 }}>
                    {icon}
                    <span>{label}</span>
                  </Box>
                }
                onClick={() => toggleType(type)}
                sx={{
                  height: 32,
                  borderRadius: '8px',
                  fontSize: '0.8rem',
                  fontWeight: 500,
                  cursor: 'pointer',
                  transition: 'all 0.15s ease',
                  backgroundColor: isSelected 
                    ? lightTheme.isDark ? 'rgba(0, 213, 255, 0.15)' : 'rgba(0, 150, 200, 0.1)'
                    : 'transparent',
                  border: isSelected 
                    ? `1px solid ${lightTheme.isDark ? 'rgba(0, 213, 255, 0.5)' : 'rgba(0, 150, 200, 0.4)'}`
                    : `1px solid ${lightTheme.isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.1)'}`,
                  color: isSelected 
                    ? lightTheme.isDark ? '#00d5ff' : '#0096c8'
                    : lightTheme.textColorFaded,
                  '&:hover': {
                    backgroundColor: isSelected 
                      ? lightTheme.isDark ? 'rgba(0, 213, 255, 0.2)' : 'rgba(0, 150, 200, 0.15)'
                      : lightTheme.isDark ? 'rgba(255, 255, 255, 0.05)' : 'rgba(0, 0, 0, 0.05)',
                  },
                  '& .MuiChip-label': {
                    px: 1.5,
                  },
                }}
              />
            )
          })}
        </Box>

        {/* Results area */}
        <Box sx={{ 
          flex: 1, 
          overflow: 'auto',
          px: 4,
          py: 3,
          ...lightTheme.scrollbar,
        }}>
          {!debouncedQuery ? (
            <Box sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              gap: 2,
            }}>
              <Search size={48} color={lightTheme.textColorFaded} strokeWidth={1.5} />
              <Typography sx={{ 
                color: lightTheme.textColorFaded, 
                fontSize: '1rem',
                textAlign: 'center',
              }}>
                Type to search...
              </Typography>
            </Box>
          ) : groupedResults.size === 0 && !isLoading ? (
            <Box sx={{
              display: 'flex',
              flexDirection: 'column',
              alignItems: 'center',
              justifyContent: 'center',
              height: '100%',
              gap: 2,
            }}>
              <X size={48} color={lightTheme.textColorFaded} strokeWidth={1.5} />
              <Typography sx={{ 
                color: lightTheme.textColorFaded, 
                fontSize: '1rem',
                textAlign: 'center',
              }}>
                No results found for "{debouncedQuery}"
              </Typography>
            </Box>
          ) : (
            <Box sx={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
              {Array.from(groupedResults.entries()).map(([type, results]) => (
                <Box key={type}>
                  {/* Group header */}
                  <Box sx={{ 
                    display: 'flex', 
                    alignItems: 'center', 
                    gap: 1.5, 
                    mb: 2,
                    pb: 1.5,
                    borderBottom: `1px solid ${lightTheme.isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.06)'}`,
                  }}>
                    <Box sx={{ 
                      color: lightTheme.isDark ? '#00d5ff' : '#0096c8',
                      display: 'flex',
                      alignItems: 'center',
                    }}>
                      {getIconForType(type)}
                    </Box>
                    <Typography sx={{ 
                      fontSize: '0.75rem',
                      fontWeight: 600,
                      textTransform: 'uppercase',
                      letterSpacing: '0.5px',
                      color: lightTheme.textColorFaded,
                    }}>
                      {getLabelForType(type)}
                    </Typography>
                    <Typography sx={{ 
                      fontSize: '0.75rem',
                      color: lightTheme.textColorFaded,
                      opacity: 0.6,
                    }}>
                      ({results.length})
                    </Typography>
                  </Box>

                  {/* Results grid */}
                  <Box sx={{ 
                    display: 'grid',
                    gridTemplateColumns: 'repeat(auto-fill, minmax(300px, 1fr))',
                    gap: 2,
                  }}>
                    {results.map((result) => (
                      <Box
                        key={result.id}
                        sx={{
                          p: 2.5,
                          borderRadius: '12px',
                          backgroundColor: lightTheme.isDark ? 'rgba(255, 255, 255, 0.03)' : 'rgba(0, 0, 0, 0.02)',
                          border: `1px solid ${lightTheme.isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.06)'}`,
                          cursor: 'pointer',
                          transition: 'all 0.15s ease',
                          '&:hover': {
                            backgroundColor: lightTheme.isDark ? 'rgba(255, 255, 255, 0.06)' : 'rgba(0, 0, 0, 0.04)',
                            borderColor: lightTheme.isDark ? 'rgba(0, 213, 255, 0.3)' : 'rgba(0, 150, 200, 0.3)',
                            transform: 'translateY(-1px)',
                          },
                        }}
                      >
                        <Typography sx={{ 
                          fontSize: '0.95rem',
                          fontWeight: 500,
                          color: lightTheme.textColor,
                          mb: result.description ? 1 : 0,
                          overflow: 'hidden',
                          textOverflow: 'ellipsis',
                          whiteSpace: 'nowrap',
                        }}>
                          {result.name || 'Untitled'}
                        </Typography>
                        {result.description && (
                          <Typography sx={{ 
                            fontSize: '0.85rem',
                            color: lightTheme.textColorFaded,
                            overflow: 'hidden',
                            textOverflow: 'ellipsis',
                            display: '-webkit-box',
                            WebkitLineClamp: 2,
                            WebkitBoxOrient: 'vertical',
                            lineHeight: 1.5,
                          }}>
                            {result.description}
                          </Typography>
                        )}
                      </Box>
                    ))}
                  </Box>
                </Box>
              ))}
            </Box>
          )}
        </Box>

        {/* Footer with keyboard hints */}
        <Box sx={{
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'flex-end',
          gap: 3,
          px: 4,
          py: 2,
          borderTop: lightTheme.border,
          backgroundColor: lightTheme.isDark ? 'rgba(255, 255, 255, 0.02)' : 'rgba(0, 0, 0, 0.02)',
        }}>
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <Box sx={{
              px: 1,
              py: 0.25,
              borderRadius: '4px',
              backgroundColor: lightTheme.isDark ? 'rgba(255, 255, 255, 0.1)' : 'rgba(0, 0, 0, 0.08)',
              fontSize: '0.7rem',
              fontWeight: 500,
              color: lightTheme.textColorFaded,
            }}>
              ESC
            </Box>
            <Typography sx={{ fontSize: '0.75rem', color: lightTheme.textColorFaded }}>
              to close
            </Typography>
          </Box>
        </Box>
      </Box>
    </Dialog>
  )
}

export default GlobalSearchDialog
