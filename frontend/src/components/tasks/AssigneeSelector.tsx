import React, { FC, useState, useMemo } from 'react'
import {
  Popover,
  Box,
  Typography,
  Avatar,
  List,
  ListItemButton,
  ListItemAvatar,
  ListItemText,
  TextField,
  InputAdornment,
  Divider,
  CircularProgress,
} from '@mui/material'
import { Search, UserX, User } from 'lucide-react'
import { TypesOrganizationMembership, TypesUser } from '../../api/api'

interface AssigneeSelectorProps {
  /** Currently assigned user ID (empty string or undefined for unassigned) */
  assigneeId?: string
  /** List of organization members to choose from */
  members: TypesOrganizationMembership[]
  /** Callback when assignee is changed */
  onAssigneeChange: (userId: string | null) => void
  /** Whether the mutation is in progress */
  isLoading?: boolean
  /** Anchor element for the popover */
  anchorEl: HTMLElement | null
  /** Callback to close the popover */
  onClose: () => void
}

/**
 * AssigneeSelector - A popover component for selecting task assignees
 * 
 * Displays a searchable list of organization members with an option to unassign.
 */
const AssigneeSelector: FC<AssigneeSelectorProps> = ({
  assigneeId,
  members,
  onAssigneeChange,
  isLoading = false,
  anchorEl,
  onClose,
}) => {
  const [searchQuery, setSearchQuery] = useState('')

  // Filter members based on search query
  const filteredMembers = useMemo(() => {
    if (!searchQuery.trim()) return members
    
    const query = searchQuery.toLowerCase()
    return members.filter((member) => {
      const user = member.user as TypesUser | undefined
      if (!user) return false
      
      const name = user.full_name || user.username || ''
      const email = user.email || ''
      
      return (
        name.toLowerCase().includes(query) ||
        email.toLowerCase().includes(query)
      )
    })
  }, [members, searchQuery])

  // Get display name for a user
  const getDisplayName = (user: TypesUser | undefined): string => {
    if (!user) return 'Unknown User'
    return user.full_name || user.username || user.email || 'Unknown User'
  }

  // Get initials for avatar
  const getInitials = (user: TypesUser | undefined): string => {
    if (!user) return '?'
    const name = user.full_name || user.username || user.email || ''
    const parts = name.split(' ')
    if (parts.length >= 2) {
      return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
    }
    return name.slice(0, 2).toUpperCase()
  }

  const handleSelect = (userId: string | null) => {
    onAssigneeChange(userId)
    onClose()
  }

  const open = Boolean(anchorEl)

  return (
    <Popover
      open={open}
      anchorEl={anchorEl}
      onClose={onClose}
      anchorOrigin={{
        vertical: 'bottom',
        horizontal: 'left',
      }}
      transformOrigin={{
        vertical: 'top',
        horizontal: 'left',
      }}
      slotProps={{
        paper: {
          sx: {
            width: 280,
            maxHeight: 400,
            overflow: 'hidden',
            display: 'flex',
            flexDirection: 'column',
          },
        },
      }}
    >
      {/* Search input */}
      <Box sx={{ p: 1.5, borderBottom: 1, borderColor: 'divider' }}>
        <TextField
          size="small"
          fullWidth
          placeholder="Search members..."
          value={searchQuery}
          onChange={(e) => setSearchQuery(e.target.value)}
          autoFocus
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <Search size={16} />
              </InputAdornment>
            ),
          }}
          sx={{
            '& .MuiOutlinedInput-root': {
              fontSize: '0.875rem',
            },
          }}
        />
      </Box>

      {/* Loading state */}
      {isLoading && (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 2 }}>
          <CircularProgress size={24} />
        </Box>
      )}

      {/* Member list */}
      {!isLoading && (
        <List
          sx={{
            flex: 1,
            overflow: 'auto',
            py: 0.5,
          }}
        >
          {/* Unassigned option */}
          <ListItemButton
            selected={!assigneeId}
            onClick={() => handleSelect(null)}
            sx={{ py: 1 }}
          >
            <ListItemAvatar sx={{ minWidth: 40 }}>
              <Avatar
                sx={{
                  width: 28,
                  height: 28,
                  bgcolor: 'action.selected',
                }}
              >
                <UserX size={16} />
              </Avatar>
            </ListItemAvatar>
            <ListItemText
              primary="Unassigned"
              primaryTypographyProps={{
                fontSize: '0.875rem',
                color: 'text.secondary',
              }}
            />
          </ListItemButton>

          {members.length > 0 && <Divider sx={{ my: 0.5 }} />}

          {/* Filtered members */}
          {filteredMembers.map((member) => {
            const user = member.user as TypesUser | undefined
            const userId = member.user_id || ''
            const isSelected = assigneeId === userId

            return (
              <ListItemButton
                key={userId}
                selected={isSelected}
                onClick={() => handleSelect(userId)}
                sx={{ py: 1 }}
              >
                <ListItemAvatar sx={{ minWidth: 40 }}>
                  <Avatar
                    src={user?.avatar_url}
                    sx={{
                      width: 28,
                      height: 28,
                      fontSize: '0.75rem',
                    }}
                  >
                    {getInitials(user)}
                  </Avatar>
                </ListItemAvatar>
                <ListItemText
                  primary={getDisplayName(user)}
                  secondary={user?.email}
                  primaryTypographyProps={{
                    fontSize: '0.875rem',
                    fontWeight: isSelected ? 600 : 400,
                  }}
                  secondaryTypographyProps={{
                    fontSize: '0.75rem',
                    noWrap: true,
                  }}
                />
              </ListItemButton>
            )
          })}

          {/* Empty state */}
          {filteredMembers.length === 0 && members.length > 0 && (
            <Box sx={{ p: 2, textAlign: 'center' }}>
              <Typography variant="body2" color="text.secondary">
                No members found
              </Typography>
            </Box>
          )}

          {/* No members state */}
          {members.length === 0 && (
            <Box sx={{ p: 2, textAlign: 'center' }}>
              <User size={24} style={{ opacity: 0.5, marginBottom: 8 }} />
              <Typography variant="body2" color="text.secondary">
                No team members available
              </Typography>
            </Box>
          )}
        </List>
      )}
    </Popover>
  )
}

export default AssigneeSelector