import { FC, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Dialog from '@mui/material/Dialog'
import InputBase from '@mui/material/InputBase'
import Typography from '@mui/material/Typography'
import useMediaQuery from '@mui/material/useMediaQuery'
import { useTheme } from '@mui/material/styles'

import { ArrowLeft, Folder, MessagesSquare } from 'lucide-react'

import type { TypesProject } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import { matchesAllTokens } from '../../utils/searchUtils'

export type NewChatTarget = { projectId?: string }

type NewChatProjectDialogProps = {
  open: boolean
  projects: TypesProject[]
  onClose: () => void
  onSelect: (target: NewChatTarget) => void
}

type Row = {
  key: string
  name: string
  detail: string
  target: NewChatTarget
  standalone?: boolean
}

const isMacPlatform = (): boolean =>
  typeof navigator !== 'undefined' && /Mac|iPhone|iPad/.test(navigator.platform || '')

/** Only the first nine rows can carry a modifier-digit shortcut. */
const SHORTCUT_LIMIT = 9

const projectDetail = (project: TypesProject): string =>
  project.github_repo_url || project.description || ''

export const buildNewChatRows = (projects: TypesProject[], query: string): Row[] => {
  const rows: Row[] = [
    {
      key: 'none',
      name: 'No project',
      detail: 'A standalone chat, not attached to a repository',
      target: {},
      standalone: true,
    },
    ...projects.flatMap((project) => (project.id
      ? [{
          key: project.id,
          name: project.name || 'Untitled project',
          detail: projectDetail(project),
          target: { projectId: project.id },
        }]
      : [])),
  ]

  return rows.filter((row) => matchesAllTokens(query, row.name, row.detail))
}

/**
 * Pick where a new chat goes.
 *
 * The bottom-bar compose button on a phone and ⌘⇧O on the desktop both land
 * here: choosing the project is the first decision of a new chat, and making it
 * up front is also the quickest way to move between projects.
 */
const NewChatProjectDialog: FC<NewChatProjectDialogProps> = ({
  open,
  projects,
  onClose,
  onSelect,
}) => {
  const theme = useTheme()
  const lightTheme = useLightTheme()
  const isNarrow = useMediaQuery(theme.breakpoints.down('sm'))
  const inputRef = useRef<HTMLInputElement>(null)
  const [query, setQuery] = useState('')
  const [selectedIndex, setSelectedIndex] = useState(0)

  const rows = useMemo(() => buildNewChatRows(projects, query), [projects, query])

  useEffect(() => {
    if (!open) return
    setQuery('')
    setSelectedIndex(0)
  }, [open])

  // A filtered list can be shorter than the cursor that was sitting in it.
  useEffect(() => {
    setSelectedIndex((current) => (current >= rows.length ? Math.max(rows.length - 1, 0) : current))
  }, [rows.length])

  const choose = useCallback((row?: Row) => {
    if (!row) return
    onSelect(row.target)
    onClose()
  }, [onClose, onSelect])

  const handleKeyDown = useCallback((event: React.KeyboardEvent) => {
    if (event.key === 'ArrowDown') {
      event.preventDefault()
      setSelectedIndex((current) => (rows.length ? (current + 1) % rows.length : 0))
      return
    }
    if (event.key === 'ArrowUp') {
      event.preventDefault()
      setSelectedIndex((current) => (rows.length ? (current - 1 + rows.length) % rows.length : 0))
      return
    }
    if (event.key === 'Enter') {
      event.preventDefault()
      choose(rows[selectedIndex])
      return
    }
    // ⌘1…⌘9 jumps straight to a row, matching the hints in the list.
    const modifier = isMacPlatform() ? event.metaKey : event.ctrlKey
    if (modifier && /^[1-9]$/.test(event.key)) {
      const index = Number(event.key) - 1
      if (index < rows.length && index < SHORTCUT_LIMIT) {
        event.preventDefault()
        choose(rows[index])
      }
    }
  }, [choose, rows, selectedIndex])

  const shortcutLabel = isMacPlatform() ? '⌘' : 'Ctrl+'
  const mutedColor = lightTheme.isLight ? 'rgba(113,113,122,0.9)' : 'rgba(163,163,163,0.75)'

  return (
    <Dialog
      open={open}
      onClose={onClose}
      fullWidth
      maxWidth="sm"
      fullScreen={isNarrow}
      onKeyDown={handleKeyDown}
      TransitionProps={{ onEntered: () => inputRef.current?.focus() }}
      PaperProps={{
        sx: {
          ...(isNarrow
            ? {
                m: 0,
                borderRadius: 0,
                height: '100%',
                maxHeight: '100%',
                pt: 'env(safe-area-inset-top)',
                pb: 'env(safe-area-inset-bottom)',
              }
            : {
                position: 'fixed',
                top: '12%',
                m: 0,
                borderRadius: '14px',
              }),
          display: 'flex',
          flexDirection: 'column',
          overflow: 'hidden',
        },
      }}
    >
      <Box
        sx={{
          display: 'flex',
          alignItems: 'center',
          gap: 1.5,
          px: 2,
          py: 1.5,
          borderBottom: '1px solid',
          borderColor: 'divider',
          flexShrink: 0,
        }}
      >
        <Box
          component="button"
          type="button"
          aria-label="Close new chat picker"
          onClick={onClose}
          sx={{
            appearance: 'none',
            border: 0,
            p: 0,
            backgroundColor: 'transparent',
            color: mutedColor,
            cursor: 'pointer',
            display: 'inline-flex',
            alignItems: 'center',
            justifyContent: 'center',
            width: 30,
            height: 30,
            flexShrink: 0,
            '&:hover': { color: 'text.primary' },
          }}
        >
          <ArrowLeft size={18} />
        </Box>
        <InputBase
          inputRef={inputRef}
          autoFocus
          value={query}
          onChange={(event) => {
            setQuery(event.target.value)
            setSelectedIndex(0)
          }}
          placeholder="Search…"
          inputProps={{ 'aria-label': 'Search projects' }}
          sx={{ flex: 1, fontSize: '15px' }}
        />
      </Box>

      <Box
        sx={{
          flex: 1,
          minHeight: 0,
          overflowY: 'auto',
          overscrollBehavior: 'contain',
          WebkitOverflowScrolling: 'touch',
          py: 1,
        }}
      >
        <Typography
          sx={{
            px: 2,
            py: 0.75,
            fontSize: '11px',
            fontWeight: 600,
            letterSpacing: '0.4px',
            color: mutedColor,
          }}
        >
          Projects
        </Typography>
        {rows.length === 0 && (
          <Typography sx={{ px: 2, py: 1.5, fontSize: '13px', color: mutedColor }}>
            No projects match “{query}”.
          </Typography>
        )}
        {rows.map((row, index) => (
          <Box
            key={row.key}
            role="button"
            tabIndex={-1}
            data-new-chat-row={row.key}
            aria-label={`New chat in ${row.name}`}
            onMouseEnter={() => setSelectedIndex(index)}
            onClick={() => choose(row)}
            sx={{
              display: 'flex',
              alignItems: 'center',
              gap: 1.5,
              mx: 1,
              px: 1.5,
              // Comfortable to tap, and tall enough for two lines of text.
              minHeight: 52,
              borderRadius: '8px',
              cursor: 'pointer',
              backgroundColor: index === selectedIndex
                ? (lightTheme.isLight ? 'rgba(0,0,0,0.05)' : 'rgba(241,243,247,0.09)')
                : 'transparent',
            }}
          >
            <Box sx={{ display: 'inline-flex', flexShrink: 0, color: mutedColor }}>
              {row.standalone ? <MessagesSquare size={16} /> : <Folder size={16} />}
            </Box>
            <Box sx={{ minWidth: 0, flex: 1 }}>
              <Typography
                noWrap
                sx={{ fontSize: '14px', fontWeight: 500, lineHeight: '20px' }}
              >
                {row.name}
              </Typography>
              {row.detail && (
                <Typography
                  noWrap
                  sx={{ fontSize: '12px', lineHeight: '16px', color: mutedColor }}
                >
                  {row.detail}
                </Typography>
              )}
            </Box>
            {index < SHORTCUT_LIMIT && !isNarrow && (
              <Typography
                sx={{
                  flexShrink: 0,
                  fontSize: '11px',
                  color: mutedColor,
                  fontVariantNumeric: 'tabular-nums',
                }}
              >
                {shortcutLabel}{index + 1}
              </Typography>
            )}
          </Box>
        ))}
      </Box>

      {!isNarrow && (
        <Box
          sx={{
            display: 'flex',
            alignItems: 'center',
            gap: 2,
            px: 2,
            py: 1,
            borderTop: '1px solid',
            borderColor: 'divider',
            fontSize: '11px',
            color: mutedColor,
            flexShrink: 0,
          }}
        >
          <span>↑↓ Navigate</span>
          <span>Enter Select</span>
          <span>Esc Close</span>
        </Box>
      )}
    </Dialog>
  )
}

export default NewChatProjectDialog
