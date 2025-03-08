import React, { FC, useMemo, useCallback } from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import PersonIcon from '@mui/icons-material/Person'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import useTheme from '@mui/material/styles/useTheme'

import SimpleTable from '../widgets/SimpleTable'
import ClickLink from '../widgets/ClickLink'

import { TypesOrganizationMembership, TypesTeamMembership } from '../../api/api'

// Type for membership that can be either organization or team membership
type Membership = TypesOrganizationMembership | TypesTeamMembership

// Props for the MembersTable component
interface MembersTableProps {
  data: Membership[]
  onDelete: (member: Membership) => void
  loading?: boolean
  currentUserID?: string
}

// Display a table of organization or team members with their roles and actions
const MembersTable: FC<MembersTableProps> = ({ 
  data, 
  onDelete, 
  loading = false,
  currentUserID
}) => {
  const theme = useTheme()

  // Get the role display name and color
  const getRoleDisplay = (role: string | undefined) => {
    switch (role) {
      case 'owner':
        return { label: 'Owner', color: 'error' }
      case 'admin':
        return { label: 'Admin', color: 'warning' }
      case 'member':
      default:
        return { label: 'Member', color: 'primary' }
    }
  }

  // Check if the current user is viewing themselves (can't delete self)
  const isSelf = (memberID: string | undefined) => {
    return memberID === currentUserID
  }
  
  // Transform member data for the table display
  const tableData = useMemo(() => {
    return data.map(member => {
      // For organization memberships, use the role field
      // For team memberships, default to 'member' since they don't have roles
      const isOrgMembership = 'role' in member
      const roleDisplay = getRoleDisplay(isOrgMembership ? member.role : 'member')

      // TODO: this is because the props are coming in as uppercase
      // let's fix this in the api so the props come through lowercase
      // e.g. we are getting member.user.FullName, but we should get member.user.fullName
      const anyUser = member.user as any
      return {
        id: member.user_id,
        _data: member, // Store original data for actions
        user: (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <PersonIcon color="action" />
            <span>{anyUser.FullName || 'Unnamed User'}</span>
          </Box>
        ),
        email: anyUser.Email || '',
        role: (
          <Chip 
            label={roleDisplay.label} 
            color={roleDisplay.color as any} 
            size="small" 
            variant="outlined" 
          />
        )
      }
    })
  }, [data])

  // Generate action buttons for each member row
  const getActions = useCallback((row: any) => {
    const isCurrentUser = isSelf(row._data.user_id)
    
    return (
      <Box sx={{
        width: '100%',
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'flex-end',
        justifyContent: 'flex-end',
        pl: 2,
        pr: 2,
      }}>
        {!isCurrentUser && (
          <ClickLink
            onClick={() => onDelete(row._data)}
          >
            <Tooltip title="Delete">
              <DeleteIcon color="action" />
            </Tooltip>
          </ClickLink>
        )}
      </Box>
    )
  }, [onDelete])

  return (
    <SimpleTable
      fields={[{
        name: 'user',
        title: 'User',
      }, {
        name: 'email',
        title: 'Email',
      }, {
        name: 'role',
        title: 'Role',
      }]}
      data={tableData}
      getActions={getActions}
      loading={loading}
    />
  )
}

export default MembersTable 