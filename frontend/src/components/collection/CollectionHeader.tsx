import React, { FC, useState, useEffect } from 'react'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import IconButton from '@mui/material/IconButton'
import EditIcon from '@mui/icons-material/Edit'
import SaveIcon from '@mui/icons-material/Save'
import TextField from '@mui/material/TextField'
import Row from '../widgets/Row'
import Cell from '../widgets/Cell'
import MenuIcon from '@mui/icons-material/Menu'

export const CollectionHeader: FC<{
  collection: any,
  onOpenMobileMenu?: () => void,
}> = ({
  collection,
  onOpenMobileMenu,
}) => {
  const [editingCollection, setEditingCollection] = useState(false)
  const [collectionName, setCollectionName] = useState(collection.name)

  useEffect(() => {
    setCollectionName(collection.name)
  }, [collection.name])

  const handleCollectionNameChange = (event: React.ChangeEvent<HTMLInputElement>) => {
    setCollectionName(event.target.value)
  }

  const handleCollectionNameSubmit = () => {
    // Simulate updating collection name in fixture data
    console.log(`Collection name updated to: ${collectionName}`)
    setEditingCollection(false)
  }

  return (
    <Row
      sx={{
        height: '78px',
      }}
    >
      <IconButton
        onClick={onOpenMobileMenu}
        size="large"
        edge="start"
        color="inherit"
        aria-label="menu"
        sx={{ mr: 2, display: { sm: 'block', md: 'none' } }}
      >
        <MenuIcon />
      </IconButton>
      <Cell flexGrow={1}>
        <Box
          sx={{
            display: 'flex',
            flexDirection: 'column',
            justifyContent: 'center'
          }}
        >
          <Box
            sx={{
              display: 'flex',
              alignItems: 'center'
            }}
          >
            {editingCollection ? (
              <Box sx={{ display: 'flex', alignItems: 'center' }}>
                <TextField
                  size="small"
                  value={collectionName}
                  onChange={handleCollectionNameChange}
                  autoFocus
                  fullWidth
                  sx={{
                    mr: 1,
                  }}
                />
                <IconButton
                  onClick={handleCollectionNameSubmit}
                  size="small"
                  sx={{ ml: 1 }}
                >
                  <SaveIcon />
                </IconButton>
              </Box>
            ) : (
              <>
                <Typography
                  component="h1"
                  sx={{
                    fontSize: { xs: 'small', sm: 'medium', md: 'large' },
                    whiteSpace: 'nowrap',
                    overflow: 'hidden',
                    textOverflow: 'ellipsis',
                    maxWidth: '22ch',
                  }}
                >
                  {collection.name}
                </Typography>
                <IconButton
                  onClick={() => setEditingCollection(true)}
                  size="small"
                  sx={{ ml: 1 }}
                >
                  <EditIcon />
                </IconButton>
              </>
            )}
          </Box>
          <Typography variant="caption" sx={{ color: 'gray' }}>
            Created on {new Date(collection.created).toLocaleDateString()} {/* Adjust date formatting as needed */}
          </Typography>
        </Box>
      </Cell>
      <Cell>
        <Box sx={{ display: { xs: 'block', sm: 'flex' }, alignItems: 'center' }}>
          {/* Collection specific actions and information can be added here */}
        </Box>
      </Cell>
    </Row>
  )
}

export default CollectionHeader
