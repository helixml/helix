import { FC, ReactNode } from 'react'
import Box from '@mui/material/Box'

import useLightTheme from '../../hooks/useLightTheme'
import { getSidebarColors } from '../../styles/themeTokens'

// Shared geometry for the sidebar's collapsible groups (projects, org bots,
// people). Every group header lays out the same fixed columns:
//
//   [pad][chevron][gap][icon][gap][label ...]
//
// and the group's rows start at the header's icon column: clearly inside the
// parent (right of its chevron) without spending the label inset on every
// child title.
export const SIDEBAR_TREE_ROW_PADDING_X = 6
export const SIDEBAR_TREE_CHEVRON_SIZE = 16
export const SIDEBAR_TREE_CHEVRON_SIZE_PHONE = 28
export const SIDEBAR_TREE_GAP = 6
export const SIDEBAR_TREE_ICON_SIZE = 22
// Child rows (`ProjectChatItemRow`) carry their own `px: 1`.
const CHILD_ROW_PADDING_X = 8

export const sidebarTreeChevronSize = (isPhone: boolean): number => (
  isPhone ? SIDEBAR_TREE_CHEVRON_SIZE_PHONE : SIDEBAR_TREE_CHEVRON_SIZE
)

// x offset of the header's icon column, from the group's left edge.
export const sidebarTreeIconInset = (isPhone: boolean): number => (
  SIDEBAR_TREE_ROW_PADDING_X + sidebarTreeChevronSize(isPhone) + SIDEBAR_TREE_GAP
)

// Spread into a group header row's `sx` so every header shares the columns.
export const sidebarTreeHeaderSx = {
  height: 32,
  px: `${SIDEBAR_TREE_ROW_PADDING_X}px`,
  display: 'flex',
  alignItems: 'center',
  gap: `${SIDEBAR_TREE_GAP}px`,
  borderRadius: '6px',
} as const

// Fixed-size slot for a header's leading icon (folder, bot portrait, avatar),
// so icons of different sizes never move the label column.
export const SidebarTreeIconSlot: FC<{ children: ReactNode }> = ({ children }) => (
  <Box
    component="span"
    data-testid="sidebar-tree-icon-slot"
    sx={{
      position: 'relative',
      width: SIDEBAR_TREE_ICON_SIZE,
      height: SIDEBAR_TREE_ICON_SIZE,
      flexShrink: 0,
      display: 'inline-flex',
      alignItems: 'center',
      justifyContent: 'center',
    }}
  >
    {children}
  </Box>
)

// The rows of an expanded group: indented to the header's icon column, with
// a hairline guide under the header chevron tying them to their parent.
export const SidebarTreeChildren: FC<{ isPhone: boolean; children: ReactNode }> = ({ isPhone, children }) => {
  const lightTheme = useLightTheme()
  const sidebarColors = getSidebarColors(lightTheme.isLight)
  const guideX = SIDEBAR_TREE_ROW_PADDING_X + Math.floor(sidebarTreeChevronSize(isPhone) / 2)
  return (
    <Box
      data-testid="sidebar-tree-children"
      sx={{
        position: 'relative',
        pl: `${sidebarTreeIconInset(isPhone) - CHILD_ROW_PADDING_X}px`,
        '&::before': {
          content: '""',
          position: 'absolute',
          left: guideX,
          top: 2,
          bottom: 6,
          width: '1px',
          backgroundColor: sidebarColors.border,
          pointerEvents: 'none',
        },
      }}
    >
      {children}
    </Box>
  )
}
