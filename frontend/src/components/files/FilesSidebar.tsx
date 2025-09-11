import { FC, Fragment, useState, useCallback, useEffect, useRef } from 'react'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemButton from '@mui/material/ListItemButton'
import ListItemIcon from '@mui/material/ListItemIcon'
import ListItemText from '@mui/material/ListItemText'
import CircularProgress from '@mui/material/CircularProgress'
import Typography from '@mui/material/Typography'
import Tooltip from '@mui/material/Tooltip'
import Box from '@mui/material/Box'

import FolderIcon from '@mui/icons-material/Folder'
import InsertDriveFileIcon from '@mui/icons-material/InsertDriveFile'
import ImageIcon from '@mui/icons-material/Image'
import DescriptionIcon from '@mui/icons-material/Description'
import VideoFileIcon from '@mui/icons-material/VideoFile'
import AudioFileIcon from '@mui/icons-material/AudioFile'
import CodeIcon from '@mui/icons-material/Code'
import ArchiveIcon from '@mui/icons-material/Archive'

import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import ClickLink from '../widgets/ClickLink'
import SlideMenuContainer from '../system/SlideMenuContainer'

import useRouter from '../../hooks/useRouter'
import useLightTheme from '../../hooks/useLightTheme'
import useAccount from '../../hooks/useAccount'
import { useListFilestore } from '../../services/filestoreService'
import { FilestoreItem } from '../../api/api'

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
  const [currentPage, setCurrentPage] = useState(0)
  const [allFiles, setAllFiles] = useState<FilestoreItem[]>([])
  const [hasMore, setHasMore] = useState(true)
  const [totalCount, setTotalCount] = useState(0)

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
    </SlideMenuContainer>
  )
}

export default FilesSidebar
