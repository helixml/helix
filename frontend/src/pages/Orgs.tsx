import React, { FC, useState, useCallback } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'

import Page from '../components/system/Page'
import OrgsTable from '../components/orgs/OrgsTable'
import EditOrgWindow from '../components/orgs/EditOrgWindow'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import useOrganizations from '../hooks/useOrganizations'
import useSnackbar from '../hooks/useSnackbar'

import {
  TypesOrganization,
} from '../api/api'

const Orgs: FC = () => {
  const {
    organizations,
    loading,
    createOrganization,
    updateOrganization,
    deleteOrganization,
  } = useOrganizations()

  const [editOrg, setEditOrg] = useState<TypesOrganization | undefined>()
  const [deleteOrg, setDeleteOrg] = useState<TypesOrganization | undefined>()
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

  const handleCreate = () => {
    setEditOrg(undefined)
    setEditDialogOpen(true)
  }

  const handleEdit = (org: TypesOrganization) => {
    setEditOrg(org)
    setEditDialogOpen(true)
  }

  const handleDelete = (org: TypesOrganization) => {
    setDeleteOrg(org)
    setDeleteDialogOpen(true)
  }

  const handleSubmit = async (org: TypesOrganization) => {
    if (org.id) {
      await updateOrganization(org.id, org)
    } else {
      await createOrganization(org)
    }
  }

  const handleConfirmDelete = async () => {
    if (deleteOrg) {
      await deleteOrganization(deleteOrg.id!)
      setDeleteDialogOpen(false)
    }
  }

  return (
    <Page
      breadcrumbTitle="Organizations"
      topbarContent={(
        <Button
          variant="contained"
          color="primary"
          startIcon={<AddIcon />}
          onClick={handleCreate}
        >
          Create Organization
        </Button>
      )}
    >
      <Container maxWidth="xl">
        <Box sx={{ mt: 3 }}>
          <OrgsTable
            data={organizations}
            onEdit={handleEdit}
            onDelete={handleDelete}
            loading={loading}
          />
        </Box>
      </Container>

      <EditOrgWindow
        open={editDialogOpen}
        org={editOrg}
        onClose={() => setEditDialogOpen(false)}
        onSubmit={handleSubmit}
      />

      <DeleteConfirmWindow
        open={deleteDialogOpen}
        title={`organization "${deleteOrg?.display_name || deleteOrg?.name}"`}
        onCancel={() => setDeleteDialogOpen(false)}
        onSubmit={handleConfirmDelete}
      />
    </Page>
  )
}

export default Orgs
