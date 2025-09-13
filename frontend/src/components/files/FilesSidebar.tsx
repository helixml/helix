import { FC, useState, useCallback, useEffect } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'
import Breadcrumbs from '@mui/material/Breadcrumbs'
import Link from '@mui/material/Link'
import Box from '@mui/material/Box'
import IconButton from '@mui/material/IconButton'
import Menu from '@mui/material/Menu'
import MenuItem from '@mui/material/MenuItem'

import { 
  Folder, 
  File, 
  Image, 
  FileText, 
  Video, 
  Music, 
  Code, 
  Archive, 
  ArrowLeft, 
  Home,
  MoreVertical,
  Trash2
} from 'lucide-react'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import SlideMenuContainer from '../system/SlideMenuContainer'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'

import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import { useListFilestore, useFilestoreConfig, useDeleteFilestoreItem } from '../../services/filestoreService'
import { FilestoreItem } from '../../api/api'

// Menu identifier constant
const MENU_TYPE = 'files'

export const FilesSidebar: FC<{
  onOpenFile: () => void,
}> = ({
  onOpenFile,
}) => {
  const account = useAccount()
  const router = useRouter()
  const lightTheme = useLightTheme()
  const snackbar = useSnackbar()
  
  // Get current directory and selected file from URL parameters
  const currentDirectory = router.params.directory || ''
  const selectedFilePath = router.params.file_path || ''

  const {
    data: filestoreConfig,
  } = useFilestoreConfig()
  
  const {
    data: filesData,
    isLoading: isLoadingFiles,
    error
  } = useListFilestore(
    currentDirectory, // List current directory
    !!account.user?.id // Only load if logged in
  )

  // Delete functionality
  const deleteFilestoreItem = useDeleteFilestoreItem()

  // Menu state
  const [menuAnchorEl, setMenuAnchorEl] = useState<null | HTMLElement>(null)
  const [selectedFile, setSelectedFile] = useState<FilestoreItem | null>(null)
  const [deleteDialogOpen, setDeleteDialogOpen] = useState(false)

  // Navigation functions using URL parameters
  const navigateToDirectory = useCallback((path: string) => {
    router.setParams({ directory: path, file_path: '' })
  }, [router])

  const navigateBack = useCallback(() => {
    // Use browser's back button functionality
    window.history.back()
  }, [])

  const navigateToRoot = useCallback(() => {
    router.setParams({ directory: '', file_path: '' })
  }, [router])

  const navigateToPath = useCallback((path: string) => {
    router.setParams({ directory: path, file_path: '' })
  }, [router])

  // Menu handling functions
  const handleMenuOpen = useCallback((event: React.MouseEvent<HTMLElement>, file: FilestoreItem) => {
    event.stopPropagation() // Prevent file selection when clicking menu
    setMenuAnchorEl(event.currentTarget)
    setSelectedFile(file)
  }, [])

  const handleMenuClose = useCallback(() => {
    setMenuAnchorEl(null)
  }, [])

  const handleDeleteClick = useCallback(() => {
    setDeleteDialogOpen(true)
    handleMenuClose()
  }, [handleMenuClose])

  const handleDeleteConfirm = useCallback(async () => {
    if (selectedFile) {
      let filePath = selectedFile.path || selectedFile.name || ''
      // Remove the user prefix if it exists
      if (filestoreConfig?.user_prefix && filePath.startsWith(filestoreConfig.user_prefix)) {
        filePath = filePath.substring(filestoreConfig.user_prefix.length)
      }
      try {        

        await deleteFilestoreItem.mutateAsync(filePath)
        snackbar.success(`${selectedFile.directory ? 'Folder' : 'File'} deleted successfully`)
        setDeleteDialogOpen(false)
        setSelectedFile(null)
      } catch (error) {
        console.error('Failed to delete file:', error)
        snackbar.error(`Failed to delete ${selectedFile.directory ? 'folder' : 'file'}`)
      }
    }
  }, [selectedFile, deleteFilestoreItem, snackbar])

  const handleDeleteCancel = useCallback(() => {
    setDeleteDialogOpen(false)
    setSelectedFile(null)
  }, [])

  // Get breadcrumb segments - only show directory path, not selected file
  const getBreadcrumbSegments = useCallback(() => {
    if (!currentDirectory) return [{ name: 'home', path: '' }]
    
    // Remove user prefix from the path for display purposes
    let displayPath = currentDirectory
    if (filestoreConfig?.user_prefix && currentDirectory.startsWith(filestoreConfig.user_prefix)) {
      displayPath = currentDirectory.substring(filestoreConfig.user_prefix.length)
      // Remove leading slash if present
      if (displayPath.startsWith('/')) {
        displayPath = displayPath.substring(1)
      }
    }
    
    const segments = displayPath.split('/').filter(Boolean)
    const breadcrumbs = [{ name: 'home', path: '' }]
    
    // For directories, show the full path in breadcrumbs
    let currentBreadcrumbPath = ''
    segments.forEach(segment => {
      currentBreadcrumbPath = currentBreadcrumbPath ? `${currentBreadcrumbPath}/${segment}` : segment
      // Reconstruct the full path including user prefix for navigation
      const fullPath = filestoreConfig?.user_prefix 
        ? `${filestoreConfig.user_prefix}/${currentBreadcrumbPath}`.replace(/\/+/g, '/')
        : currentBreadcrumbPath
      
      breadcrumbs.push({
        name: segment,
        path: fullPath
      })
    })
    
    return breadcrumbs
  }, [currentDirectory, filestoreConfig?.user_prefix])

  const getFileIcon = (file: FilestoreItem) => {
    if (file.directory) {
      return <Folder size={20} color="#00E5FF" />
    }

    const fileName = file.name || ''
    const extension = fileName.split('.').pop()?.toLowerCase()

    // Image files
    if (['jpg', 'jpeg', 'png', 'gif', 'bmp', 'svg', 'webp'].includes(extension || '')) {
      return <Image size={20} color="#00E5FF" />
    }

    // Video files
    if (['mp4', 'avi', 'mov', 'wmv', 'flv', 'webm', 'mkv'].includes(extension || '')) {
      return <Video size={20} color="#00E5FF" />
    }

    // Audio files
    if (['mp3', 'wav', 'flac', 'aac', 'ogg', 'm4a'].includes(extension || '')) {
      return <Music size={20} color="#00E5FF" />
    }

    // Archive files
    if (['zip', 'rar', '7z', 'tar', 'gz', 'bz2'].includes(extension || '')) {
      return <Archive size={20} color="#00E5FF" />
    }

    // Code files
    if (['js', 'ts', 'jsx', 'tsx', 'py', 'java', 'cpp', 'c', 'h', 'go', 'rs', 'php', 'rb', 'swift', 'kt', 'scala', 'sh', 'bash', 'ps1', 'bat', 'cmd'].includes(extension || '')) {
      return <Code size={20} color="#00E5FF" />
    }

    // Document files
    if (['pdf', 'doc', 'docx', 'txt', 'rtf', 'odt', 'pages'].includes(extension || '')) {
      return <FileText size={20} color="#00E5FF" />
    }

    // Default file icon
    return <File size={20} color="#00E5FF" />
  }


  const renderFile = (file: FilestoreItem) => {
    const filePath = file.path || file.name || ''
    const isActive = filePath === selectedFilePath
    return (
      <ListItem
        sx={{
          borderRadius: '20px',
          cursor: 'pointer',
          width: '100%',
          padding: 0,
          display: 'flex',
          alignItems: 'center',
        }}
        key={filePath}
        onClick={() => {
          if (file.directory) {
            // Navigate into directory
            const fileName = file.name || ''
            const newPath = currentDirectory ? `${currentDirectory}/${fileName}` : fileName
            navigateToDirectory(newPath)
          } else {
            // Open file - set file_path in URL query but keep current directory
            router.setParams({ file_path: filePath })
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
            flex: 1,
            mr: 0.5,
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
        <IconButton
          size="small"
          onClick={(e) => handleMenuOpen(e, file)}
          sx={{
            color: lightTheme.textColorFaded,
            opacity: 0.7,
            '&:hover': {
              opacity: 1,
              color: '#fff',
            },
          }}
        >
          <MoreVertical size={16} />
        </IconButton>
      </ListItem>
    )
  }


  // Show loading state for initial load
  if (isLoadingFiles) {
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
      {/* Navigation Header */}
      <Box sx={{ p: 2, borderBottom: '1px solid', borderColor: 'divider' }}>
        <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1, ml: 1 }}>
          <IconButton 
            size="small" 
            onClick={navigateBack}
            sx={{ color: lightTheme.textColorFaded }}
            title="Go back"
          >
            <ArrowLeft size={16} />
          </IconButton>
          {currentDirectory && (
            <IconButton 
              size="small" 
              onClick={navigateToRoot}
              sx={{ color: lightTheme.textColorFaded }}
              title="Go to root"
            >
              <Home size={16} />
            </IconButton>
          )}
        </Box>
        
        {currentDirectory && (
          <Breadcrumbs 
            separator="/" 
            sx={{
              ml: 2,
              '& .MuiBreadcrumbs-separator': { 
                color: lightTheme.textColorFaded 
              } 
            }}
          >
            {getBreadcrumbSegments().map((segment, index) => (
              <Link
                key={segment.path}
                component="button"
                variant="body2"
                onClick={() => navigateToPath(segment.path)}
                sx={{
                  color: index === getBreadcrumbSegments().length - 1 
                    ? lightTheme.textColor 
                    : lightTheme.textColorFaded,
                  textDecoration: 'none',
                  cursor: 'pointer',
                  '&:hover': {
                    textDecoration: 'underline',
                  },
                  fontSize: '0.875rem',
                }}
              >
                {segment.name}
              </Link>
            ))}
          </Breadcrumbs>
        )}
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
        {(filesData || []).map(renderFile)}
      </List>

      {/* Context Menu */}
      <Menu
        anchorEl={menuAnchorEl}
        open={Boolean(menuAnchorEl)}
        onClose={handleMenuClose}
        anchorOrigin={{
          vertical: 'bottom',
          horizontal: 'right',
        }}
        transformOrigin={{
          vertical: 'top',
          horizontal: 'right',
        }}
        sx={{
          '& .MuiPaper-root': {
            backgroundColor: '#1a1a2f',
            border: '1px solid #333',
            borderRadius: '8px',
          },
        }}
      >
        <MenuItem
          onClick={handleDeleteClick}
          sx={{
            color: '#ff6b6b',
            '&:hover': {
              backgroundColor: '#2a1a1a',
            },
          }}
        >
          <Trash2 size={16} style={{ marginRight: '8px' }} />
          Delete
        </MenuItem>
      </Menu>

      {/* Delete Confirmation Dialog */}
      <DeleteConfirmWindow
        open={deleteDialogOpen}
        title={selectedFile?.directory ? `folder "${selectedFile.name}"` : `file "${selectedFile?.name}"`}
        confirmString="delete"
        onCancel={handleDeleteCancel}
        onSubmit={handleDeleteConfirm}
      />
    </SlideMenuContainer>
  )
}

export default FilesSidebar
