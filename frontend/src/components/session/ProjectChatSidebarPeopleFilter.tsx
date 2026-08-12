import { FC, MouseEvent, useState } from 'react'
import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import Checkbox from '@mui/material/Checkbox'
import ClickAwayListener from '@mui/material/ClickAwayListener'
import IconButton from '@mui/material/IconButton'
import InputAdornment from '@mui/material/InputAdornment'
import List from '@mui/material/List'
import ListItemAvatar from '@mui/material/ListItemAvatar'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemText from '@mui/material/ListItemText'
import Paper from '@mui/material/Paper'
import Popper from '@mui/material/Popper'
import TextField from '@mui/material/TextField'
import Tooltip from '@mui/material/Tooltip'
import Typography from '@mui/material/Typography'
import { Search, UsersRound } from 'lucide-react'

import type { TypesOrganizationMembership, TypesUser } from '../../api/api'
import useLightTheme from '../../hooks/useLightTheme'
import { getUserInitials } from '../../utils/user'
import { getSidebarMemberResults } from './ProjectChatSidebar.logic'

type ProjectChatSidebarPeopleFilterProps = {
  members: TypesOrganizationMembership[]
  currentUser?: TypesUser
  selectedUserIds: string[]
  onSelectedUserIdsChange: (userIds: string[]) => void
}

const memberLabel = (user?: TypesUser): string => (
  user?.full_name || user?.username || user?.email || 'Unknown user'
)

const ProjectChatSidebarPeopleFilter: FC<ProjectChatSidebarPeopleFilterProps> = ({
  members,
  currentUser,
  selectedUserIds,
  onSelectedUserIdsChange,
}) => {
  const lightTheme = useLightTheme()
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const [query, setQuery] = useState('')
  const currentUserId = currentUser?.id || ''
  const open = !!anchorEl
  const selectedUserIdSet = new Set(selectedUserIds)
  const memberResults = getSidebarMemberResults(members, query, currentUserId, selectedUserIds)
  const surface = lightTheme.isLight ? '#ffffff' : '#191919'
  const selectedSurface = lightTheme.isLight ? 'rgba(24,24,27,0.07)' : 'rgba(255,255,255,0.08)'

  const openFilter = (event: MouseEvent<HTMLElement>) => setAnchorEl(event.currentTarget)
  const closeFilter = () => {
    setAnchorEl(null)
    setQuery('')
  }

  const toggleMember = (userId: string) => {
    if (!userId) return
    if (selectedUserIdSet.has(userId)) {
      onSelectedUserIdsChange(selectedUserIds.filter((selectedUserId) => selectedUserId !== userId))
      return
    }
    onSelectedUserIdsChange([...selectedUserIds, userId])
  }

  return (
    <>
      <Tooltip title={`Filter people (${selectedUserIds.length} selected)`}>
        <IconButton
          size="small"
          aria-label="Filter tasks by people"
          aria-haspopup="dialog"
          aria-expanded={open || undefined}
          onClick={openFilter}
          sx={{
            color: selectedUserIds.length > 1
              ? (lightTheme.isLight ? '#27272a' : '#f1f3f7')
              : (lightTheme.isLight ? 'rgba(113,113,122,0.65)' : 'rgba(163,163,163,0.55)'),
          }}
        >
          <UsersRound size={15} strokeWidth={1.7} />
        </IconButton>
      </Tooltip>
      <Popper
        anchorEl={anchorEl}
        open={open}
        placement="bottom-end"
        modifiers={[{ name: 'offset', options: { offset: [0, 4] } }]}
        sx={{ zIndex: (theme) => theme.zIndex.modal + 1 }}
      >
        <ClickAwayListener onClickAway={closeFilter}>
          <Paper
            role="dialog"
            aria-label="Filter tasks by people"
            onKeyDown={(event) => {
              if (event.key === 'Escape') closeFilter()
            }}
            sx={{
              width: 280,
              maxHeight: 400,
              display: 'flex',
              flexDirection: 'column',
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
            <Box sx={{ p: 1 }}>
              <TextField
                autoFocus
                fullWidth
                size="small"
                value={query}
                onChange={(event) => setQuery(event.target.value)}
                placeholder="Search name or email"
                inputProps={{ 'aria-label': 'Search organization members' }}
                InputProps={{
                  startAdornment: (
                    <InputAdornment position="start">
                      <Search size={15} />
                    </InputAdornment>
                  ),
                }}
                sx={{ '& .MuiInputBase-root': { fontSize: '13px' } }}
              />
            </Box>
            <List dense disablePadding sx={{ overflowY: 'auto', pb: 0.5 }}>
              {memberResults.members.map((member) => {
                const user = member.user
                const userId = member.user_id || ''
                const isCurrentUser = userId === currentUserId
                const selected = selectedUserIdSet.has(userId)
                return (
                  <ListItemButton
                    key={userId}
                    selected={selected}
                    onClick={() => toggleMember(userId)}
                    sx={{
                      minHeight: 44,
                      px: 1,
                      py: 0.5,
                      '&.Mui-selected': { backgroundColor: selectedSurface },
                    }}
                  >
                    <Checkbox
                      checked={selected}
                      size="small"
                      tabIndex={-1}
                      sx={{ p: 0.5, mr: 0.5 }}
                    />
                    <ListItemAvatar sx={{ minWidth: 34 }}>
                      <Avatar sx={{ width: 26, height: 26, fontSize: '0.7rem' }}>
                        {getUserInitials(user)}
                      </Avatar>
                    </ListItemAvatar>
                    <ListItemText
                      primary={`${memberLabel(user)}${isCurrentUser ? ' (you)' : ''}`}
                      secondary={user?.email}
                      primaryTypographyProps={{ fontSize: '13px', noWrap: true }}
                      secondaryTypographyProps={{ fontSize: '11px', noWrap: true }}
                    />
                  </ListItemButton>
                )
              })}
              {memberResults.total === 0 && (
                <Typography color="text.secondary" sx={{ px: 2, py: 2, textAlign: 'center', fontSize: '12px' }}>
                  No members found
                </Typography>
              )}
              {memberResults.total > memberResults.members.length && (
                <Typography color="text.secondary" sx={{ px: 2, py: 1, textAlign: 'center', fontSize: '11px' }}>
                  Search to find {memberResults.total - memberResults.members.length} more
                </Typography>
              )}
            </List>
          </Paper>
        </ClickAwayListener>
      </Popper>
    </>
  )
}

export default ProjectChatSidebarPeopleFilter
