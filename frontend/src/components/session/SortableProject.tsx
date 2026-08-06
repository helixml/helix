import { ReactNode } from 'react'
import { useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import Box from '@mui/material/Box'

export type SortableProjectHandleProps = Pick<
  ReturnType<typeof useSortable>,
  'attributes' | 'listeners' | 'setActivatorNodeRef'
>

type SortableProjectProps = {
  projectId: string
  disabled: boolean
  render: (dragHandleProps: SortableProjectHandleProps) => ReactNode
}

const SortableProject = ({ projectId, disabled, render }: SortableProjectProps) => {
  const {
    attributes,
    listeners,
    setActivatorNodeRef,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: projectId, disabled })

  return (
    <Box
      ref={setNodeRef}
      sx={{
        position: 'relative',
        zIndex: isDragging ? 2 : 'auto',
        opacity: isDragging ? 0.78 : 1,
        transform: CSS.Transform.toString(transform ? { ...transform, x: 0 } : null),
        transition,
      }}
    >
      {render({ attributes, listeners, setActivatorNodeRef })}
    </Box>
  )
}

export default SortableProject
