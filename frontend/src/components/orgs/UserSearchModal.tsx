import React, { FC, useState, useCallback, useEffect, useMemo, ReactNode } from 'react'
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
import Alert from '@mui/material/Alert'
import Box from '@mui/material/Box'
import Typography from '@mui/material/Typography'
import PersonIcon from '@mui/icons-material/Person'
import MailOutlineIcon from '@mui/icons-material/MailOutline'
import InputAdornment from '@mui/material/InputAdornment'
import SearchIcon from '@mui/icons-material/Search'
import Tooltip from '@mui/material/Tooltip'

import useApi from '../../hooks/useApi'
import useDebounce from '../../hooks/useDebounce'
import useOrganizations from '../../hooks/useOrganizations'
import useAccount from '../../hooks/useAccount'
import { isUserMemberOfOrganization } from '../../utils/organizations'

import { TypesOrgUserLookupResponse, TypesUser } from '../../api/api'

const EMAIL_REGEX = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

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
  // When the typed query is a complete email and the search returned no
  // hits, we hit the org user-lookup endpoint to learn whether the email
  // belongs to an existing Helix account (outside this org) or doesn't
  // exist at all. This drives the CTA label so the admin knows whether
  // they're adding an existing user or sending an invitation.
  const [emailLookup, setEmailLookup] = useState<TypesOrgUserLookupResponse | null>(null)
  const [lookingUpEmail, setLookingUpEmail] = useState(false)

  // Get the debounced search query to avoid excessive API calls
  const debouncedSearchQuery = useDebounce(searchQuery, 300)

  // Get the searchUsers function from useOrganizations hook
  const { searchUsers } = useOrganizations()
  const account = useAccount()
  const api = useApi()
  
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
    setEmailLookup(null)
    setLookingUpEmail(false)
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

  // Org-scoped lookup when the user has typed a complete email. Skip if
  // the email already appears in the search results — we know they exist
  // and are an org member in that case. Otherwise the lookup tells us
  // whether to render "Add to organization" or "Send invitation".
  const orgId = account.organizationTools.organization?.id
  useEffect(() => {
    const trimmed = debouncedSearchQuery.trim()
    if (!open || !orgId || !EMAIL_REGEX.test(trimmed)) {
      setEmailLookup(null)
      setLookingUpEmail(false)
      return
    }
    const lower = trimmed.toLowerCase()
    if (searchResults.some(u => (u.email || '').toLowerCase() === lower)) {
      setEmailLookup({ email: lower, exists: true, is_member: true })
      setLookingUpEmail(false)
      return
    }

    let cancelled = false
    setLookingUpEmail(true)
    api.getApiClient().v1OrganizationsUsersLookupDetail(orgId, { email: trimmed })
      .then((resp) => {
        if (cancelled) return
        setEmailLookup(resp.data || { email: lower, exists: false, is_member: false })
      })
      .catch((err) => {
        console.error('Failed to look up user by email:', err)
        if (!cancelled) {
          // Soft-fail to "doesn't exist" — server will validate again.
          setEmailLookup({ email: lower, exists: false, is_member: false })
        }
      })
      .finally(() => {
        if (!cancelled) setLookingUpEmail(false)
      })
    return () => { cancelled = true }
  }, [api.getApiClient, debouncedSearchQuery, open, orgId, searchResults])

  // Derive what the "invite/add by email" row should look like. Only
  // active when the input is a complete email AND in-org search missed.
  // The `already_invited` state takes precedence over `not_helix` —
  // someone might have an account by now but the invitation is still
  // pending, and we don't want to spam a duplicate.
  type EmailState = 'loading' | 'not_helix' | 'not_in_org' | 'in_org' | 'already_invited'
  const emailEntry = useMemo<{ state: EmailState; label: string; helper: string } | null>(() => {
    const trimmed = debouncedSearchQuery.trim()
    if (!EMAIL_REGEX.test(trimmed)) return null
    if (searchResults.some(u => (u.email || '').toLowerCase() === trimmed.toLowerCase())) {
      return null // user already in search list — no separate "invite" entry needed
    }
    if (lookingUpEmail || !emailLookup) {
      return { state: 'loading', label: 'Checking…', helper: 'Checking whether this email already has a Helix account…' }
    }
    if (emailLookup.is_invited) {
      return { state: 'already_invited', label: 'Invitation sent', helper: 'An invitation has already been sent to this email. They will join the organization automatically when they register.' }
    }
    if (!emailLookup.exists) {
      return { state: 'not_helix', label: 'Send invitation', helper: 'No Helix account exists for this email. We\'ll email them an invitation — they\'ll join the org automatically when they register.' }
    }
    if (emailLookup.is_member) {
      return { state: 'in_org', label: 'Already a member', helper: 'This user is already in the organization.' }
    }
    return { state: 'not_in_org', label: 'Add to organization', helper: 'This user already has a Helix account. Adding them will make them a member of this organization.' }
  }, [debouncedSearchQuery, searchResults, lookingUpEmail, emailLookup])

  // Trigger the invite/add-by-email action. We pass the email through —
  // useOrganizations.addMemberToOrganization passes it as `user_reference`
  // and the backend creates a membership or an invitation accordingly.
  const handleInviteByEmail = useCallback(() => {
    const email = debouncedSearchQuery.trim()
    if (!email || !EMAIL_REGEX.test(email)) return
    onAddMember(email)
    onClose()
  }, [debouncedSearchQuery, onAddMember, onClose])
  
  // Get the results message
  const getResultsMessage = () => {
    if (searchResults.length === 0 && !searching && debouncedSearchQuery.length >= 2) {
      if (emailEntry) {
        // We've detected a typed email; the dedicated email-entry row
        // below describes the action — keep the top message neutral so
        // it doesn't contradict.
        return `${ messagePrefix ? messagePrefix + ' ' : '' }No existing members match — see the option below.`
      }
      return `${ messagePrefix ? messagePrefix + ' ' : '' }No users found. Try a different search term, or type a full email to invite someone.`;
    } else if (searchResults.length > 0) {
      const resultsText = `Found ${searchResults.length} user${searchResults.length === 1 ? '' : 's'}`;
      return messagePrefix ? `${messagePrefix} ${resultsText}` : resultsText;
    } else {
      return 'Enter at least 2 characters to search — or a full email address to invite someone.';
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
          {emailEntry && (
            <ListItem divider>
              <Box sx={{ display: 'flex', alignItems: 'center', mr: 2 }}>
                {emailEntry.state === 'not_helix'
                  ? <MailOutlineIcon color="action" />
                  : <PersonIcon color="action" />}
              </Box>
              <ListItemText
                primary={debouncedSearchQuery.trim()}
                secondary={emailEntry.helper}
              />
              <ListItemSecondaryAction>
                <Button
                  variant="contained"
                  color="primary"
                  size="small"
                  onClick={handleInviteByEmail}
                  disabled={emailEntry.state === 'loading' || emailEntry.state === 'in_org' || emailEntry.state === 'already_invited'}
                >
                  {emailEntry.label}
                </Button>
              </ListItemSecondaryAction>
            </ListItem>
          )}
        </List>

        {emailEntry?.state === 'not_helix' && (
          <Alert severity="info" sx={{ mt: 1 }}>
            They'll receive an email with a link to create their account. As soon as they register, they'll automatically appear here as a member of the organization.
          </Alert>
        )}
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