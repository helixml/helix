import { FC, useMemo } from 'react'

import CardGrid from '../widgets/CardGrid'
import { ViewMode } from '../widgets/ViewModeToggle'
import CronTaskCard from './CronTaskCard'
import TasksTable from './TasksTable'
import { TypesTriggerConfiguration } from '../../api/api'
import { IApp } from '../../types'

interface TasksViewProps {
  mode: ViewMode
  authenticated: boolean
  data: TypesTriggerConfiguration[]
  apps: IApp[]
  onEdit: (task: TypesTriggerConfiguration) => void
  onDelete: (task: TypesTriggerConfiguration) => void
  onToggleStatus: (task: TypesTriggerConfiguration) => void
}

const TasksView: FC<TasksViewProps> = ({
  mode,
  authenticated,
  data,
  apps,
  onEdit,
  onDelete,
  onToggleStatus,
}) => {
  const appsById = useMemo(() => {
    const map: Record<string, IApp> = {}
    apps.forEach((a) => {
      if (a.id) map[a.id] = a
    })
    return map
  }, [apps])

  if (mode === 'cards') {
    return (
      <CardGrid
        items={data}
        getKey={(t) => t.id ?? ''}
        renderCard={(t) => (
          <CronTaskCard
            task={t}
            app={t.app_id ? appsById[t.app_id] : undefined}
            onEdit={onEdit}
            onDelete={onDelete}
            onToggleStatus={onToggleStatus}
          />
        )}
      />
    )
  }

  return (
    <TasksTable
      authenticated={authenticated}
      data={data}
      apps={apps}
      onEdit={onEdit}
      onDelete={onDelete}
      onToggleStatus={onToggleStatus}
    />
  )
}

export default TasksView
