import { FC, ReactNode, useEffect, useRef, useState } from 'react'
import Box from '@mui/material/Box'
import Drawer from '@mui/material/Drawer'
import IconButton from '@mui/material/IconButton'
import InputBase from '@mui/material/InputBase'
import Typography from '@mui/material/Typography'

import { ListFilter, Search, SquarePen, X } from 'lucide-react'

import useLightTheme from '../../hooks/useLightTheme'

type ProjectChatSidebarMobileBarProps = {
  query: string
  onQueryChange: (query: string) => void
  onNewChat: () => void
  /** The same filter controls the desktop toolbar shows, for the sheet. */
  filters: ReactNode
}

/** Height the list has to keep clear so the bar never covers the last row. */
export const MOBILE_BAR_CLEARANCE = 84

/**
 * Filter, search and new-chat, within thumb reach.
 *
 * On a phone the desktop toolbar sits at the very top of a full-height list —
 * the hardest place on the screen to reach one-handed, and the first thing to
 * scroll past. These are the three things you actually do to a chat list, so
 * they float at the bottom instead, and the toolbar is hidden rather than
 * duplicated.
 */
const ProjectChatSidebarMobileBar: FC<ProjectChatSidebarMobileBarProps> = ({
  query,
  onQueryChange,
  onNewChat,
  filters,
}) => {
  const lightTheme = useLightTheme()
  const [searchOpen, setSearchOpen] = useState(false)
  const [filtersOpen, setFiltersOpen] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (searchOpen) inputRef.current?.focus()
  }, [searchOpen])

  // A query typed here has to stay reachable, so the field stays open while it
  // has content.
  const expanded = searchOpen || !!query

  const surface = lightTheme.isLight ? 'rgba(250,250,250,0.94)' : 'rgba(23,23,23,0.94)'
  const border = lightTheme.isLight ? 'rgba(0,0,0,0.10)' : 'rgba(255,255,255,0.10)'

  const roundButtonSx = {
    width: 44,
    height: 44,
    flexShrink: 0,
    borderRadius: '999px',
    border: '1px solid',
    borderColor: border,
    backgroundColor: surface,
    backdropFilter: 'blur(12px)',
    color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
    '&:hover': {
      backgroundColor: lightTheme.isLight ? 'rgba(240,240,240,0.98)' : 'rgba(38,38,38,0.98)',
    },
  } as const

  return (
    <>
      <Box
        sx={{
          position: 'absolute',
          left: 0,
          right: 0,
          bottom: 0,
          zIndex: 3,
          display: 'flex',
          alignItems: 'center',
          gap: 1,
          px: 1.5,
          pt: 1,
          pb: 'calc(12px + env(safe-area-inset-bottom))',
          // The list scrolls under the bar, so the fade stops rows from
          // colliding with it mid-scroll.
          background: `linear-gradient(to top, ${lightTheme.isLight ? '#fafafa' : '#000000'} 55%, transparent)`,
          pointerEvents: 'none',
          '& > *': { pointerEvents: 'auto' },
        }}
      >
        <IconButton
          aria-label="Filter and sort chats"
          onClick={() => setFiltersOpen(true)}
          sx={roundButtonSx}
        >
          <ListFilter size={18} strokeWidth={1.8} />
        </IconButton>

        <Box
          sx={{
            flex: 1,
            minWidth: 0,
            height: 44,
            display: 'flex',
            alignItems: 'center',
            gap: 1,
            px: 2,
            borderRadius: '999px',
            border: '1px solid',
            borderColor: border,
            backgroundColor: surface,
            backdropFilter: 'blur(12px)',
            cursor: 'text',
          }}
          onClick={() => setSearchOpen(true)}
        >
          <Search size={17} strokeWidth={1.8} style={{ opacity: 0.6, flexShrink: 0 }} />
          {expanded ? (
            <>
              <InputBase
                inputRef={inputRef}
                value={query}
                onChange={(event) => onQueryChange(event.target.value)}
                placeholder="Search"
                inputProps={{ 'aria-label': 'Search chats' }}
                sx={{ flex: 1, minWidth: 0, color: 'inherit', fontSize: '15px' }}
              />
              <IconButton
                aria-label="Clear search"
                size="small"
                onClick={(event) => {
                  event.stopPropagation()
                  onQueryChange('')
                  setSearchOpen(false)
                }}
                sx={{ color: 'inherit', flexShrink: 0 }}
              >
                <X size={16} />
              </IconButton>
            </>
          ) : (
            <Typography sx={{ fontSize: '15px', opacity: 0.6 }}>Search</Typography>
          )}
        </Box>

        <IconButton aria-label="New chat" onClick={onNewChat} sx={roundButtonSx}>
          <SquarePen size={18} strokeWidth={1.8} />
        </IconButton>
      </Box>

      <Drawer
        anchor="bottom"
        open={filtersOpen}
        onClose={() => setFiltersOpen(false)}
        PaperProps={{
          sx: {
            borderTopLeftRadius: 16,
            borderTopRightRadius: 16,
            pb: 'calc(16px + env(safe-area-inset-bottom))',
          },
        }}
      >
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
          <Typography sx={{ fontSize: '15px', fontWeight: 600 }}>Filter &amp; sort</Typography>
          <IconButton aria-label="Close filters" onClick={() => setFiltersOpen(false)}>
            <X size={18} />
          </IconButton>
        </Box>
        <Box sx={{ px: 1.5, py: 1.5, display: 'flex', alignItems: 'center', flexWrap: 'wrap', gap: 1 }}>
          {filters}
        </Box>
      </Drawer>
    </>
  )
}

export default ProjectChatSidebarMobileBar
