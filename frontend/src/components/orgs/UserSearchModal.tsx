import React, { FC, useState, useCallback, useEffect, ReactNode } from 'react'
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
import Tooltip from '@mui/material/Tooltip'

import useDebounce from '../../hooks/useDebounce'
import useOrganizations from '../../hooks/useOrganizations'
import useAccount from '../../hooks/useAccount'
import { isUserMemberOfOrganization } from '../../utils/organizations'

import { TypesUser } from '../../api/api'

// Interface for component props
interface UserSearchModalProps {
  open: boolean
  onClose: () => void
  onAddMember: (userId: string) => void
  title?: string
  messagePrefix?: string
  organizationMembersOnly?: boolean
  existingMembers?: TypesUser[]
}

// UserSearchModal component for searching and adding users to an organization or team
const UserSearchModal: FC<UserSearchModalProps> = ({ 
  open, 
  onClose, 
  onAddMember, 
  title = "Add Organization Member",
  messagePrefix,
  organizationMembersOnly = false,
  existingMembers = []
}) => {
  // State for the search query
  const [searchQuery, setSearchQuery] = useState('')
  const [searching, setSearching] = useState(false)
  const [searchResults, setSearchResults] = useState<TypesUser[]>([])
  
  // Get the debounced search query to avoid excessive API calls
  const debouncedSearchQuery = useDebounce(searchQuery, 300)
  
  // Get the searchUsers function from useOrganizations hook
  const { searchUsers } = useOrganizations()
  const account = useAccount()
  
  // Check if a user is already a member of the organization
  const isUserAlreadyMember = useCallback((user: TypesUser) => {
    return existingMembers.some(member => member.id === user.id)
  }, [existingMembers])
  
  // Handle search query change
  const handleSearchChange = useCallback((event: React.ChangeEvent<HTMLInputElement>) => {
    setSearchQuery(event.target.value)
  }, [])
  
  // Handle adding a user
  const handleAddUser = useCallback((user: TypesUser) => {    
    onAddMember(user.id || '')
    onClose()
  }, [onAddMember, onClose])

  useEffect(() => {
    setSearchQuery('')
  }, [open])
  
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
          query: debouncedSearchQuery,
          organizationId: organizationMembersOnly ? account.organizationTools.organization?.id : undefined
        })

        // If users is an empty array, thing to return
        if (response.users?.length === 0) {
          setSearchResults([])
          return
        }
        
        // Normalize the case of API response fields
        const normalizedResults = response.users?.map((user: TypesUser) => {
          // Use type assertion to access capitalized properties
          
          return {
            id: user.id,
            email: user.email || '',
            full_name: user.full_name || '',
            username: user.username || ''
          };
        });
        
        setSearchResults(normalizedResults || [])
      } catch (error) {
        console.error('Error searching users:', error)
        setSearchResults([])
      } finally {
        setSearching(false)
      }
    }
    
    performSearch()
  }, [debouncedSearchQuery])
  
  // Get the results message
  const getResultsMessage = () => {
    if (searchResults.length === 0 && !searching && debouncedSearchQuery.length >= 2) {
      return `${ messagePrefix ? messagePrefix + ' ' : '' }No users found. Try a different search term.`;
    } else if (searchResults.length > 0) {
      const resultsText = `Found ${searchResults.length} user${searchResults.length === 1 ? '' : 's'}`;
      return messagePrefix ? `${messagePrefix} ${resultsText}` : resultsText;
    } else {
      return 'Enter at least 2 characters to search';
    }
  };

  return (
    <Dialog open={open} onClose={onClose} maxWidth="sm" fullWidth>
      <DialogTitle>{title}</DialogTitle>
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
            {getResultsMessage()}
          </Typography>
        </Box>
        
        <List sx={{ width: '100%' }}>
          {searchResults.map((user) => {
            const alreadyMember = isUserAlreadyMember(user)
            return (
              <ListItem key={user.id} divider>
                <Box sx={{ display: 'flex', alignItems: 'center', mr: 2 }}>
                  <PersonIcon color="action" />
                </Box>
                <ListItemText
                  primary={user.full_name || 'Unnamed User'}
                  secondary={user.email}
                />
                <ListItemSecondaryAction>
                  <Tooltip title={alreadyMember ? "User is already a member of this organization" : ""}>
                    <span>
                      <Button
                        variant="contained"
                        color="primary"
                        size="small"
                        onClick={() => handleAddUser(user)}
                        disabled={alreadyMember}
                      >
                        {alreadyMember ? 'Already Member' : 'Add'}
                      </Button>
                    </span>
                  </Tooltip>
                </ListItemSecondaryAction>
              </ListItem>
            )
          })}
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