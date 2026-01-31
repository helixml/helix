import React, { FC, useState, useEffect } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'

import Page from '../components/system/Page'
import MembersTable from '../components/orgs/MembersTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'
import UserSearchModal from '../components/orgs/UserSearchModal'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'

import { TypesOrganizationMembership, TypesTeamMembership, TypesTeam, TypesUser } from '../api/api'

// Team People page that lists and manages team members
const TeamPeople: FC = () => {
  // Get account context and router
  const account = useAccount()
  const router = useRouter()
  const snackbar = useSnackbar()
  
  // State for the delete modal
  const [deleteMember, setDeleteMember] = useState<TypesOrganizationMembership | undefined>()
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)
  
  // State for the user search modal
  const [searchModalOpen, setSearchModalOpen] = useState(false)
  
  // State for the current team
  const [currentTeam, setCurrentTeam] = useState<TypesTeam | undefined>()

  // Extract org_id and team_id parameters from router
  const orgId = router.params.org_id
  const teamId = router.params.team_id

  const organization = account.organizationTools.organization
  
  // Find the current team in the organization's teams array
  useEffect(() => {
    if (account.organizationTools.organization?.teams && teamId) {
      const team = account.organizationTools.organization.teams.find(t => t.id === teamId)
      setCurrentTeam(team)
    }
  }, [account.organizationTools.organization, teamId])

  // Handler for opening the search modal
  const handleAdd = () => {
    setSearchModalOpen(true)
  }

  // Handler for adding a team member
  const addTeamMember = async (userId: string) => {
    if (!organization?.id || !teamId) {
      snackbar.error('Organization or team ID not found')
      return
    }
    
    try {
      const success = await account.organizationTools.addTeamMember(
        organization?.id,
        teamId,
        userId
      )
      
      if (success) {
        setSearchModalOpen(false)
      }
    } catch (error) {
      console.error('Error adding team member:', error)
      snackbar.error('Failed to add member to team')
    }
  }

  // Handler for initiating delete of a member
  const handleDelete = (member: TypesOrganizationMembership | TypesTeamMembership) => {
    setDeleteMember(member as TypesOrganizationMembership)
    setDeleteDialogOpen(true)
  }

  // Handler for confirming member deletion
  const handleConfirmDelete = async () => {
    if (deleteMember && deleteMember.user_id) {
      if (!organization?.id || !teamId) {
        snackbar.error('Organization or team ID not found')
        return
      }
      
      try {
        await account.organizationTools.removeTeamMember(
          organization.id,
          teamId,
          deleteMember.user_id,
        )
        setDeleteDialogOpen(false)
      } catch (error) {
        console.error('Error removing team member:', error)
      }
    }
  }

  // Use the isOrgAdmin property from the useOrganizations hook
  const isOrgOwner = account.isOrgAdmin

  const existingMembers = currentTeam?.memberships?.map(m => m.user).filter((user): user is TypesUser => user !== undefined) || []

  if(!account.user) return null
  if(!organization) return null
  if(!currentTeam) return null

  return (
    <Page
      // breadcrumbTitle={ currentTeam ? `${organization.display_name} : Teams : ${currentTeam.name} : Members` : 'Team Members' }
      // breadcrumbTitle={ currentTeam.name }
      orgBreadcrumbs={true}
      breadcrumbs={[
        {
          title: 'Teams',
          routeName: `org_teams`,
          params: {
            org_id: organization.id,
          },
        },
        {
          title: currentTeam.name || '',
          routeName: `orgs/${organization.id}/teams`,
        },
      ]}
      topbarContent={isOrgOwner ? (
        <Button
          variant="contained"
          color="secondary"
          startIcon={<AddIcon />}
          onClick={handleAdd}
        >
          Add Team Member
        </Button>
      ) : null}
    >
      <Container maxWidth="xl">
        <Box sx={{ mt: 3 }}>
          {currentTeam?.memberships ? (
            <MembersTable
              data={currentTeam.memberships}
              onDelete={handleDelete}
              loading={account.organizationTools.loading}
              showRoles={false}
              isOrgAdmin={isOrgOwner}
            />
          ) : (
            <Typography variant="body1" color="text.secondary" align="center">
              {account.organizationTools.loading ? 'Loading team members...' : 'No team members found'}
            </Typography>
          )}
        </Box>
      </Container>

      <DeleteConfirmWindow
        open={deleteDialogOpen}
        title={`team member "${deleteMember?.user?.full_name || deleteMember?.user?.email || deleteMember?.user_id}"`}
        onCancel={() => setDeleteDialogOpen(false)}
        onSubmit={handleConfirmDelete}
      />
      
      <UserSearchModal
        open={searchModalOpen}
        onClose={() => setSearchModalOpen(false)}
        onAddMember={addTeamMember}
        title="Add Team Member"
        messagePrefix="Only showing users in organization."
        organizationMembersOnly={true}
        existingMembers={existingMembers || []}
      />
    </Page>
  )
}

export default TeamPeople 