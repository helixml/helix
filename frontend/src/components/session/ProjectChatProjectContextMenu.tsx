import { FC } from 'react'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import { Kanban, Settings } from 'lucide-react'

import type { TypesProject } from '../../api/api'
import type { ProjectChatContextMenuPosition } from './ProjectChatItemContextMenu'

type ProjectChatProjectContextMenuProps = {
  project: TypesProject | null
  position: ProjectChatContextMenuPosition | null
  onClose: () => void
  onOpenBoard: (project: TypesProject) => void
  onOpenSettings: (project: TypesProject) => void
}

const ProjectChatProjectContextMenu: FC<ProjectChatProjectContextMenuProps> = ({
  project,
  position,
  onClose,
  onOpenBoard,
  onOpenSettings,
}) => {
  const select = (action: (selectedProject: TypesProject) => void) => {
    if (!project) return
    onClose()
    action(project)
  }

  return (
    <Menu
      open={!!project && !!position}
      onClose={onClose}
      anchorReference="anchorPosition"
      anchorPosition={position ? { top: position.mouseY, left: position.mouseX } : undefined}
    >
      <MenuItem onClick={() => select(onOpenBoard)}>
        <ListItemIcon>
          <Kanban size={16} />
        </ListItemIcon>
        <ListItemText>Project board</ListItemText>
      </MenuItem>
      <MenuItem onClick={() => select(onOpenSettings)}>
        <ListItemIcon>
          <Settings size={16} />
        </ListItemIcon>
        <ListItemText>Project settings</ListItemText>
      </MenuItem>
    </Menu>
  )
}

export default ProjectChatProjectContextMenu
