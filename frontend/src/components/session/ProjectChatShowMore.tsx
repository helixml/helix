import { FC } from 'react'
import Box from '@mui/material/Box'

import useLightTheme from '../../hooks/useLightTheme'
import { getSidebarColors } from '../../styles/themeTokens'
import { TYPOGRAPHY } from '../../styles/typography'
import type { SidebarItemPagination } from './useSidebarItemPagination'

type ProjectChatShowMoreProps = {
  pagination: SidebarItemPagination
  hasMore: boolean
  fetching: boolean
}

// The "Show more / Show less" footer under a sidebar group.
const ProjectChatShowMore: FC<ProjectChatShowMoreProps> = ({ pagination, hasMore, fetching }) => {
  const lightTheme = useLightTheme()
  const sidebarColors = getSidebarColors(lightTheme.isLight)
  if (!pagination.canShowLess && !hasMore) return null
  const buttonSx = {
    appearance: 'none',
    border: 0,
    height: 30,
    px: 1,
    backgroundColor: 'transparent',
    color: sidebarColors.subtleForeground,
    cursor: fetching ? 'default' : 'pointer',
    font: 'inherit',
    fontSize: TYPOGRAPHY.sidebar.metadataFontSize,
    '&:hover': {
      color: sidebarColors.foreground,
      backgroundColor: sidebarColors.rowHover,
    },
  }
  return (
    <Box sx={{ display: 'flex', alignItems: 'center', gap: 0.25 }}>
      {pagination.canShowLess && (
        <Box component="button" type="button" disabled={fetching} onClick={pagination.showLess} sx={buttonSx}>
          Show less
        </Box>
      )}
      {hasMore && (
        <Box component="button" type="button" disabled={fetching} onClick={pagination.showMore} sx={buttonSx}>
          {fetching ? 'Loading…' : 'Show more'}
        </Box>
      )}
    </Box>
  )
}

export default ProjectChatShowMore
