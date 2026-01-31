import React, { FC, useState } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import InfoIcon from '@mui/icons-material/Info'

import Page from '../components/system/Page'
import MembersTable from '../components/orgs/MembersTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import UserSearchModal from '../components/orgs/UserSearchModal'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'

import { TypesOrganizationMembership, TypesOrganizationRole, TypesUser } from '../api/api'

// Organization People page that lists and manages members
const OrgPeople: FC = () => {
  // Get account context and router
  const account = useAccount()
  const router = useRouter()
  const snackbar = useSnackbar()
  
  // State for the delete modal
  const [deleteMember, setDeleteMember] = useState<TypesOrganizationMembership | undefined>()
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  
  // State for the user search modal
  const [searchModalOpen, setSearchModalOpen] = useState(false)

  const organization = account.organizationTools.organization

  // Handler for opening the search modal
  const handleAdd = () => {
    setSearchModalOpen(true)
  }

  // Handler for adding a member
  const addMemberToOrganisation = async (userId: string) => {
    if (!organization?.id) {
      snackbar.error('No active organization')
      return
    }
    
    try {
      const success = await account.organizationTools.addMemberToOrganization(
        organization.id,
        userId
      )
      
      if (success) {
        setSearchModalOpen(false)
      }
    } catch (error) {
      console.error('Error adding member:', error)
      snackbar.error('Failed to add member to organization')
    }
  }

  // Handler for initiating delete of a member
  const handleDelete = (member: TypesOrganizationMembership) => {
    // Check if this is the last owner and prevent deletion
    if (member.role === 'owner' && isLastOwner(member)) {
      snackbar.error('Cannot delete the last owner of the organization')
      return
    }
    
    setDeleteMember(member)
    setDeleteDialogOpen(true)
  }

  // Handler for confirming member deletion
  const handleConfirmDelete = async () => {
    if (deleteMember) {
      await account.organizationTools.deleteMemberFromOrganization(account.organizationTools.organization?.id!, deleteMember.user_id!)
      setDeleteDialogOpen(false)
    }
  }

  // Handler for changing a user's role
  const handleUserRoleChanged = async (member: TypesOrganizationMembership, newRole: TypesOrganizationRole) => {
    if (!organization?.id) {
      snackbar.error('No active organization')
      return
    }

    // Check if changing the last owner to a member
    if (member.role === 'owner' && newRole === 'member' && isLastOwner(member)) {
      snackbar.error('Cannot change the role of the last owner')
      return
    }

    try {
      // Implement the role change using the organization tools
      const success = await account.organizationTools.updateOrganizationMemberRole(
        organization.id,
        member.user_id!,
        newRole
      )
      
      if (success) {
        snackbar.success(`User role updated to ${newRole}`)
      }
    } catch (error) {
      console.error('Error updating member role:', error)
      snackbar.error('Failed to update member role')
    }
  }

  // Check if the given member is the last owner in the organization
  const isLastOwner = (member: TypesOrganizationMembership): boolean => {
    if (!organization?.memberships) return false
    
    const owners = organization.memberships.filter(m => m.role === 'owner')
    return owners.length === 1 && owners[0].user_id === member.user_id
  }

  // Use the isOrgAdmin property from the useOrganizations hook
  const isOrgOwner = account.isOrgAdmin

  if(!account.user) return null

  const existingMembers = account.organizationTools.organization?.memberships?.map(m => m.user).filter((user): user is TypesUser => user !== undefined) || []

  const deleteUserAny = deleteMember?.user as any

  return (
    <Page
      breadcrumbTitle={ organization ? `People` : 'Organization People' }
      breadcrumbParent={{ title: 'Organizations', routeName: 'orgs', useOrgRouter: false }}
      breadcrumbShowHome={ true }
      orgBreadcrumbs={ true }
      topbarContent={isOrgOwner ? (
        <Button
          variant="contained"
          color="secondary"
          startIcon={<AddIcon />}
          onClick={handleAdd}
        >
          Add Member
        </Button>
      ) : null}
    >
      
      <Container maxWidth="xl">
        <Box sx={{ mt: 3 }}>
          {account.organizationTools.organization?.memberships && (
            <MembersTable
              data={account.organizationTools.organization.memberships}
              onDelete={handleDelete}
              onUserRoleChanged={handleUserRoleChanged}
              loading={account.organizationTools.loading}
              showRoles={true}
              isOrgAdmin={isOrgOwner}
            />
          )}
        </Box>
      </Container>

      <DeleteConfirmWindow
        open={deleteDialogOpen}
        title={`member from organization "${deleteUserAny?.full_name || deleteMember?.user?.email || deleteMember?.user_id}"`}
        onCancel={() => setDeleteDialogOpen(false)}
        onSubmit={handleConfirmDelete}
      />
      
      <UserSearchModal
        open={searchModalOpen}
        onClose={() => setSearchModalOpen(false)}
        onAddMember={addMemberToOrganisation}
        title="Add Organization Member"
        existingMembers={existingMembers || []}
      />
    </Page>
  )
}

export default OrgPeople
