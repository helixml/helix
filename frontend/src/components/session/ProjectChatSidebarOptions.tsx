import { FC, MouseEvent, useState } from 'react'
import Box from '@mui/material/Box'
import ClickAwayListener from '@mui/material/ClickAwayListener'
import IconButton from '@mui/material/IconButton'
import Paper from '@mui/material/Paper'
import Popper from '@mui/material/Popper'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { ArrowUpDown, Minus, Plus } from 'lucide-react'

import useLightTheme from '../../hooks/useLightTheme'
import {
  MAX_VISIBLE_THREAD_COUNT,
  MIN_VISIBLE_THREAD_COUNT,
} from './ProjectChatSidebar.logic'
import type {
  SidebarProjectSortOrder,
  SidebarThreadSortOrder,
} from './ProjectChatSidebar.logic'

const PROJECT_SORT_OPTIONS: Array<{ value: SidebarProjectSortOrder; label: string }> = [
  { value: 'updated_at', label: 'Last user message' },
  { value: 'created_at', label: 'Created at' },
  { value: 'manual', label: 'Manual' },
]

const THREAD_SORT_OPTIONS: Array<{ value: SidebarThreadSortOrder; label: string }> = [
  { value: 'updated_at', label: 'Last user message' },
  { value: 'created_at', label: 'Created at' },
]

type ProjectChatSidebarOptionsProps = {
  projectSortOrder: SidebarProjectSortOrder
  threadSortOrder: SidebarThreadSortOrder
  visibleThreadCount: number
  onProjectSortOrderChange: (value: SidebarProjectSortOrder) => void
  onThreadSortOrderChange: (value: SidebarThreadSortOrder) => void
  onVisibleThreadCountChange: (value: number) => void
}

const ProjectChatSidebarOptions: FC<ProjectChatSidebarOptionsProps> = ({
  projectSortOrder,
  threadSortOrder,
  visibleThreadCount,
  onProjectSortOrderChange,
  onThreadSortOrderChange,
  onVisibleThreadCountChange,
}) => {
  const lightTheme = useLightTheme()
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const open = !!anchorEl
  const muted = lightTheme.isLight ? '#71717a' : '#8f8f94'
  const surface = lightTheme.isLight ? '#ffffff' : '#191919'
  const selected = lightTheme.isLight ? 'rgba(24,24,27,0.07)' : 'rgba(255,255,255,0.08)'

  const openMenu = (event: MouseEvent<HTMLElement>) => setAnchorEl(event.currentTarget)
  const closeMenu = () => setAnchorEl(null)
  const sectionLabelSx = {
    px: 1,
    pt: 0.75,
    pb: 0.4,
    color: muted,
    fontSize: '12px',
    fontWeight: 500,
    lineHeight: 1.4,
  }
  const optionSx = {
    appearance: 'none',
    width: 'calc(100% - 8px)',
    minHeight: 28,
    mx: 0.5,
    px: 1,
    py: 0.25,
    display: 'flex',
    alignItems: 'center',
    border: 0,
    borderRadius: '6px',
    backgroundColor: 'transparent',
    color: 'inherit',
    cursor: 'pointer',
    font: 'inherit',
    fontSize: '13px',
    textAlign: 'left',
    '&:hover, &:focus-visible': { backgroundColor: selected, outline: 'none' },
  }

  return (
    <>
      <Tooltip title="Sidebar options">
        <IconButton
          size="small"
          aria-label="Sidebar options"
          aria-haspopup="menu"
          aria-expanded={open || undefined}
          onClick={openMenu}
          sx={{
            color: lightTheme.isLight
              ? 'rgba(113,113,122,0.65)'
              : 'rgba(163,163,163,0.55)',
          }}
        >
          <ArrowUpDown size={15} strokeWidth={1.7} />
        </IconButton>
      </Tooltip>
      <Popper
        anchorEl={anchorEl}
        open={open}
        placement="bottom-end"
        modifiers={[{ name: 'offset', options: { offset: [0, 4] } }]}
        sx={{ zIndex: (theme) => theme.zIndex.modal + 1 }}
      >
        <ClickAwayListener onClickAway={closeMenu}>
          <Paper
            role="menu"
            aria-label="Sidebar options"
            onKeyDown={(event) => {
              if (event.key === 'Escape') closeMenu()
            }}
            sx={{
              minWidth: 168,
              py: 0.5,
              borderRadius: '9px',
              border: `1px solid ${lightTheme.isLight ? 'rgba(24,24,27,0.12)' : 'rgba(255,255,255,0.10)'}`,
              backgroundColor: surface,
              backgroundImage: 'none',
              color: 'text.primary',
              boxShadow: lightTheme.isLight
                ? '0 12px 32px rgba(0,0,0,0.16)'
                : '0 12px 32px rgba(0,0,0,0.48)',
            }}
          >
            <Typography sx={sectionLabelSx}>Sort projects</Typography>
            {PROJECT_SORT_OPTIONS.map((option) => (
              <Box
                component="button"
                type="button"
                role="menuitemradio"
                aria-checked={projectSortOrder === option.value}
                key={option.value}
                onClick={() => onProjectSortOrderChange(option.value)}
                sx={{
                  ...optionSx,
                  backgroundColor: projectSortOrder === option.value ? selected : 'transparent',
                }}
              >
                {option.label}
              </Box>
            ))}

            <Typography sx={{ ...sectionLabelSx, pt: 1.25 }}>Sort threads</Typography>
            {THREAD_SORT_OPTIONS.map((option) => (
              <Box
                component="button"
                type="button"
                role="menuitemradio"
                aria-checked={threadSortOrder === option.value}
                key={option.value}
                onClick={() => onThreadSortOrderChange(option.value)}
                sx={{
                  ...optionSx,
                  backgroundColor: threadSortOrder === option.value ? selected : 'transparent',
                }}
              >
                {option.label}
              </Box>
            ))}

            <Typography sx={{ ...sectionLabelSx, pt: 1.25 }}>Visible threads</Typography>
            <Box
              sx={{
                height: 30,
                mx: 1,
                mb: 0.5,
                display: 'grid',
                gridTemplateColumns: '32px 1fr 32px',
                alignItems: 'center',
                border: `1px solid ${lightTheme.isLight ? 'rgba(24,24,27,0.16)' : 'rgba(255,255,255,0.14)'}`,
                borderRadius: '8px',
              }}
            >
              <IconButton
                size="small"
                aria-label="Decrease visible thread count"
                disabled={visibleThreadCount <= MIN_VISIBLE_THREAD_COUNT}
                onClick={() => onVisibleThreadCountChange(visibleThreadCount - 1)}
                sx={{ width: 32, height: 28, borderRadius: '7px' }}
              >
                <Minus size={14} />
              </IconButton>
              <Typography
                aria-label="Visible thread count"
                sx={{ textAlign: 'center', fontSize: '13px', fontVariantNumeric: 'tabular-nums' }}
              >
                {visibleThreadCount}
              </Typography>
              <IconButton
                size="small"
                aria-label="Increase visible thread count"
                disabled={visibleThreadCount >= MAX_VISIBLE_THREAD_COUNT}
                onClick={() => onVisibleThreadCountChange(visibleThreadCount + 1)}
                sx={{ width: 32, height: 28, borderRadius: '7px' }}
              >
                <Plus size={14} />
              </IconButton>
            </Box>
          </Paper>
        </ClickAwayListener>
      </Popper>
    </>
  )
}

export default ProjectChatSidebarOptions
