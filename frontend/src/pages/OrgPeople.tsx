import React, { FC, useState } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'

import Page from '../components/system/Page'
import MembersTable from '../components/orgs/MembersTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'

import { TypesOrganizationMembership } from '../api/api'

// Organization People page that lists and manages members
const OrgPeople: FC = () => {
  // Get account context and router
  const account = useAccount()
  const router = useRouter()
  const snackbar = useSnackbar()
  
  // State for the delete modal
  const [deleteMember, setDeleteMember] = useState<TypesOrganizationMembership | undefined>()
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

  // Handler for adding a new member (for now just logs to console)
  const handleAdd = () => {
    console.log('Add member clicked - will implement search functionality later')
  }

  // Handler for initiating delete of a member
  const handleDelete = (member: TypesOrganizationMembership) => {
    setDeleteMember(member)
    setDeleteDialogOpen(true)
  }

  // Handler for confirming member deletion
  const handleConfirmDelete = async () => {
    if (deleteMember) {
      await account.organizationTools.deleteMember(deleteMember.user_id!)
      setDeleteDialogOpen(false)
    }
  }

  // Check if the current user is an organization owner 
  // to determine if they can add/remove members
  const isOrgOwner = account.user && account.organizationTools.organization?.memberships?.some(
    m => m.user_id === account.user?.id && m.role === 'owner'
  )
 
  if(!account.user) return null

  return (
    <Page
      breadcrumbTitle={ account.organizationTools.organization?.display_name || 'Organization People' }
      breadcrumbShowHome={ false }
      topbarContent={isOrgOwner ? (
        <Button
          variant="contained"
          color="primary"
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
              loading={account.organizationTools.loading}
              currentUserID={account.user?.id}
            />
          )}
        </Box>
      </Container>

      <DeleteConfirmWindow
        open={deleteDialogOpen}
        title={`member "${deleteMember?.user?.fullName || deleteMember?.user?.email || deleteMember?.user_id}"`}
        onCancel={() => setDeleteDialogOpen(false)}
        onSubmit={handleConfirmDelete}
      />
    </Page>
  )
}

export default OrgPeople
