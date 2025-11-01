import React, { FC, useState } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import LoadingSpinner from '../components/widgets/LoadingSpinner'

import Page from '../components/system/Page'
import QuestionSetsTable from '../components/questionSets/QuestionSetsTable'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import { useListQuestionSets, useDeleteQuestionSet } from '../services/questionSetsService'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'

import { TypesQuestionSet } from '../api/api'

const QuestionSets: FC = () => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const [deletingQuestionSet, setDeletingQuestionSet] = useState<TypesQuestionSet | undefined>()

  const orgId = account.organizationTools.organization?.id || ''

  const { data: questionSets, isLoading, refetch } = useListQuestionSets(
    orgId || undefined,
    {
      enabled: !!account.user,
    }
  )

  const deleteQuestionSetMutation = useDeleteQuestionSet()

  const handleEditQuestionSet = (questionSet: TypesQuestionSet) => {
    // TODO: Implement edit dialog/navigation
    console.log('Edit question set:', questionSet)
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
    >
      <Container maxWidth="xl" sx={{ mb: 4 }}>
        {renderContent()}
      </Container>

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

