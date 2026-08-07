import { FC, FormEvent, useState } from 'react'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import TextField from '@mui/material/TextField'
import { Pencil } from 'lucide-react'

import useSnackbar from '../../hooks/useSnackbar'
import { useRenameSession } from '../../services/sessionService'
import { useUpdateSpecTask } from '../../services/specTaskService'
import type { SidebarItem } from './ProjectChatSidebar.logic'

export type ProjectChatContextMenuPosition = {
  mouseX: number
  mouseY: number
}

type ProjectChatItemContextMenuProps = {
  item: SidebarItem | null
  position: ProjectChatContextMenuPosition | null
  onClose: () => void
}

const ProjectChatItemContextMenu: FC<ProjectChatItemContextMenuProps> = ({
  item,
  position,
  onClose,
}) => {
  const snackbar = useSnackbar()
  const renameSession = useRenameSession()
  const updateSpecTask = useUpdateSpecTask()
  const [renameItem, setRenameItem] = useState<SidebarItem | null>(null)
  const [name, setName] = useState('')
  const [saving, setSaving] = useState(false)

  const openRenameDialog = () => {
    if (!item) return
    setRenameItem(item)
    setName(item.title)
    onClose()
  }

  const closeRenameDialog = () => {
    if (saving) return
    setRenameItem(null)
    setName('')
  }

  const submitRename = async (event: FormEvent) => {
    event.preventDefault()
    const trimmedName = name.trim()
    if (!renameItem || !trimmedName || saving) return

    const label = renameItem.kind === 'spec-task' ? 'task' : 'chat'
    if (trimmedName === renameItem.title) {
      closeRenameDialog()
      return
    }

    setSaving(true)
    try {
      if (renameItem.kind === 'spec-task') {
        await updateSpecTask.mutateAsync({
          taskId: renameItem.id,
          updates: { user_short_title: trimmedName },
        })
      } else {
        await renameSession.mutateAsync({
          sessionId: renameItem.id,
          name: trimmedName,
        })
      }
      snackbar.success(`${label === 'task' ? 'Task' : 'Chat'} renamed`)
      setRenameItem(null)
      setName('')
    } catch (error: any) {
      const message = typeof error?.response?.data === 'string'
        ? error.response.data
        : error?.response?.data?.message
      snackbar.error(message || `Failed to rename ${label}`)
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <Menu
        open={!!item && !!position}
        onClose={onClose}
        anchorReference="anchorPosition"
        anchorPosition={position ? { top: position.mouseY, left: position.mouseX } : undefined}
      >
        <MenuItem onClick={openRenameDialog}>
          <ListItemIcon>
            <Pencil size={16} />
          </ListItemIcon>
          <ListItemText>Rename</ListItemText>
        </MenuItem>
      </Menu>

      <Dialog
        open={!!renameItem}
        onClose={closeRenameDialog}
        fullWidth
        maxWidth="xs"
      >
        <form onSubmit={submitRename}>
          <DialogTitle>
            Rename {renameItem?.kind === 'spec-task' ? 'task' : 'chat'}
          </DialogTitle>
          <DialogContent>
            <TextField
              autoFocus
              fullWidth
              label="Name"
              value={name}
              onChange={(event) => setName(event.target.value)}
              onFocus={(event) => event.target.select()}
              disabled={saving}
              margin="dense"
            />
          </DialogContent>
          <DialogActions>
            <Button onClick={closeRenameDialog} disabled={saving}>Cancel</Button>
            <Button type="submit" variant="contained" disabled={!name.trim() || saving}>
              {saving ? 'Renaming…' : 'Rename'}
            </Button>
          </DialogActions>
        </form>
      </Dialog>
    </>
  )
}

export default ProjectChatItemContextMenu
