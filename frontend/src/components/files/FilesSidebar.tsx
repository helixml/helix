import { FC, useState, useCallback } from 'react'
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
  Home 
} from 'lucide-react'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import SlideMenuContainer from '../system/SlideMenuContainer'

import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useAccount from '../../hooks/useAccount'
import { useListFilestore } from '../../services/filestoreService'
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
  
  // State for current directory navigation
  const [currentPath, setCurrentPath] = useState<string>('')
  const [pathHistory, setPathHistory] = useState<string[]>([])

  const {
    data: filesData,
    isLoading: isLoadingFiles,
    error
  } = useListFilestore(
    currentPath, // List current directory
    !!account.user?.id // Only load if logged in
  )
  
  const {
    params,
  } = useRouter()

  // Navigation functions
  const navigateToDirectory = useCallback((path: string) => {
    setPathHistory(prev => [...prev, currentPath])
    setCurrentPath(path)
  }, [currentPath])

  const navigateBack = useCallback(() => {
    if (pathHistory.length > 0) {
      const previousPath = pathHistory[pathHistory.length - 1]
      setPathHistory(prev => prev.slice(0, -1))
      setCurrentPath(previousPath)
    }
  }, [pathHistory])

  const navigateToRoot = useCallback(() => {
    setPathHistory([])
    setCurrentPath('')
  }, [])

  const navigateToPath = useCallback((path: string) => {
    setPathHistory([])
    setCurrentPath(path)
  }, [])

  // Get breadcrumb segments
  const getBreadcrumbSegments = useCallback(() => {
    if (!currentPath) return [{ name: 'home', path: '' }]
    
    const segments = currentPath.split('/').filter(Boolean)
    const breadcrumbs = [{ name: 'home', path: '' }]
    
    let currentBreadcrumbPath = ''
    segments.forEach(segment => {
      currentBreadcrumbPath = currentBreadcrumbPath ? `${currentBreadcrumbPath}/${segment}` : segment
      breadcrumbs.push({
        name: segment,
        path: currentBreadcrumbPath
      })
    })
    
    return breadcrumbs
  }, [currentPath])

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
            // Navigate into directory
            const fileName = file.name || ''
            const newPath = currentPath ? `${currentPath}/${fileName}` : fileName
            navigateToDirectory(newPath)
          } else {
            // Open file - set file_path in URL query
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
        {(currentPath || pathHistory.length > 0) && (
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1, mb: 1, ml: 1 }}>
            {pathHistory.length > 0 && (
              <IconButton 
                size="small" 
                onClick={navigateBack}
                sx={{ color: lightTheme.textColorFaded }}
              >
                <ArrowLeft size={16} />
              </IconButton>
            )}
            {currentPath && (
              <IconButton 
                size="small" 
                onClick={navigateToRoot}
                sx={{ color: lightTheme.textColorFaded }}
              >
                <Home size={16} />
              </IconButton>
            )}
          </Box>
        )}
        
        {currentPath && (
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
    </SlideMenuContainer>
  )
}

export default FilesSidebar
