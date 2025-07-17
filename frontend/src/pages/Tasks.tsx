import React, { FC, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import LoadingSpinner from '../components/widgets/LoadingSpinner'

import Page from '../components/system/Page'
import TaskDialog from '../components/tasks/TaskDialog'
import TasksTable from '../components/tasks/TasksTable'
import EmptyTasksState from '../components/tasks/EmptyTasksState'
import DeleteConfirmWindow from '../components/widgets/DeleteConfirmWindow'

import { useListUserCronTriggers, useDeleteAppTrigger } from '../services/appService'
import useAccount from '../hooks/useAccount'
import useApps from '../hooks/useApps'
import useSnackbar from '../hooks/useSnackbar'
import useApi from '../hooks/useApi'

import { TypesTriggerConfiguration } from '../api/api'

const Tasks: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const snackbar = useSnackbar()
  const api = useApi()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [selectedTask, setSelectedTask] = useState<TypesTriggerConfiguration | undefined>()
  const [deletingTask, setDeletingTask] = useState<TypesTriggerConfiguration | undefined>()

  const { data: triggers, isLoading, refetch } = useListUserCronTriggers(
    account.organizationTools.organization?.id || ''
  )

  const [deleteTriggerId, setDeleteTriggerId] = useState<string>('')
  const deleteTriggerMutation = useDeleteAppTrigger(deleteTriggerId, account.organizationTools.organization?.id || '')

  useEffect(() => {
    apps.loadApps()
  }, [
    apps.loadApps,
  ])

  // Check for ?task=new or ?task=<id> query parameter on component mount
  useEffect(() => {
    const urlParams = new URLSearchParams(window.location.search)
    const taskParam = urlParams.get('task')
    
    if (taskParam === 'new') {
      setSelectedTask(undefined)
      setDialogOpen(true)
      
      // Clean up the URL by removing the query parameter
      const newUrl = new URL(window.location.href)
      newUrl.searchParams.delete('task')
      window.history.replaceState({}, '', newUrl.toString())
    } else if (taskParam && taskParam !== 'new') {
      // Find the task by ID
      const task = triggers?.data?.find(t => t.id === taskParam)
      if (task) {
        setSelectedTask(task)
        setDialogOpen(true)
        
        // Clean up the URL by removing the query parameter
        const newUrl = new URL(window.location.href)
        newUrl.searchParams.delete('task')
        window.history.replaceState({}, '', newUrl.toString())
      }
    }
  }, [triggers?.data])

  const handleCreateTask = () => {
    setSelectedTask(undefined)
    setDialogOpen(true)
  }

  const handleEditTask = (task: TypesTriggerConfiguration) => {
    setSelectedTask(task)
    setDialogOpen(true)
  }

  const handleCloseDialog = () => {
    setDialogOpen(false)
    setSelectedTask(undefined)
  }

  const handleDeleteTask = (task: TypesTriggerConfiguration) => {
    setDeletingTask(task)
  }

  const handleConfirmDelete = async () => {
    if (!deletingTask || !deletingTask.id) return
    
    try {
      setDeleteTriggerId(deletingTask.id)
      await deleteTriggerMutation.mutateAsync()
      snackbar.success('Task deleted successfully')
      setDeletingTask(undefined)
      setDeleteTriggerId('')
    } catch (error) {
      console.error('Error deleting task:', error)
      snackbar.error('Failed to delete task')
      setDeleteTriggerId('')
    }
  }

  const handleToggleStatus = async (task: TypesTriggerConfiguration) => {
    if (!task.id) {
      snackbar.error('Task ID is missing')
      return
    }

    try {
      const apiClient = api.getApiClient()
      
      // Toggle the enabled status
      const updatedTask = {
        ...task,
        enabled: !task.enabled
      }

      await apiClient.v1TriggersUpdate(task.id, updatedTask)
      snackbar.success(task.enabled ? 'Task paused' : 'Task enabled')
      
      // Refetch the triggers list to update the UI
      refetch()
    } catch (error) {
      console.error('Error toggling task status:', error)
      snackbar.error('Failed to update task status')
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

    if (!triggers?.data || triggers.data.length === 0) {
      return <EmptyTasksState onCreateTask={handleCreateTask} />
    }

    return (
      <TasksTable
        data={triggers.data}
        apps={apps.apps}
        onEdit={handleEditTask}
        onDelete={handleDeleteTask}
        onToggleStatus={handleToggleStatus}
      />
    )
  }

  return (
    <Page
      breadcrumbTitle="Tasks"
      orgBreadcrumbs={true}
      topbarContent={(
        <div>
          <Button
            id="new-task-button"
            variant="contained"
            color="secondary"
            endIcon={<AddIcon />}
            onClick={handleCreateTask}
          >
            Add new
          </Button>
        </div>
      )}
    >
      <Container maxWidth="xl" sx={{ mb: 4 }}>
        {renderContent()}
      </Container>

      {/* Task Dialog */}
      <TaskDialog
        open={dialogOpen}
        onClose={handleCloseDialog}
        task={selectedTask}
        apps={apps.apps}
      />

      {/* Delete Confirmation */}
      {deletingTask && (
        <DeleteConfirmWindow
          title="this task"
          onCancel={() => setDeletingTask(undefined)}
          onSubmit={handleConfirmDelete}
        />
      )}
    </Page>
  )
}

export default Tasks
