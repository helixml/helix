import { FC, Fragment, useState, useCallback, useEffect, useRef } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Box from '@mui/material/Box'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'

import FolderIcon from '@mui/icons-material/Folder'
import InsertDriveFileIcon from '@mui/icons-material/InsertDriveFile'
import ImageIcon from '@mui/icons-material/Image'
import DescriptionIcon from '@mui/icons-material/Description'
import VideoFileIcon from '@mui/icons-material/VideoFile'
import AudioFileIcon from '@mui/icons-material/AudioFile'
import CodeIcon from '@mui/icons-material/Code'
import ArchiveIcon from '@mui/icons-material/Archive'
import AddIcon from '@mui/icons-material/Add'
import CreateNewFolderIcon from '@mui/icons-material/CreateNewFolder'
import CloudUploadIcon from '@mui/icons-material/CloudUpload'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ClickLink from '../widgets/ClickLink'
import SlideMenuContainer from '../system/SlideMenuContainer'

import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useAccount from '../../hooks/useAccount'
import { useListFilestore, useCreateFilestoreFolder, useUploadFilestoreFiles } from '../../services/filestoreService'
import { FilestoreItem } from '../../api/api'
import useSnackbar from '../../hooks/useSnackbar'

// Menu identifier constant
const MENU_TYPE = 'files'

// Pagination constants
const PAGE_SIZE = 20

export const FilesSidebar: FC<{
  onOpenFile: () => void,
}> = ({
  onOpenFile,
}) => {
  const account = useAccount()
  const router = useRouter()
  const snackbar = useSnackbar()
  const [currentPage, setCurrentPage] = useState(0)
  const [allFiles, setAllFiles] = useState<FilestoreItem[]>([])
  const [hasMore, setHasMore] = useState(true)
  const [totalCount, setTotalCount] = useState(0)

  // New file menu state
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null)
  const [createFolderDialogOpen, setCreateFolderDialogOpen] = useState(false)
  const [folderName, setFolderName] = useState('')
  const [fileInputRef, setFileInputRef] = useState<HTMLInputElement | null>(null)

  const orgId = router.params.org_id

  const {
    data: filesData,
    isLoading: isLoadingFiles,
    isFetching: isLoadingMore,
    error
  } = useListFilestore(
    '', // List root directory
    !!account.user?.id // Only load if logged in
  )

  // Mutation hooks for file operations
  const createFolderMutation = useCreateFilestoreFolder()
  const uploadFilesMutation = useUploadFilestoreFiles()

  // Update state when files data changes
  useEffect(() => {
    if (filesData) {
      // For now, we'll show all files at once since the API doesn't support pagination
      // In the future, we can implement pagination if the API supports it
      setAllFiles(filesData || [])
      setTotalCount(filesData?.length || 0)
      setHasMore(false) // No pagination for now
    }
  }, [filesData])

  const loadMore = useCallback(() => {
    // No pagination for now
  }, [])

  const resetPagination = useCallback(() => {
    setCurrentPage(0)
    setAllFiles([])
    setHasMore(true)
  }, [])

  // Reset pagination when organization changes
  useEffect(() => {
    resetPagination()
  }, [orgId, resetPagination])

  // Handler for opening the new file menu
  const handleNewFileClick = (event: React.MouseEvent<HTMLElement>) => {
    setMenuAnchorEl(event.currentTarget)
  }

  // Handler for closing the new file menu
  const handleMenuClose = () => {
    setMenuAnchorEl(null)
  }

  // Handler for creating a new folder
  const handleCreateFolder = () => {
    setCreateFolderDialogOpen(true)
    handleMenuClose()
  }

  // Handler for uploading files
  const handleUploadFiles = () => {
    if (fileInputRef) {
      fileInputRef.click()
    }
    handleMenuClose()
  }

  // Handler for file input change
  const handleFileInputChange = async (event: React.ChangeEvent<HTMLInputElement>) => {
    const files = Array.from(event.target.files || [])
    if (files.length === 0) return

    try {
      await uploadFilesMutation.mutateAsync({
        path: '', // Upload to root directory
        files: files
      })
      snackbar.success(`Successfully uploaded ${files.length} file(s)`)
    } catch (error) {
      console.error('Upload error:', error)
      snackbar.error('Failed to upload files')
    }

    // Reset the input
    if (event.target) {
      event.target.value = ''
    }
  }

  // Handler for creating folder dialog
  const handleCreateFolderSubmit = async () => {
    if (!folderName.trim()) return

    try {
      await createFolderMutation.mutateAsync(folderName.trim())
      snackbar.success('Folder created successfully')
      setCreateFolderDialogOpen(false)
      setFolderName('')
    } catch (error) {
      console.error('Create folder error:', error)
      snackbar.error('Failed to create folder')
    }
  }

  // Handler for canceling folder creation
  const handleCreateFolderCancel = () => {
    setCreateFolderDialogOpen(false)
    setFolderName('')
  }
  
  const lightTheme = useLightTheme()
  const {
    params,
  } = useRouter()

  const getFileIcon = (file: FilestoreItem) => {
    if (file.directory) {
      return <FolderIcon color="primary" />
    }

    const fileName = file.name || ''
    const extension = fileName.split('.').pop()?.toLowerCase()

    // Image files
    if (['jpg', 'jpeg', 'png', 'gif', 'bmp', 'svg', 'webp'].includes(extension || '')) {
      return <ImageIcon color="primary" />
    }

    // Video files
    if (['mp4', 'avi', 'mov', 'wmv', 'flv', 'webm', 'mkv'].includes(extension || '')) {
      return <VideoFileIcon color="primary" />
    }

    // Audio files
    if (['mp3', 'wav', 'flac', 'aac', 'ogg', 'm4a'].includes(extension || '')) {
      return <AudioFileIcon color="primary" />
    }

    // Archive files
    if (['zip', 'rar', '7z', 'tar', 'gz', 'bz2'].includes(extension || '')) {
      return <ArchiveIcon color="primary" />
    }

    // Code files
    if (['js', 'ts', 'jsx', 'tsx', 'py', 'java', 'cpp', 'c', 'h', 'go', 'rs', 'php', 'rb', 'swift', 'kt', 'scala', 'sh', 'bash', 'ps1', 'bat', 'cmd'].includes(extension || '')) {
      return <CodeIcon color="primary" />
    }

    // Document files
    if (['pdf', 'doc', 'docx', 'txt', 'rtf', 'odt', 'pages'].includes(extension || '')) {
      return <DescriptionIcon color="primary" />
    }

    // Default file icon
    return <InsertDriveFileIcon color="primary" />
  }


  const renderFile = (file: FilestoreItem) => {
    const filePath = file.path || file.name || ''
    const isActive = filePath === params["file_path"]
    return (
      <ListItem
        sx={{
          borderRadius: '20px',
          cursor: 'pointer',
          width: '100%',
          padding: 0,
        }}
        key={filePath}
        onClick={() => {
          if (file.directory) {
            // Navigate to folder
            account.orgNavigate('files', {file_path: filePath})
          } else {
            // Open file
            account.orgNavigate('files', {file_path: filePath})
            onOpenFile()
          }
        }}
      >
        <ListItemButton
          selected={isActive}
          sx={{
            borderRadius: '4px',
            backgroundColor: isActive ? '#1a1a2f' : 'transparent',
            cursor: 'pointer',
            width: '100%',
            mr: -2,
            '&:hover': {
              '.MuiListItemText-root .MuiTypography-root': { color: '#fff' },
              '.MuiListItemIcon-root': { color: '#fff' },
            },
          }}
        >
          <ListItemIcon
            sx={{color:'red'}}
          >
            {getFileIcon(file)}
          </ListItemIcon>
          <ListItemText
            sx={{marginLeft: "-15px"}}
            primaryTypographyProps={{
              fontSize: 'small',
              overflow: 'hidden',
              textOverflow: 'ellipsis',
              whiteSpace: 'nowrap',
              color: isActive ? '#fff' : lightTheme.textColorFaded,
            }}
            primary={file.name}
            id={filePath}
          />
        </ListItemButton>
      </ListItem>
    )
  }


  // Show loading state for initial load
  if (isLoadingFiles && currentPage === 0) {
    return (
      <SlideMenuContainer menuType={MENU_TYPE}>
        <Row center sx={{ py: 4 }}>
          <Cell>
            <CircularProgress size={24} />
          </Cell>
        </Row>
      </SlideMenuContainer>
    )
  }

  // Show error state if there's an error
  if (error) {
    return (
      <SlideMenuContainer menuType={MENU_TYPE}>
        <Row center sx={{ py: 4 }}>
          <Cell>
            <Typography color="error" variant="body2">
              Failed to load files
            </Typography>
          </Cell>
        </Row>
      </SlideMenuContainer>
    )
  }

  // Show message when user is not logged in
  if (!account.user?.id) {
    return (
      <SlideMenuContainer menuType={MENU_TYPE}>
        <Row center sx={{ py: 4 }}>
          <Cell>
            <Typography 
              variant="body2" 
              sx={{ 
                color: 'text.secondary',
                opacity: 0.6,
                textAlign: 'center'
              }}
            >
              Login to see your files
            </Typography>
          </Cell>
        </Row>
      </SlideMenuContainer>
    )
  }

  return (
    <SlideMenuContainer menuType={MENU_TYPE}>
      {/* New File Button */}
      <Box
        sx={{
          width: '100%',
          px: 2,
          py: 1,
        }}
      >
        <Button
          variant="contained"
          color="secondary"
          startIcon={<AddIcon />}
          onClick={handleNewFileClick}
          sx={{
            width: '100%',
            height: '48px',
            borderRadius: '8px',
            fontWeight: 'bold',
            textTransform: 'none',
            fontSize: '14px',
            backgroundColor: '#00E5FF',
            color: '#000',
            '&:hover': {
              backgroundColor: '#00D4E6',
            },
          }}
        >
          New File
        </Button>
      </Box>

      <List
        sx={{
          py: 1,
          px: 2,
          minHeight: 'fit-content', // Allow natural content height
          overflow: 'visible', // Let content contribute to parent height
          width: '100%', // Ensure it doesn't exceed container width
        }}
      >
        {allFiles.map(renderFile)}
      </List>
      {
        totalCount > 0 && totalCount > PAGE_SIZE && (
          <Row
            sx={{
              mt: 2,
              mb: 1,
            }}
            center
          >
            <Cell grow sx={{
              textAlign: 'center',
              fontSize: '0.8em'
            }}>
              {
                isLoadingMore && (
                  <CircularProgress
                    size={ 20 }
                  />
                )
              }
              {
                !isLoadingMore && hasMore && (
                  <ClickLink
                    onClick={ loadMore }
                  >
                    Load More...
                  </ClickLink>
                )
              }
              {
                !isLoadingMore && !hasMore && totalCount > PAGE_SIZE && (
                  <Typography variant="caption" color="text.secondary">
                    All files loaded
                  </Typography>
                )
              }
            </Cell>
          </Row>
        )
      }

      {/* New File Dropdown Menu */}
      <Menu
        anchorEl={menuAnchorEl}
        open={Boolean(menuAnchorEl)}
        onClose={handleMenuClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'left',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'left',
        }}
        sx={{
          '& .MuiPaper-root': {
            minWidth: 200,
            mt: 1,
          },
        }}
      >
        <MenuItem onClick={handleCreateFolder}>
          <CreateNewFolderIcon sx={{ mr: 1 }} />
          Create Directory
        </MenuItem>
        <MenuItem onClick={handleUploadFiles}>
          <CloudUploadIcon sx={{ mr: 1 }} />
          Upload File
        </MenuItem>
      </Menu>

      {/* Hidden file input for uploads */}
      <input
        type="file"
        ref={setFileInputRef}
        onChange={handleFileInputChange}
        multiple
        style={{ display: 'none' }}
      />

      {/* Create Folder Dialog */}
      <Dialog
        open={createFolderDialogOpen}
        onClose={handleCreateFolderCancel}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Create New Directory</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            margin="dense"
            label="Directory Name"
            fullWidth
            variant="outlined"
            value={folderName}
            onChange={(e) => setFolderName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                handleCreateFolderSubmit()
              }
            }}
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCreateFolderCancel}>
            Cancel
          </Button>
          <Button 
            onClick={handleCreateFolderSubmit}
            variant="contained"
            disabled={!folderName.trim() || createFolderMutation.isPending}
          >
            {createFolderMutation.isPending ? 'Creating...' : 'Create'}
          </Button>
        </DialogActions>
      </Dialog>
    </SlideMenuContainer>
  )
}

export default FilesSidebar
