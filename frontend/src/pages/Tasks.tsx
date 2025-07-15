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

import { useListUserCronTriggers } from '../services/appService'
import useAccount from '../hooks/useAccount'
import useApps from '../hooks/useApps'
import useSnackbar from '../hooks/useSnackbar'

import { TypesTriggerConfiguration } from '../api/api'

const Tasks: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const snackbar = useSnackbar()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [selectedTask, setSelectedTask] = useState<TypesTriggerConfiguration | undefined>()
  const [deletingTask, setDeletingTask] = useState<TypesTriggerConfiguration | undefined>()

  const { data: triggers, isLoading } = useListUserCronTriggers(
    account.organizationTools.organization?.id || ''
  )

  useEffect(() => {
    apps.loadApps()
  }, [
    apps.loadApps,
  ])

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
    if (!deletingTask) return
    
    // TODO: Implement delete API call
    // const result = await deleteTask(deletingTask.id)
    // if (result) {
    //   snackbar.success('Task deleted')
    // }
    
    setDeletingTask(undefined)
  }

  const handleToggleStatus = async (task: TypesTriggerConfiguration) => {
    // TODO: Implement toggle status API call
    // const result = await toggleTaskStatus(task.id, !task.enabled)
    // if (result) {
    //   snackbar.success(task.enabled ? 'Task paused' : 'Task enabled')
    // }
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
