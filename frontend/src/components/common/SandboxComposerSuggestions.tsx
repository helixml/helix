import React, { FC, useEffect } from 'react'
import {
  Box,
  CircularProgress,
  ListItemButton,
  ListItemIcon,
  Paper,
  Typography,
} from '@mui/material'
import { useTheme } from '@mui/material/styles'
import { Folder, PackageOpen } from 'lucide-react'
import { getChatColors } from '../session/chatStyles'
import ChangedFileIcon from '../session/ChangedFileIcon'
import {
  SandboxComposerSuggestion,
  SandboxComposerTrigger,
} from './sandboxComposerSuggestions.logic'

const SandboxComposerSuggestions: FC<{
  trigger: SandboxComposerTrigger
  items: SandboxComposerSuggestion[]
  loading: boolean
  error: boolean
  selectedIndex: number
  onSelectedIndexChange: (index: number) => void
  onSelect: (suggestion: SandboxComposerSuggestion) => void
}> = ({ trigger, items, loading, error, selectedIndex, onSelectedIndexChange, onSelect }) => {
  const theme = useTheme()

  useEffect(() => {
    if (selectedIndex >= items.length) onSelectedIndexChange(0)
  }, [items.length, onSelectedIndexChange, selectedIndex])

  return (
    <Paper
      elevation={8}
      role="listbox"
      aria-label={trigger.kind === 'file' ? 'Workspace files' : 'Sandbox skills'}
      sx={{
        position: 'absolute',
        left: 0,
        right: 0,
        bottom: 'calc(100% + 8px)',
        zIndex: 20,
        maxHeight: 288,
        overflowY: 'auto',
        border: '1px solid',
        borderColor: (theme) => getChatColors(theme).borderStrong,
        borderRadius: 2,
        bgcolor: (theme) => getChatColors(theme).composerSurface,
        backgroundImage: 'none',
        p: 0.25,
      }}
      onMouseDown={(event) => event.preventDefault()}
    >
      <Typography
        variant="caption"
        sx={{
          display: 'block',
          px: 1.25,
          pt: 0.75,
          pb: 0.375,
          color: (theme) => getChatColors(theme).muted,
          fontSize: 10,
          fontWeight: 600,
          letterSpacing: '0.08em',
          textTransform: 'uppercase',
        }}
      >
        {trigger.kind === 'file' ? 'Workspace files' : 'Skills'}
      </Typography>
      {loading ? (
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, px: 1.25, py: 1 }}>
          <CircularProgress size={14} />
          <Typography variant="caption" color="text.secondary">Searching…</Typography>
        </Box>
      ) : error ? (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', px: 1.25, py: 1 }}>
          Sandbox suggestions are unavailable while the desktop is stopped.
        </Typography>
      ) : items.length === 0 ? (
        <Typography variant="caption" color="text.secondary" sx={{ display: 'block', px: 1.25, py: 1 }}>
          {trigger.kind === 'file' ? 'No matching files.' : 'No matching skills.'}
        </Typography>
      ) : items.map((suggestion, index) => {
        const isFile = suggestion.kind === 'file'
        const filePath = isFile ? suggestion.entry.path || '' : ''
        const pathSegments = filePath.replace(/\/$/, '').split('/')
        const label = isFile ? pathSegments.at(-1) || filePath : `$${suggestion.entry.name || ''}`
        const description = isFile
          ? pathSegments.length > 1
            ? pathSegments.slice(0, -1).join('/')
            : suggestion.entry.kind === 'directory' ? 'Directory' : 'File'
          : suggestion.entry.description || `${suggestion.entry.scope || 'Sandbox'} skill`
        const icon = isFile
          ? suggestion.entry.kind === 'directory'
            ? <Folder size={14} />
            : <ChangedFileIcon
                path={suggestion.entry.path || ''}
                darkMode={theme.palette.mode === 'dark'}
                size={14}
              />
          : <PackageOpen size={14} />
        return (
          <ListItemButton
            key={suggestion.id}
            role="option"
            aria-selected={index === selectedIndex}
            selected={index === selectedIndex}
            onMouseMove={() => onSelectedIndexChange(index)}
            onClick={() => onSelect(suggestion)}
            sx={{
              borderRadius: 1.5,
              px: 1,
              py: 0.5,
              gap: 0.75,
              '&.Mui-selected': {
                bgcolor: (theme) => theme.palette.mode === 'dark'
                  ? 'rgba(255, 255, 255, 0.07)'
                  : 'rgba(0, 0, 0, 0.06)',
              },
              '&.Mui-selected:hover': {
                bgcolor: (theme) => theme.palette.mode === 'dark'
                  ? 'rgba(255, 255, 255, 0.09)'
                  : 'rgba(0, 0, 0, 0.08)',
              },
            }}
          >
            <ListItemIcon sx={{ minWidth: 18, color: 'text.secondary' }}>{icon}</ListItemIcon>
            <Box sx={{ minWidth: 0, flex: 1, display: 'flex', alignItems: 'baseline', gap: 1 }}>
              <Typography
                noWrap
                title={label}
                sx={{ maxWidth: '55%', color: 'text.primary', fontSize: 12.5, lineHeight: 1.35, fontWeight: 500, flexShrink: 0 }}
              >
                {label}
              </Typography>
              <Typography
                noWrap
                title={description}
                sx={{ minWidth: 0, color: 'text.secondary', fontSize: 11.5, lineHeight: 1.35 }}
              >
                {description}
              </Typography>
            </Box>
            {!isFile && (
              <Typography sx={{ color: 'text.secondary', fontSize: 11, lineHeight: 1.35, textTransform: 'capitalize' }}>
                {suggestion.entry.scope}
              </Typography>
            )}
          </ListItemButton>
        )
      })}
    </Paper>
  )
}

export default SandboxComposerSuggestions
