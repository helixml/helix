import React, { FC, useState } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'

import Page from '../components/system/Page'
import TeamsTable from '../components/orgs/TeamsTable'
import EditTeamWindow from '../components/orgs/EditTeamWindow'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useAccount from '../hooks/useAccount'
import useRouter from '../hooks/useRouter'
import useSnackbar from '../hooks/useSnackbar'

import { TypesTeam } from '../api/api'

// Organization Teams page that lists and manages teams
const OrgTeams: FC = () => {
  // Get account context and router
  const account = useAccount()
  const router = useRouter()
  const snackbar = useSnackbar()

  console.log('--------------------------------------------')
  console.dir(account.organizationTools.organization)
  
  // State for the edit/delete modals
  const [editTeam, setEditTeam] = useState<TypesTeam | undefined>()
  const [deleteTeam, setDeleteTeam] = useState<TypesTeam | undefined>()
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

  // Handler for creating a new team
  const handleCreate = () => {
    setEditTeam(undefined)
    setEditDialogOpen(true)
  }

  const handleView = (team: TypesTeam) => {
    router.navigate('team_people', {
      org_id: account.organizationTools.organization?.name,
      team_id: team.id,
    })
  }

  // Handler for editing an existing team
  const handleEdit = (team: TypesTeam) => {
    setEditTeam(team)
    setEditDialogOpen(true)
  }

  // Handler for initiating delete of a team
  const handleDelete = (team: TypesTeam) => {
    setDeleteTeam(team)
    setDeleteDialogOpen(true)
  }

  // Handler for submitting team creation/edit
  const handleSubmit = async (team: TypesTeam) => {
    const org = account.organizationTools.organization
    if(!org || !org.id) return
    if(!account.user || !account.user.id) return
    if (team.id) {
      await account.organizationTools.updateTeam(org.id, team.id, team)
    } else {
      await account.organizationTools.createTeamWithCreator(org.id, account.user.id, team)
    }
    setEditDialogOpen(false)
  }

  // Handler for confirming team deletion
  const handleConfirmDelete = async () => {
    const org = account.organizationTools.organization
    if(!org || !org.id) return
    if (deleteTeam) {
      await account.organizationTools.deleteTeam(org.id, deleteTeam.id!)
      setDeleteDialogOpen(false)
    }
  }

  // Check if the current user is an organization owner 
  // to determine if they can add/edit/delete teams
  const isOrgOwner = account.user && account.organizationTools.organization?.memberships?.some(
    m => m.user_id === account.user?.id && m.role === 'owner'
  )
 
  if(!account.user) return null
  if(!account.organizationTools.organization) return null

  return (
    <Page
      breadcrumbTitle={ account.organizationTools.organization?.display_name || 'Organization Teams' }
      breadcrumbShowHome={ false }
      topbarContent={isOrgOwner ? (
        <Button
          variant="contained"
          color="primary"
          startIcon={<AddIcon />}
          onClick={handleCreate}
        >
          Add Team
        </Button>
      ) : null}
    >
      <Container maxWidth="xl">
        <Box sx={{ mt: 3 }}>
          {account.organizationTools.organization?.teams && (
            <TeamsTable
              data={account.organizationTools.organization.teams}
              onView={handleView}
              onEdit={handleEdit}
              onDelete={handleDelete}
              loading={account.organizationTools.loading}
            />
          )}
        </Box>
      </Container>

      <EditTeamWindow
        open={editDialogOpen}
        team={editTeam}
        onClose={() => setEditDialogOpen(false)}
        onSubmit={handleSubmit}
      />

      <DeleteConfirmWindow
        open={deleteDialogOpen}
        title={`team "${deleteTeam?.name}"`}
        onCancel={() => setDeleteDialogOpen(false)}
        onSubmit={handleConfirmDelete}
      />
    </Page>
  )
}

export default OrgTeams
