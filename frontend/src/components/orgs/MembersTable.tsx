import React, { FC, useMemo, useCallback, useState } from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import EditIcon from '@mui/icons-material/Edit'
import PersonIcon from '@mui/icons-material/Person'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import useTheme from '@mui/material/styles/useTheme'

import SimpleTable from '../widgets/SimpleTable'
import ClickLink from '../widgets/ClickLink'
import EditRoleModal from './EditRoleModal'

import { TypesOrganizationMembership, TypesTeamMembership, TypesOrganizationRole } from '../../api/api'

// Type for membership that can be either organization or team membership
type Membership = TypesOrganizationMembership | TypesTeamMembership

// Props for the MembersTable component
interface MembersTableProps {
  data: Membership[]
  onDelete: (member: Membership) => void
  onUserRoleChanged?: (member: TypesOrganizationMembership, newRole: TypesOrganizationRole) => Promise<void>
  loading?: boolean
  showRoles?: boolean
  isOrgAdmin?: boolean
}

// Display a table of organization or team members with their roles and actions
const MembersTable: FC<MembersTableProps> = ({ 
  data, 
  onDelete, 
  onUserRoleChanged,
  loading = false,
  showRoles = true,
  isOrgAdmin = false
}) => {
  const theme = useTheme()
  const [editMember, setEditMember] = useState<TypesOrganizationMembership | undefined>(undefined)
  const [editModalOpen, setEditModalOpen] = useState(false)

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

  // Check if there's only one owner in the organization
  const hasOnlyOneOwner = useMemo(() => {
    const owners = data.filter(member => 'role' in member && member.role === 'owner')
    return owners.length === 1
  }, [data])

  // Handle opening the edit modal
  const handleEdit = useCallback((member: Membership) => {
    if ('role' in member) {
      setEditMember(member as TypesOrganizationMembership)
      setEditModalOpen(true)
    }
  }, [])

  // Handle saving the role change
  const handleSaveRole = async (member: TypesOrganizationMembership, newRole: TypesOrganizationRole) => {
    if (onUserRoleChanged) {
      await onUserRoleChanged(member, newRole)
    }
  }

  // Generate action buttons for each member row
  const getActions = useCallback((row: any) => {
    const isOrgMembership = 'role' in row._data
    
    return (
      <Box sx={{
        width: '100%',
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'flex-end',
        justifyContent: 'flex-end',
        pl: 2,
        pr: 2,
        gap: 2
      }}>
        {showRoles && isOrgMembership && isOrgAdmin && (
          <ClickLink
            onClick={() => handleEdit(row._data)}
          >
            <Tooltip title="Edit Role">
              <EditIcon color="action" />
            </Tooltip>
          </ClickLink>
        )}
        <ClickLink
          onClick={() => onDelete(row._data)}
        >
          <Tooltip title="Delete">
            <DeleteIcon color="action" />
          </Tooltip>
        </ClickLink>
      </Box>
    )
  }, [onDelete, showRoles, isOrgAdmin, handleEdit])

  return (
    <>
      <SimpleTable
        fields={[{
          name: 'user',
          title: 'User',
        }, {
          name: 'email',
          title: 'Email',
        }, 
        ...(showRoles ? [{
          name: 'role',
          title: 'Role',
        }] : [])]}
        data={tableData}
        getActions={getActions}
        loading={loading}
      />

      {editMember && (
        <EditRoleModal
          open={editModalOpen}
          member={editMember}
          onClose={() => setEditModalOpen(false)}
          onSave={handleSaveRole}
          isLastOwner={hasOnlyOneOwner && editMember.role === 'owner'}
        />
      )}
    </>
  )
}

export default MembersTable 