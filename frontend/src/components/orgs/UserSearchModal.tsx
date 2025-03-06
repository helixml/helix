import React, { FC, useState, useCallback, useEffect } from 'react'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import TextField from '@mui/material/TextField'
import Button from '@mui/material/Button'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import CircularProgress from '@mui/material/CircularProgress'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import PersonIcon from '@mui/icons-material/Person'
import InputAdornment from '@mui/material/InputAdornment'
import SearchIcon from '@mui/icons-material/Search'

import useDebounce from '../../hooks/useDebounce'
import useOrganizations, { UserSearchResult } from '../../hooks/useOrganizations'

// Interface for component props
interface UserSearchModalProps {
  open: boolean
  onClose: () => void
  onAddMember: (userId: string) => void
}

// UserSearchModal component for searching and adding users to an organization
const UserSearchModal: FC<UserSearchModalProps> = ({ open, onClose, onAddMember }) => {
  // State for the search query
  const [searchQuery, setSearchQuery] = useState('')
  const [searching, setSearching] = useState(false)
  const [searchResults, setSearchResults] = useState<UserSearchResult[]>([])
  
  // Get the debounced search query to avoid excessive API calls
  const debouncedSearchQuery = useDebounce(searchQuery, 300)
  
  // Get the searchUsers function from useOrganizations hook
  const { searchUsers } = useOrganizations()
  
  // Handle search query change
  const handleSearchChange = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
    setSearchQuery(event.target.value)
  }, [])
  
  // Handle adding a user
  const handleAddUser = useCallback((user: UserSearchResult) => {
    onAddMember(user.id)
    onClose()
  }, [onAddMember, onClose])
  
  // Perform search when debounced query changes
  useEffect(() => {
    const performSearch = async () => {
      if (!debouncedSearchQuery || debouncedSearchQuery.length < 2) {
        setSearchResults([])
        return
      }
      
      setSearching(true)
      try {
        // Search by both email and name
        const response = await searchUsers({
          email: debouncedSearchQuery,
          name: debouncedSearchQuery,
        })
        
        setSearchResults(response.users)
      } catch (error) {
        console.error('Error searching users:', error)
        setSearchResults([])
      } finally {
        setSearching(false)
      }
    }
    
    performSearch()
  }, [debouncedSearchQuery])
  
  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>Add Organization Member</DialogTitle>
      <DialogContent>
        <TextField
          autoFocus
          margin="dense"
          id="search"
          label="Search by name or email"
          type="text"
          fullWidth
          variant="outlined"
          value={searchQuery}
          onChange={handleSearchChange}
          InputProps={{
            startAdornment: (
              <InputAdornment position="start">
                <SearchIcon color="action" />
              </InputAdornment>
            ),
            endAdornment: searching ? (
              <InputAdornment position="end">
                <CircularProgress size={20} />
              </InputAdornment>
            ) : null,
          }}
        />
        
        <Box sx={{ mt: 2, mb: 1 }}>
          <Typography variant="subtitle2" color="text.secondary">
            {searchResults.length === 0 && !searching && debouncedSearchQuery.length >= 2
              ? 'No users found. Try a different search term.'
              : searchResults.length > 0
                ? `Found ${searchResults.length} users`
                : 'Enter at least 2 characters to search'}
          </Typography>
        </Box>
        
        <List sx={{ width: '100%' }}>
          {searchResults.map((user) => (
            <ListItem key={user.id} divider>
              <Box sx={{ display: 'flex', alignItems: 'center', mr: 2 }}>
                <PersonIcon color="action" />
              </Box>
              <ListItemText
                primary={user.fullName || 'Unnamed User'}
                secondary={user.email}
              />
              <ListItemSecondaryAction>
                <Button
                  variant="contained"
                  color="primary"
                  size="small"
                  onClick={() => handleAddUser(user)}
                >
                  Add
                </Button>
              </ListItemSecondaryAction>
            </ListItem>
          ))}
        </List>
      </DialogContent>
      <DialogActions>
        <Button onClick={onClose} color="primary">
          Cancel
        </Button>
      </DialogActions>
    </Dialog>
  )
}

export default UserSearchModal 