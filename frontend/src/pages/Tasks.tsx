import React, { FC, useEffect, useState } from 'react'
import Button from '@mui/material/Button'
import AddIcon from '@mui/icons-material/Add'
import Container from '@mui/material/Container'
import Box from '@mui/material/Box'
import Grid from '@mui/material/Grid'
import Typography from '@mui/material/Typography'
import Tabs from '@mui/material/Tabs'
import Tab from '@mui/material/Tab'
import LoadingSpinner from '../components/widgets/LoadingSpinner'

import Page from '../components/system/Page'
import TaskDialog from '../components/tasks/TaskDialog'
import TaskCard from '../components/tasks/TaskCard'
import EmptyTasksState from '../components/tasks/EmptyTasksState'

import { useListUserCronTriggers } from '../services/appService'
import useAccount from '../hooks/useAccount'
import useApps from '../hooks/useApps'

import { TypesTriggerConfiguration } from '../api/api'

const Tasks: FC = () => {
  const account = useAccount()
  const apps = useApps()
  const [dialogOpen, setDialogOpen] = useState(false)
  const [selectedTask, setSelectedTask] = useState<TypesTriggerConfiguration | undefined>()
  const [activeTab, setActiveTab] = useState(0)

  const { data: triggers, isLoading } = useListUserCronTriggers(
    account.organizationTools.organization?.id || '',
    { enabled: !!account.organizationTools.organization?.id }
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

  const handleTabChange = (event: React.SyntheticEvent, newValue: number) => {
    setActiveTab(newValue)
  }

  // Filter tasks based on active tab
  const activeTasks = triggers?.data?.filter((task: TypesTriggerConfiguration) => !task.archived) || []
  const archivedTasks = triggers?.data?.filter((task: TypesTriggerConfiguration) => task.archived) || []
  const currentTasks = activeTab === 0 ? activeTasks : archivedTasks

  const renderContent = () => {
    if (isLoading) {
      return (
        <Box sx={{ display: 'flex', justifyContent: 'center', py: 8 }}>
          <LoadingSpinner />
        </Box>
      )
    }

    if (currentTasks.length === 0) {
      return (
        <EmptyTasksState onCreateTask={handleCreateTask} />
      )
    }

    return (
      <Grid container spacing={3}>
        {currentTasks.map((task: TypesTriggerConfiguration) => (
          <Grid item xs={12} sm={6} md={4} key={task.id}>
            <TaskCard task={task} onClick={handleEditTask} />
          </Grid>
        ))}
      </Grid>
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
        {/* Tabs */}
        <Box sx={{ borderBottom: 1, borderColor: 'divider', mb: 3 }}>
          <Tabs 
            value={activeTab} 
            onChange={handleTabChange}
            sx={{
              '& .MuiTab-root': {
                textTransform: 'none',
                fontWeight: 500,
              }
            }}
          >
            <Tab 
              label={`Active (${activeTasks.length})`} 
              id="active-tab"
            />
            <Tab 
              label={`Archived (${archivedTasks.length})`} 
              id="archived-tab"
            />
          </Tabs>
        </Box>

        {/* Content */}
        {renderContent()}
      </Container>

      {/* Task Dialog */}
      <TaskDialog
        open={dialogOpen}
        onClose={handleCloseDialog}
        task={selectedTask}
        apps={apps.apps}
      />
    </Page>
  )
}

export default Tasks
