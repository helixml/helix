import { FC, useState } from 'react'
import Avatar from '@mui/material/Avatar'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Typography from '@mui/material/Typography'
import { Check, ChevronDown, Users } from 'lucide-react'

import useLightTheme from '../../hooks/useLightTheme'
import { getUserInitials } from '../../utils/user'
import { getSidebarColors } from '../../styles/themeTokens'
import { TYPOGRAPHY } from '../../styles/typography'
import { ALL_USERS_FILTER } from './ProjectChatSidebar.logic'
import type { SidebarMember } from './ProjectChatSidebar.logic'
import { sidebarMemberLabel } from './ProjectChatPersonGroup'

type ProjectChatSidebarUserFilterProps = {
  members: SidebarMember[]
  selectedUserId: string
  onChange: (userId: string) => void
}

// Whose chats and tasks the project view lists: everyone's, or one member's.
const ProjectChatSidebarUserFilter: FC<ProjectChatSidebarUserFilterProps> = ({
  members,
  selectedUserId,
  onChange,
}) => {
  const lightTheme = useLightTheme()
  const sidebarColors = getSidebarColors(lightTheme.isLight)
  const [anchorEl, setAnchorEl] = useState<HTMLElement | null>(null)
  const selectedMember = members.find((member) => member.userId === selectedUserId)
  const selectedLabel = selectedMember ? sidebarMemberLabel(selectedMember) : 'Everyone'

  const selectUser = (userId: string) => {
    setAnchorEl(null)
    onChange(userId)
  }

  return (
    <>
      <Button
        variant="text"
        size="small"
        aria-label="Filter chats by user"
        aria-haspopup="menu"
        aria-expanded={!!anchorEl || undefined}
        onClick={(event) => setAnchorEl(event.currentTarget)}
        startIcon={selectedMember ? (
          <Avatar sx={{ width: 16, height: 16, fontSize: '0.5rem' }}>
            {getUserInitials(selectedMember.user)}
          </Avatar>
        ) : (
          <Users size={13} strokeWidth={1.7} style={{ flexShrink: 0 }} />
        )}
        endIcon={<ChevronDown size={13} strokeWidth={1.7} />}
        sx={{
          flex: 1,
          minWidth: 0,
          height: 30,
          px: 0.75,
          justifyContent: 'space-between',
          color: sidebarColors.mutedForeground,
          fontFamily: 'inherit',
          fontSize: TYPOGRAPHY.sidebar.metadataFontSize,
          fontWeight: 500,
          lineHeight: 1,
          textTransform: 'none',
          '& .MuiButton-startIcon': { mr: 0.5, flexShrink: 0 },
          '& .MuiButton-endIcon': { ml: 0.5, flexShrink: 0 },
          '&:hover': {
            color: sidebarColors.foreground,
            backgroundColor: sidebarColors.rowHover,
          },
        }}
      >
        <Box
          component="span"
          sx={{ minWidth: 0, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}
        >
          {selectedLabel}
        </Box>
      </Button>
      <Menu
        anchorEl={anchorEl}
        open={!!anchorEl}
        onClose={() => setAnchorEl(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'left' }}
        transformOrigin={{ vertical: 'top', horizontal: 'left' }}
        slotProps={{
          paper: {
            sx: {
              mt: 0.5,
              minWidth: 220,
              maxWidth: 300,
              maxHeight: 360,
              backgroundImage: 'none',
            },
          },
        }}
      >
        <MenuItem
          selected={selectedUserId === ALL_USERS_FILTER}
          onClick={() => selectUser(ALL_USERS_FILTER)}
          sx={{ gap: 1, fontSize: TYPOGRAPHY.sidebar.controlFontSize }}
        >
          <Box sx={{ width: 18, display: 'inline-flex', justifyContent: 'center' }}>
            {selectedUserId === ALL_USERS_FILTER ? <Check size={14} /> : <Users size={14} />}
          </Box>
          Everyone
        </MenuItem>
        {members.map((member) => (
          <MenuItem
            key={member.userId}
            selected={selectedUserId === member.userId}
            onClick={() => selectUser(member.userId)}
            sx={{ gap: 1, fontSize: TYPOGRAPHY.sidebar.controlFontSize }}
          >
            <Box sx={{ width: 18, display: 'inline-flex', justifyContent: 'center' }}>
              {selectedUserId === member.userId ? <Check size={14} /> : (
                <Avatar sx={{ width: 16, height: 16, fontSize: '0.5rem' }}>
                  {getUserInitials(member.user)}
                </Avatar>
              )}
            </Box>
            <Typography
              component="span"
              sx={{
                overflow: 'hidden',
                textOverflow: 'ellipsis',
                fontSize: TYPOGRAPHY.sidebar.controlFontSize,
              }}
            >
              {sidebarMemberLabel(member)}
            </Typography>
          </MenuItem>
        ))}
      </Menu>
    </>
  )
}

export default ProjectChatSidebarUserFilter
