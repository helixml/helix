import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import { ChevronDown, ChevronRight } from 'lucide-react'

import useLightTheme from '../../hooks/useLightTheme'

type ProjectChatSectionHeaderProps = {
  label: string
  collapsed: boolean
  onToggle: () => void
  /** Controls kept on the right of the label (filters, add). */
  actions?: ReactNode
}

// Uppercase divider between the sidebar's sections (Bots / Projects / People).
// The whole label is the collapse control; actions sit outside it so a filter
// click never folds the section.
const ProjectChatSectionHeader: FC<ProjectChatSectionHeaderProps> = ({
  label,
  collapsed,
  onToggle,
  actions,
}) => {
  const lightTheme = useLightTheme()
  const color = lightTheme.isLight ? 'rgba(113,113,122,0.9)' : 'rgba(163,163,163,0.7)'
  return (
    <Box
      sx={{
        height: 28,
        mt: 1,
        px: 0.5,
        display: 'flex',
        alignItems: 'center',
        gap: 0.25,
        color,
      }}
    >
      <Box
        component="button"
        type="button"
        aria-label={`${collapsed ? 'Expand' : 'Collapse'} ${label}`}
        aria-expanded={!collapsed}
        onClick={onToggle}
        sx={{
          appearance: 'none',
          border: 0,
          p: 0,
          pl: 0.25,
          m: 0,
          flex: 1,
          minWidth: 0,
          height: '100%',
          display: 'flex',
          alignItems: 'center',
          gap: 0.5,
          backgroundColor: 'transparent',
          color: 'inherit',
          cursor: 'pointer',
          font: 'inherit',
          borderRadius: '4px',
          '&:hover': { color: lightTheme.isLight ? '#27272a' : '#f1f3f7' },
        }}
      >
        {collapsed ? <ChevronRight size={12} /> : <ChevronDown size={12} />}
        <Typography
          component="span"
          sx={{
            fontFamily: 'inherit',
            fontSize: '10.5px',
            fontWeight: 600,
            letterSpacing: '0.08em',
            textTransform: 'uppercase',
            lineHeight: 1,
          }}
        >
          {label}
        </Typography>
      </Box>
      {actions && (
        <Box sx={{ display: 'flex', alignItems: 'center', flexShrink: 0 }}>{actions}</Box>
      )}
    </Box>
  )
}

export default ProjectChatSectionHeader
