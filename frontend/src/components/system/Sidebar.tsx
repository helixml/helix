import React, { useState, useMemo, useEffect, ReactNode } from 'react'
import Button from '@mui/material/Button'
import Box from '@mui/material/Box'
import List from '@mui/material/List'
import Divider from '@mui/material/Divider'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemText from '@mui/material/ListItemText'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import TextField from '@mui/material/TextField'
import { styled, keyframes } from '@mui/material/styles'

import { Plus, FolderPlus, Upload, FileText } from 'lucide-react'
import useThemeConfig from '../../hooks/useThemeConfig'
import useLightTheme from '../../hooks/useLightTheme'
import useRouter from '../../hooks/useRouter'
import useAccount from '../../hooks/useAccount'
import useApp from '../../hooks/useApp'
import useApi from '../../hooks/useApi'

import { useCreateFilestoreFolder, useUploadFilestoreFiles, useFilestoreConfig } from '../../services/filestoreService'
import DarkDialog from '../dialog/DarkDialog'
import useSnackbar from '../../hooks/useSnackbar'

import SlideMenuContainer from './SlideMenuContainer'
import SidebarContextHeader from './SidebarContextHeader'
import UnifiedSearchBar from '../common/UnifiedSearchBar'
import { SidebarProvider, useSidebarContext } from '../../contexts/sidebarContext'


const shimmer = keyframes`
  0% {
    background-position: -200% center;
    box-shadow: 0 0 10px rgba(0, 229, 255, 0.2);
  }
  50% {
    box-shadow: 0 0 20px rgba(0, 229, 255, 0.4);
  }
  100% {
    background-position: 200% center;
    box-shadow: 0 0 10px rgba(0, 229, 255, 0.2);
  }
`

const pulse = keyframes`
  0% {
    transform: scale(1);
  }
  50% {
    transform: scale(1.02);
  }
  100% {
    transform: scale(1);
  }
`

const ShimmerButton = styled(Button)(({ theme }) => ({
  background: `linear-gradient(
    90deg, 
    ${theme.palette.secondary.dark} 0%,
    ${theme.palette.secondary.main} 20%,
    ${theme.palette.secondary.light} 50%,
    ${theme.palette.secondary.main} 80%,
    ${theme.palette.secondary.dark} 100%
  )`,
  backgroundSize: '200% auto',
  animation: `${shimmer} 2s linear infinite, ${pulse} 3s ease-in-out infinite`,
  transition: 'all 0.3s ease-in-out',
  boxShadow: '0 0 15px rgba(0, 229, 255, 0.3)',
  fontWeight: 'bold',
  letterSpacing: '0.5px',
  padding: '6px 16px',
  fontSize: '0.875rem',
  '&:hover': {
    transform: 'scale(1.05)',
    boxShadow: '0 0 25px rgba(0, 229, 255, 0.6)',
    backgroundSize: '200% auto',
    animation: `${shimmer} 1s linear infinite`,
  },
}))

// Inner component that uses the sidebar context
const SidebarContentInner: React.FC<{
  showTopLinks?: boolean,
  menuType: string,
  children: ReactNode,
}> = ({
  children,
  showTopLinks = true,
  menuType,
}) => {
  const { userMenuHeight } = useSidebarContext()
  const themeConfig = useThemeConfig()
  const lightTheme = useLightTheme()

  const {
    params
  } = useRouter()
  

  const router = useRouter()
  const api = useApi()
  const account = useAccount()
  const appTools = useApp(params.app_id)
  const snackbar = useSnackbar()

  const apiClient = api.getApiClient()

  // New file menu state
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null)
  const [createFolderDialogOpen, setCreateFolderDialogOpen] = useState(false)
  const [folderName, setFolderName] = useState('')
  const [fileInputRef, setFileInputRef] = useState<HTMLInputElement | null>(null)
  const [createFileDialogOpen, setCreateFileDialogOpen] = useState(false)
  const [fileName, setFileName] = useState('')

  // Mutation hooks for file operations
  const createFolderMutation = useCreateFilestoreFolder()
  const uploadFilesMutation = useUploadFilestoreFiles()
  const { data: filestoreConfig } = useFilestoreConfig()



  // Ensure apps are loaded when apps tab is selected
  useEffect(() => {
    const checkAuthAndLoad = async () => {
      try {
        const authResponse = await apiClient.v1AuthAuthenticatedList()
        if (!authResponse.data.authenticated) {
          return
        }        
        
      } catch (error) {
        console.error('[SIDEBAR] Error checking authentication:', error)
      }
    }

    checkAuthAndLoad()
  }, [router.params])    

  // Handle create a new chat
  const handleCreateNew = () => {
    if (!appTools.app) {
      account.orgNavigate('chat')
      return
    }
    // If we are in the app details view, we need to create a new chat
    account.orgNavigate('new', { app_id: appTools.id })
  }

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

  // Handler for creating a new file
  const handleCreateFile = () => {
    setCreateFileDialogOpen(true)
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
        files: files,
        config: filestoreConfig
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

  // Handler for creating file dialog
  const handleCreateFileSubmit = async () => {
    if (!fileName.trim()) return

    try {
      // Create an empty file by uploading an empty string
      const emptyFile = new File([''], fileName.trim(), { type: 'text/plain' })
      await uploadFilesMutation.mutateAsync({
        path: '', // Upload to root directory
        files: [emptyFile],
        config: filestoreConfig
      })
      snackbar.success('File created successfully')
      setCreateFileDialogOpen(false)
      setFileName('')
    } catch (error) {
      console.error('Create file error:', error)
      snackbar.error('Failed to create file')
    }
  }

  // Handler for canceling file creation
  const handleCreateFileCancel = () => {
    setCreateFileDialogOpen(false)
    setFileName('')
  }

  return (
    <SlideMenuContainer menuType={menuType}>
      <Box
        sx={{
          height: '100%',
          display: 'flex',
          flexDirection: 'column',
          alignItems: 'center',
          borderRight: lightTheme.border,
          backgroundColor: lightTheme.backgroundColor,
          width: '100%',
        }}
      >
        <SidebarContextHeader />
        {/* Global search - available on all pages */}
        <Box sx={{ pl: 2, pr: 2, py: 1 }}>
          <UnifiedSearchBar compact placeholder="Search..." />
        </Box>
        <Divider sx={{ mb: 1 }} />
        <Box
          sx={{
            flexGrow: 0,
            width: '100%',
          }}
        >
          {
            showTopLinks && (router.name === 'chat' || router.name === 'session' || router.name === 'qa-results' || router.name === 'app' || router.name === 'new' ||
                           router.name === 'org_chat' || router.name === 'org_session' || router.name === 'org_qa-results' || router.name === 'org_app' || router.name === 'org_new') && (
              <List disablePadding>    
                
                {/* New resource creation button */}
                <ListItem
                  disablePadding
                  dense
                >
                  <ListItemButton
                    id="create-link"
                    onClick={handleCreateNew}
                    sx={{
                      height: '64px',
                      display: 'flex',
                      '&:hover': {
                        '.MuiListItemText-root .MuiTypography-root': { color: '#FFFFFF' },
                      },
                    }}
                  >
                    <ListItemText
                      sx={{
                        ml: 1,
                        pl: 0,
                      }}
                      primary={`New Chat`}
                      primaryTypographyProps={{
                        fontWeight: 'bold',
                        color: '#FFFFFF',
                        fontSize: '16px',
                      }}
                    />
                    <Box 
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        backgroundColor: 'transparent',
                        border: '2px solid #00E5FF',
                        borderRadius: '50%',
                        width: 32,
                        height: 32,
                        mr: 2,
                      }}
                    >
                      <Plus size={20} color="#00E5FF" />
                    </Box>
                  </ListItemButton>
                </ListItem>
                
                <Divider />
              </List>
            )
          }
          {
            showTopLinks && router.name === 'files' && (
              <List disablePadding>    
                
                {/* New file creation button */}
                <ListItem
                  disablePadding
                  dense
                >
                  <ListItemButton
                    id="create-file-link"
                    onClick={handleNewFileClick}
                    sx={{
                      height: '64px',
                      display: 'flex',
                      '&:hover': {
                        '.MuiListItemText-root .MuiTypography-root': { color: '#FFFFFF' },
                      },
                    }}
                  >
                    <ListItemText
                      sx={{
                        ml: 1,
                        pl: 0,
                      }}
                      primary={`New`}
                      primaryTypographyProps={{
                        fontWeight: 'bold',
                        color: '#FFFFFF',
                        fontSize: '16px',
                      }}
                    />
                    <Box 
                      sx={{
                        display: 'flex',
                        alignItems: 'center',
                        justifyContent: 'center',
                        backgroundColor: 'transparent',
                        border: '2px solid #00E5FF',
                        borderRadius: '50%',
                        width: 32,
                        height: 32,
                        mr: 2,
                      }}
                    >
                      <Plus size={20} color="#00E5FF" />
                    </Box>
                  </ListItemButton>
                </ListItem>
                
                <Divider />
              </List>
            )
          }
        </Box>
        <Box
          sx={{
            flexGrow: 1,
            width: '100%',
            height: '100%', // Fixed height to fill available space
            overflow: 'auto', // Enable scrollbar when content exceeds height
            boxShadow: 'none', // Remove shadow for a more flat/minimalist design
            borderRight: 'none', // Remove the border if present
            mr: 3,
            mt: 1,
            ...lightTheme.scrollbar,
          }}
        >
          { children }
        </Box>
        {/* User section moved to UserOrgSelector component */}
      </Box>

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
        <MenuItem onClick={handleCreateFile}>
          <FileText size={16} style={{ marginRight: 8 }} />
          New
        </MenuItem>
        <MenuItem onClick={handleCreateFolder}>
          <FolderPlus size={16} style={{ marginRight: 8 }} />
          Create Directory
        </MenuItem>
        <MenuItem onClick={handleUploadFiles}>
          <Upload size={16} style={{ marginRight: 8 }} />
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

      {/* Create File Dialog */}
      <DarkDialog
        open={createFileDialogOpen}
        onClose={handleCreateFileCancel}
        maxWidth="sm"
        fullWidth
      >
        <DialogTitle>Create New File</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            margin="dense"
            label="File Name"
            fullWidth
            variant="outlined"
            value={fileName}
            onChange={(e) => setFileName(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') {
                handleCreateFileSubmit()
              }
            }}
            placeholder="Enter filename (e.g., myfile.txt)"
          />
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCreateFileCancel}>
            Cancel
          </Button>
          <Button 
            onClick={handleCreateFileSubmit}
            variant="contained"
            disabled={!fileName.trim() || uploadFilesMutation.isPending}
          >
            {uploadFilesMutation.isPending ? 'Creating...' : 'Create'}
          </Button>
        </DialogActions>
      </DarkDialog>
    </SlideMenuContainer>
  )
}

// Wrapper component that provides the sidebar context
const SidebarContent: React.FC<{
  showTopLinks?: boolean,
  menuType: string,
  children: ReactNode,
  userMenuHeight?: number,
}> = ({
  children,
  showTopLinks = true,
  menuType,
  userMenuHeight = 0,
}) => {
  return (
    <SidebarProvider userMenuHeight={userMenuHeight}>
      <SidebarContentInner
        showTopLinks={showTopLinks}
        menuType={menuType}
      >
        {children}
      </SidebarContentInner>
    </SidebarProvider>
  )
}

// Main Sidebar component that determines which menuType to use
const Sidebar: React.FC<{
  showTopLinks?: boolean,
  children: ReactNode,
  userMenuHeight?: number,
}> = ({
  children,
  showTopLinks = true,
  userMenuHeight = 0,
}) => {
  const router = useRouter()
  
  // Determine the menu type based on the current route
  const menuType = router.meta.menu || router.params.resource_type || 'chat'
  
  return (
    <SidebarContent 
      showTopLinks={showTopLinks}
      menuType={menuType}
      userMenuHeight={userMenuHeight}
    >
      {children}
    </SidebarContent>
  )
}

export default Sidebar
