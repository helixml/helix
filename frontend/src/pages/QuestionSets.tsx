import React, { FC, useState, useEffect } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'

import Page from '../components/system/Page'
import QuestionSetsTable from '../components/questionSets/QuestionSetsTable'
import QuestionSetDialog from '../components/questionSets/QuestionSetDialog'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import { useListQuestionSets, useDeleteQuestionSet } from '../services/questionSetsService'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'

import { TypesQuestionSet } from '../api/api'

const QuestionSets: FC = () => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const [deletingQuestionSet, setDeletingQuestionSet] = useState<TypesQuestionSet | undefined>()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingQuestionSetId, setEditingQuestionSetId] = useState<string | undefined>()

  const orgId = account.organizationTools.organization?.id || ''

  const { data: questionSets, isLoading, refetch } = useListQuestionSets(
    orgId || undefined,
    {
      enabled: !!account.user,
    }
  )

  const deleteQuestionSetMutation = useDeleteQuestionSet()

  useEffect(() => {
    const urlParams = new URLSearchParams(window.location.search)
    const questionSetIdParam = urlParams.get('questionSetId')
    
    if (questionSetIdParam) {
      setEditingQuestionSetId(questionSetIdParam)
      setDialogOpen(true)
    }
  }, [])

  const handleEditQuestionSet = (questionSet: TypesQuestionSet) => {
    if (questionSet.id) {
      setEditingQuestionSetId(questionSet.id)
      setDialogOpen(true)
      const url = new URL(window.location.href)
      url.searchParams.set('questionSetId', questionSet.id)
      window.history.replaceState({}, '', url.toString())
    }
  }

  const handleCreateQuestionSet = () => {
    setEditingQuestionSetId(undefined)
    setDialogOpen(true)
    const url = new URL(window.location.href)
    url.searchParams.delete('questionSetId')
    window.history.replaceState({}, '', url.toString())
  }

  const handleCloseDialog = () => {
    setDialogOpen(false)
    setEditingQuestionSetId(undefined)
    const url = new URL(window.location.href)
    url.searchParams.delete('questionSetId')
    window.history.replaceState({}, '', url.toString())
    refetch()
  }

  const handleDeleteQuestionSet = (questionSet: TypesQuestionSet) => {
    setDeletingQuestionSet(questionSet)
  }

  const handleConfirmDelete = async () => {
    if (!deletingQuestionSet || !deletingQuestionSet.id) return
    
    try {
      await deleteQuestionSetMutation.mutateAsync(deletingQuestionSet.id)
      snackbar.success('Question set deleted successfully')
      setDeletingQuestionSet(undefined)
      refetch()
    } catch (error) {
      console.error('Error deleting question set:', error)
      snackbar.error('Failed to delete question set')
    }
  }

  const renderContent = () => {
    if (isLoading) {
      return (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
          <LoadingSpinner />
        </Box>
      )
    }

    if (!questionSets || questionSets.length === 0) {
      return (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
          <Box sx={{ textAlign: 'center' }}>
            No question sets found
          </Box>
        </Box>
      )
    }

    return (
      <QuestionSetsTable
        authenticated={ !!account.user }
        data={questionSets}
        onEdit={handleEditQuestionSet}
        onDelete={handleDeleteQuestionSet}
      />
    )
  }

  return (
    <Page
      breadcrumbTitle="Question Sets"
      orgBreadcrumbs={true}
      topbarContent={(
        <div>
          <Button
            variant="contained"
            color="secondary"
            endIcon={<AddIcon />}
            onClick={handleCreateQuestionSet}
          >
            New
          </Button>
        </div>
      )}
    >
      <Container maxWidth="xl" sx={{ mb: 4 }}>
        {renderContent()}
      </Container>

      <QuestionSetDialog
        open={dialogOpen}
        onClose={handleCloseDialog}
        questionSetId={editingQuestionSetId}
      />

      {deletingQuestionSet && (
        <DeleteConfirmWindow
          title="this question set"
          onCancel={() => setDeletingQuestionSet(undefined)}
          onSubmit={handleConfirmDelete}
        />
      )}
    </Page>
  )
}

export default QuestionSets

