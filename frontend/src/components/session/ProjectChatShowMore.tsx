import { FC } from 'react'
import Box from '@mui/material/Box'

import useLightTheme from '../../hooks/useLightTheme'
import type { SidebarItemPagination } from './useSidebarItemPagination'

type ProjectChatShowMoreProps = {
  pagination: SidebarItemPagination
  hasMore: boolean
  fetching: boolean
}

// The "Show more / Show less" footer under a sidebar group.
const ProjectChatShowMore: FC<ProjectChatShowMoreProps> = ({ pagination, hasMore, fetching }) => {
  const lightTheme = useLightTheme()
  if (!pagination.canShowLess && !hasMore) return null
  const buttonSx = {
    appearance: 'none',
    border: 0,
    height: 30,
    px: 1,
    backgroundColor: 'transparent',
    color: lightTheme.isLight ? 'rgba(113,113,122,0.75)' : 'rgba(163,163,163,0.75)',
    cursor: fetching ? 'default' : 'pointer',
    font: 'inherit',
    fontSize: '12px',
    '&:hover': {
      color: lightTheme.isLight ? '#27272a' : '#f1f3f7',
      backgroundColor: lightTheme.isLight ? '#fdfdfd' : 'rgba(241,243,247,0.08)',
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
