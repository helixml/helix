import { FC, MouseEvent, useState } from 'react'
import Box from '@mui/material/Box'
import ClickAwayListener from '@mui/material/ClickAwayListener'
import IconButton from '@mui/material/IconButton'
import List from '@mui/material/List'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemText from '@mui/material/ListItemText'
import Paper from '@mui/material/Paper'
import Popper from '@mui/material/Popper'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { Check, Folder, LayoutList, Users } from 'lucide-react'

import useLightTheme from '../../hooks/useLightTheme'
import type { SidebarGroupBy } from './ProjectChatSidebar.logic'

type ProjectChatGroupByControlProps = {
  value: SidebarGroupBy
  onChange: (value: SidebarGroupBy) => void
}

const OPTIONS: Array<{ value: SidebarGroupBy; label: string; description: string; icon: JSX.Element }> = [
  { value: 'project', label: 'Project', description: 'Every project, with everyone working in it', icon: <Folder size={15} /> },
  { value: 'person', label: 'Person', description: 'Every member, with what they are working on', icon: <Users size={15} /> },
]

// The toolbar's "group by" picker: one small dialog, two arrangements. The
// sidebar shows exactly one of them, never both.
const ProjectChatGroupByControl: FC<ProjectChatGroupByControlProps> = ({ value, onChange }) => {
  const lightTheme = useLightTheme()
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const open = !!anchorEl
  const surface = lightTheme.isLight ? '#ffffff' : '#191919'
  const selectedSurface = lightTheme.isLight ? 'rgba(24,24,27,0.07)' : 'rgba(255,255,255,0.08)'
  const current = OPTIONS.find((option) => option.value === value) || OPTIONS[0]

  const openPicker = (event: MouseEvent<HTMLElement>) => setAnchorEl(event.currentTarget)
  const closePicker = () => setAnchorEl(null)

  return (
    <>
      <Tooltip title={`Grouped by ${current.label.toLowerCase()}`}>
        <IconButton
          size="small"
          aria-label="Group chats by"
          aria-haspopup="dialog"
          aria-expanded={open || undefined}
          onClick={openPicker}
          sx={{
            color: value === 'person'
              ? (lightTheme.isLight ? '#27272a' : '#f1f3f7')
              : (lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.55)'),
          }}
        >
          <LayoutList size={15} strokeWidth={1.7} />
        </IconButton>
      </Tooltip>
      <Popper
        anchorEl={anchorEl}
        open={open}
        placement="bottom-end"
        modifiers={[{ name: 'offset', options: { offset: [0, 4] } }]}
        sx={{ zIndex: (theme) => theme.zIndex.modal + 1 }}
      >
        <ClickAwayListener onClickAway={closePicker}>
          <Paper
            role="dialog"
            aria-label="Group chats by"
            onKeyDown={(event) => {
              if (event.key === 'Escape') closePicker()
            }}
            sx={{
              width: 260,
              overflow: 'hidden',
              borderRadius: '9px',
              border: `1px solid ${lightTheme.isLight ? 'rgba(24,24,27,0.12)' : 'rgba(255,255,255,0.10)'}`,
              backgroundColor: surface,
              backgroundImage: 'none',
              boxShadow: lightTheme.isLight
                ? '0 12px 32px rgba(0,0,0,0.16)'
                : '0 12px 32px rgba(0,0,0,0.48)',
            }}
          >
            <Typography
              sx={{
                px: 1.5,
                pt: 1.25,
                pb: 0.5,
                fontSize: '10.5px',
                fontWeight: 600,
                letterSpacing: '0.08em',
                textTransform: 'uppercase',
                color: lightTheme.isLight ? 'rgba(113,113,122,0.9)' : 'rgba(163,163,163,0.7)',
              }}
            >
              Group by
            </Typography>
            <List dense disablePadding sx={{ pb: 0.75 }}>
              {OPTIONS.map((option) => {
                const selected = option.value === value
                return (
                  <ListItemButton
                    key={option.value}
                    role="radio"
                    aria-checked={selected}
                    selected={selected}
                    onClick={() => {
                      onChange(option.value)
                      closePicker()
                    }}
                    sx={{
                      mx: 0.75,
                      my: 0.25,
                      px: 1,
                      py: 0.75,
                      borderRadius: '6px',
                      gap: 1,
                      '&.Mui-selected': { backgroundColor: selectedSurface },
                    }}
                  >
                    <Box sx={{ display: 'inline-flex', color: 'text.secondary', flexShrink: 0 }}>{option.icon}</Box>
                    <ListItemText
                      primary={option.label}
                      secondary={option.description}
                      primaryTypographyProps={{ fontSize: '13px', fontWeight: 500 }}
                      secondaryTypographyProps={{ fontSize: '11px' }}
                    />
                    <Box sx={{ width: 16, display: 'inline-flex', justifyContent: 'center', flexShrink: 0 }}>
                      {selected && <Check size={14} />}
                    </Box>
                  </ListItemButton>
                )
              })}
            </List>
          </Paper>
        </ClickAwayListener>
      </Popper>
    </>
  )
}

export default ProjectChatGroupByControl
