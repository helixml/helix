import React, { FC, useMemo, useCallback, useState } from 'react'
import DeleteIcon from '@mui/icons-material/Delete'
import EditIcon from '@mui/icons-material/Edit'
import PersonIcon from '@mui/icons-material/Person'
import MoreVertIcon from '@mui/icons-material/MoreVert'
import Box from '@mui/material/Box'
import Tooltip from '@mui/material/Tooltip'
import Chip from '@mui/material/Chip'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
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
  
  // State for the action menu
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null)
  const [selectedMember, setSelectedMember] = useState<Membership | null>(null)

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
    
      return {
        id: member.user_id,
        _data: member, // Store original data for actions
        user: (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            <PersonIcon color="action" />
            <span>{member.user?.full_name || 'Unnamed User'}</span>
          </Box>
        ),
        email: member.user?.email || '',
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

  // Handle opening the action menu
  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>, member: Membership) => {
    setAnchorEl(event.currentTarget)
    setSelectedMember(member)
  }

  // Handle closing the action menu
  const handleMenuClose = () => {
    setAnchorEl(null)
    setSelectedMember(null)
  }

  // Handle edit action from menu
  const handleEditFromMenu = () => {
    if (selectedMember) {
      handleEdit(selectedMember)
    }
    handleMenuClose()
  }

  // Handle delete action from menu
  const handleDeleteFromMenu = () => {
    if (selectedMember) {
      onDelete(selectedMember)
    }
    handleMenuClose()
  }

  // Generate action menu for each member row
  const getActions = useCallback((row: any) => {
    if (!isOrgAdmin) return <></>

    return (
      <Box sx={{
        width: '100%',
        display: 'flex',
        flexDirection: 'row',
        alignItems: 'center',
        justifyContent: 'flex-end',
        pl: 2,
        pr: 2
      }}>
        <Tooltip title="Actions">
          <IconButton
            size="small"
            onClick={(event) => handleMenuOpen(event, row._data)}
          >
            <MoreVertIcon color="action" />
          </IconButton>
        </Tooltip>
      </Box>
    )
  }, [isOrgAdmin])

  return (
    <>
      <SimpleTable
        authenticated={true}
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
        getActions={ isOrgAdmin ? getActions : undefined }
        loading={loading}
      />

      {/* Action Menu */}
      <Menu
        anchorEl={anchorEl}
        open={Boolean(anchorEl)}
        onClose={handleMenuClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
      >
        {showRoles && selectedMember && 'role' in selectedMember && (
          <MenuItem onClick={handleEditFromMenu}>
            <ListItemIcon>
              <EditIcon fontSize="small" />
            </ListItemIcon>
            <ListItemText>Edit Role</ListItemText>
          </MenuItem>
        )}
        <MenuItem onClick={handleDeleteFromMenu}>
          <ListItemIcon>
            <DeleteIcon fontSize="small" />
          </ListItemIcon>
          <ListItemText>Delete</ListItemText>
        </MenuItem>
      </Menu>

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