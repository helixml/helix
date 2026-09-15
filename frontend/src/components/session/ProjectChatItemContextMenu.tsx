import { FC, FormEvent, useState } from 'react'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogActions from '@mui/material/DialogActions'
import DialogContent from '@mui/material/DialogContent'
import DialogTitle from '@mui/material/DialogTitle'
import Divider from '@mui/material/Divider'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import TextField from '@mui/material/TextField'
import { useQueryClient } from '@tanstack/react-query'
import { Kanban, PanelsTopLeft, Pencil, Pin, PinOff, Play, Settings, Square } from 'lucide-react'

import useApi from '../../hooks/useApi'
import useSnackbar from '../../hooks/useSnackbar'
import { GET_SESSION_QUERY_KEY, useRenameSession } from '../../services/sessionService'
import { invalidateSpecTaskStatusQueries, useUpdateSpecTask } from '../../services/specTaskService'
import { usePinChat, useUnpinChat } from '../../services/chatPinService'
import { getSandboxControl } from './ProjectChatSidebar.logic'
import type { SidebarItem } from './ProjectChatSidebar.logic'

export type ProjectChatContextMenuPosition = {
  mouseX: number
  mouseY: number
}

type ProjectChatItemContextMenuProps = {
  item: SidebarItem | null
  position: ProjectChatContextMenuPosition | null
  onClose: () => void
  onOpenProjectBoard?: (projectId: string) => void
  onOpenProjectSettings?: (projectId: string) => void
  onOpenProjectArtifacts?: (projectId: string) => void
}

const ProjectChatItemContextMenu: FC<ProjectChatItemContextMenuProps> = ({
  item,
  position,
  onClose,
  onOpenProjectBoard,
  onOpenProjectSettings,
  onOpenProjectArtifacts,
}) => {
  const api = useApi()
  const queryClient = useQueryClient()
  const snackbar = useSnackbar()
  const renameSession = useRenameSession()
  const updateSpecTask = useUpdateSpecTask()
  const pinChat = usePinChat()
  const unpinChat = useUnpinChat()
  const [renameItem, setRenameItem] = useState<SidebarItem | null>(null)
  const [name, setName] = useState('')
  const [saving, setSaving] = useState(false)

  const projectId = item?.projectId
  const sandboxControl = item ? getSandboxControl(item) : null

  const togglePin = async () => {
    if (!item) return
    const pinned = !!item.pinnedAt
    onClose()
    try {
      await (pinned ? unpinChat : pinChat).mutateAsync({
        id: item.id,
        kind: item.kind,
        project_id: item.projectId,
      })
      snackbar.success(pinned ? 'Chat unpinned' : 'Chat pinned')
    } catch {
      snackbar.error(pinned ? 'Failed to unpin chat' : 'Failed to pin chat')
    }
  }

  const openProjectPage = (open?: (projectId: string) => void) => {
    if (!projectId || !open) return
    onClose()
    open(projectId)
  }

  const toggleSandbox = async () => {
    if (!item || !sandboxControl) return
    const stopping = sandboxControl.state !== 'absent'
    const taskId = item.kind === 'spec-task' ? item.id : undefined
    onClose()
    try {
      snackbar.info(stopping ? 'Stopping sandbox…' : 'Starting sandbox…')
      if (stopping) {
        await api.getApiClient().v1SessionsStopExternalAgentDelete(sandboxControl.sessionId)
      } else {
        await api.getApiClient().v1SessionsResumeCreate(sandboxControl.sessionId)
      }
      queryClient.invalidateQueries({ queryKey: GET_SESSION_QUERY_KEY(sandboxControl.sessionId) })
      queryClient.invalidateQueries({ queryKey: ['sessions'] })
      if (taskId) await invalidateSpecTaskStatusQueries(queryClient, taskId)
      snackbar.success(stopping ? 'Sandbox stopped' : 'Sandbox starting')
    } catch (error: any) {
      const message = typeof error?.response?.data === 'string'
        ? error.response.data
        : error?.response?.data?.message || error?.message
      snackbar.error(message || (stopping ? 'Failed to stop sandbox' : 'Failed to start sandbox'))
    }
  }

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

  const showProjectActions = !!projectId
    && (!!onOpenProjectBoard || !!onOpenProjectSettings || !!onOpenProjectArtifacts)

  return (
    <>
      <Menu
        open={!!item && !!position}
        onClose={onClose}
        anchorReference="anchorPosition"
        anchorPosition={position ? { top: position.mouseY, left: position.mouseX } : undefined}
      >
        <MenuItem onClick={() => void togglePin()}>
          <ListItemIcon>
            {item?.pinnedAt ? <PinOff size={16} /> : <Pin size={16} />}
          </ListItemIcon>
          <ListItemText>{item?.pinnedAt ? 'Unpin' : 'Pin'}</ListItemText>
        </MenuItem>
        <MenuItem onClick={openRenameDialog}>
          <ListItemIcon>
            <Pencil size={16} />
          </ListItemIcon>
          <ListItemText>Rename</ListItemText>
        </MenuItem>
        {showProjectActions && <Divider />}
        {!!projectId && !!onOpenProjectBoard && (
          <MenuItem onClick={() => openProjectPage(onOpenProjectBoard)}>
            <ListItemIcon>
              <Kanban size={16} />
            </ListItemIcon>
            <ListItemText>Project board</ListItemText>
          </MenuItem>
        )}
        {!!projectId && !!onOpenProjectSettings && (
          <MenuItem onClick={() => openProjectPage(onOpenProjectSettings)}>
            <ListItemIcon>
              <Settings size={16} />
            </ListItemIcon>
            <ListItemText>Project settings</ListItemText>
          </MenuItem>
        )}
        {!!projectId && !!onOpenProjectArtifacts && (
          <MenuItem onClick={() => openProjectPage(onOpenProjectArtifacts)}>
            <ListItemIcon>
              <PanelsTopLeft size={16} />
            </ListItemIcon>
            <ListItemText>Project artifacts</ListItemText>
          </MenuItem>
        )}
        {!!sandboxControl && <Divider />}
        {!!sandboxControl && (
          <MenuItem onClick={() => void toggleSandbox()}>
            <ListItemIcon>
              {sandboxControl.state === 'absent' ? <Play size={16} /> : <Square size={16} />}
            </ListItemIcon>
            <ListItemText>
              {sandboxControl.state === 'absent' ? 'Start sandbox' : 'Stop sandbox'}
            </ListItemText>
          </MenuItem>
        )}
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
