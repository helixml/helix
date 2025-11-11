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
import useRouter from '../hooks/useRouter'

import { TypesTriggerConfiguration } from '../api/api'

interface TaskData {
  name: string
  schedule: string
  input: string
}

const Tasks: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const snackbar = useSnackbar()
  const api = useApi()
  const router = useRouter()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [selectedTask, setSelectedTask] = useState<TypesTriggerConfiguration | undefined>()
  const [deletingTask, setDeletingTask] = useState<TypesTriggerConfiguration | undefined>()
  const [prepopulatedTaskData, setPrepopulatedTaskData] = useState<TaskData | undefined>() 

  // Check if org slug is set in the URL
  const orgSlug = router.params.org_id || ''

  let listTriggersEnabled = false

  if (orgSlug === '') {
    listTriggersEnabled = true
  } else if (account.organizationTools.organization?.id) {
    listTriggersEnabled = true
  }

  const { data: triggers, isLoading, refetch } = useListUserCronTriggers(
    account.organizationTools.organization?.id || '',
    {
      enabled: listTriggersEnabled && !!account.user,
    }
  )

  const [deleteTriggerId, setDeleteTriggerId] = useState<string>('')
  const deleteTriggerMutation = useDeleteAppTrigger(deleteTriggerId, account.organizationTools.organization?.id || '')

  useEffect(() => {
    if(account.user) {
      apps.loadApps()
    }
  }, [
    account,apps.loadApps,
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

  const checkLoginStatus = (): boolean => {
    if (!account.user) {
      account.setShowLoginWindow(true)
      return false
    }
    return true
  }

  const handleCreateTask = (taskData?: TaskData) => {
    if (!checkLoginStatus()) return
    
    setSelectedTask(undefined)
    setPrepopulatedTaskData(taskData)
    setDialogOpen(true)
  }

  const handleEditTask = (task: TypesTriggerConfiguration) => {
    setSelectedTask(task)
    setPrepopulatedTaskData(undefined)
    setDialogOpen(true)
  }

  const handleCloseDialog = () => {
    setDialogOpen(false)
    setSelectedTask(undefined)
    setPrepopulatedTaskData(undefined)
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
        authenticated={ !!account.user }
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
            onClick={() => handleCreateTask()}
          >
            New
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
        prepopulatedData={prepopulatedTaskData}
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
