import React, { FC, useState, useEffect } from 'react'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import LoadingSpinner from '../components/widgets/LoadingSpinner'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import { useRoute } from 'react-router5'

import Page from '../components/system/Page'
import QuestionSetsTable from '../components/questionSets/QuestionSetsTable'
import QuestionSetDialog from '../components/questionSets/QuestionSetDialog'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import { useListQuestionSets, useDeleteQuestionSet } from '../services/questionSetsService'
import useAccount from '../hooks/useAccount'
import useSnackbar from '../hooks/useSnackbar'
import useApps from '../hooks/useApps'

import { TypesQuestionSet } from '../api/api'

const QuestionSets: FC = () => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const apps = useApps()
  const { route } = useRoute()
  const [deletingQuestionSet, setDeletingQuestionSet] = useState<TypesQuestionSet | undefined>()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [editingQuestionSetId, setEditingQuestionSetId] = useState<string | undefined>()

  const isLoggedIn = !!account.user

  // Single helper to check login and show dialog if needed
  const requireLogin = React.useCallback((): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    return true
  }, [account])

  // Show login dialog on mount if not logged in (only after account is initialized)
  useEffect(() => {
    if (account.initialized && !isLoggedIn) {
      account.setShowLoginWindow(true)
    }
  }, [account.initialized, isLoggedIn])

  const orgId = account.organizationTools.organization?.id || ''

  const { data: questionSets, isLoading, refetch } = useListQuestionSets(
    orgId || undefined,
    {
      enabled: isLoggedIn,
    }
  )

  const deleteQuestionSetMutation = useDeleteQuestionSet()

  useEffect(() => {
    if(account.user) {
      apps.loadApps()
    }
  }, [
    account,apps.loadApps,
  ])

  useEffect(() => {
    const query = (route && (route as any).query) || {}
    const questionSetIdParam = query?.questionSetId as string | undefined
    if (questionSetIdParam) {
      setEditingQuestionSetId(questionSetIdParam)
      setDialogOpen(true)
    }
  }, [route])

  useEffect(() => {
    if (dialogOpen) return
    const urlParams = new URLSearchParams(window.location.search)
    const questionSetIdParam = urlParams.get('questionSetId')
    if (questionSetIdParam) {
      setEditingQuestionSetId(questionSetIdParam)
      setDialogOpen(true)
    }
  }, [])

  const handleEditQuestionSet = (questionSet: TypesQuestionSet) => {
    if (!requireLogin()) return
    if (questionSet.id) {
      setEditingQuestionSetId(questionSet.id)
      setDialogOpen(true)
      const url = new URL(window.location.href)
      url.searchParams.set('questionSetId', questionSet.id)
      window.history.replaceState({}, '', url.toString())
    }
  }

  const handleCreateQuestionSet = () => {
    if (!requireLogin()) return
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
    if (!requireLogin()) return
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
        apps={apps.apps}
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

