import React, { useState, useEffect, useMemo, useCallback } from 'react'
import Box from '@mui/material/Box'
import Button from '@mui/material/Button'
import Typography from '@mui/material/Typography'
import Chip from '@mui/material/Chip'
import Dialog from '@mui/material/Dialog'
import DialogTitle from '@mui/material/DialogTitle'
import DialogContent from '@mui/material/DialogContent'
import DialogActions from '@mui/material/DialogActions'
import FormControl from '@mui/material/FormControl'
import InputLabel from '@mui/material/InputLabel'
import Select from '@mui/material/Select'
import MenuItem from '@mui/material/MenuItem'
import CircularProgress from '@mui/material/CircularProgress'
import IconButton from '@mui/material/IconButton'
import Tooltip from '@mui/material/Tooltip'
import RadioGroup from '@mui/material/RadioGroup'
import FormControlLabel from '@mui/material/FormControlLabel'
import Radio from '@mui/material/Radio'
import FormLabel from '@mui/material/FormLabel'
import Link from '@mui/material/Link'
import TextField from '@mui/material/TextField'
import Alert from '@mui/material/Alert'
import Snackbar from '@mui/material/Snackbar'
import List from '@mui/material/List'
import ListItem from '@mui/material/ListItem'
import ListItemText from '@mui/material/ListItemText'
import ListItemSecondaryAction from '@mui/material/ListItemSecondaryAction'
import InputAdornment from '@mui/material/InputAdornment'
// Import icons
import AddIcon from '@mui/icons-material/Add'
import DeleteIcon from '@mui/icons-material/Delete'
import PersonIcon from '@mui/icons-material/Person'
import GroupsIcon from '@mui/icons-material/Groups'
import AdminPanelSettingsIcon from '@mui/icons-material/AdminPanelSettings'
import LockPersonIcon from '@mui/icons-material/LockPerson'
import SearchIcon from '@mui/icons-material/Search'
import useAccount from '../../hooks/useAccount'
import useApi from '../../hooks/useApi'
import useLightTheme from '../../hooks/useLightTheme'
import useDebounce from '../../hooks/useDebounce'
import { extractErrorMessage } from '../../hooks/useErrorCallback'
import { TypesAccessGrant, TypesCreateAccessGrantRequest, TypesCreateAccessGrantResponse, TypesOrganizationInvitation, TypesOrganizationRole, TypesOrgUserLookupResponse, TypesUser } from '../../api/api'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import useRouter from '../../hooks/useRouter'
import useTheme from '@mui/material/styles/useTheme'

interface AccessManagementProps {
  appId: string;
  accessGrants: TypesAccessGrant[];
  isLoading: boolean;
  isReadOnly: boolean;
  organizationId?: string;
  currentUser?: { full_name?: string; email?: string; id?: string; admin?: boolean };
  projectOwnerId?: string;
  onCreateGrant: (request: TypesCreateAccessGrantRequest) => Promise<TypesCreateAccessGrantResponse | null>;
  onDeleteGrant: (grantId: string) => Promise<boolean>;
}

const AccessManagement: React.FC<AccessManagementProps> = ({
  appId,
  accessGrants,
  isLoading,
  isReadOnly,
  organizationId,
  currentUser,
  projectOwnerId,
  onCreateGrant,
  onDeleteGrant
}) => {
  // Get organization data from account context
  const account = useAccount();
  const api = useApi();
  const orgTools = account.organizationTools;
  const organization = orgTools.organization;
  const router = useRouter();
  const theme = useTheme();
  const lightTheme = useLightTheme();

  // Local state
  const [openTeamDialog, setOpenTeamDialog] = useState(false);
  const [openUserDialog, setOpenUserDialog] = useState(false);
  const [selectedTeamId, setSelectedTeamId] = useState('');
  const [selectedUserId, setSelectedUserId] = useState('');
  const [selectedUserReference, setSelectedUserReference] = useState('');
  const [userInputValue, setUserInputValue] = useState('');
  const debouncedUserInputValue = useDebounce(userInputValue, 300);
  const [selectedRole, setSelectedRole] = useState('app_user'); // Default role
  const [deleteGrantId, setDeleteGrantId] = useState<string | null>(null);
  const [orgAddSnackbar, setOrgAddSnackbar] = useState<string | null>(null);
  const [orgAddSnackbarSeverity, setOrgAddSnackbarSeverity] = useState<'info' | 'error'>('info');
  const [userSearchResults, setUserSearchResults] = useState<TypesUser[]>([]);
  const [searchingUsers, setSearchingUsers] = useState(false);
  // emailLookup is populated once the user has typed a full email AND the
  // in-org search returned no results. It drives the button label so the
  // user knows whether confirming will send an invitation, add an existing
  // Helix user to the org, or just grant project access.
  const [emailLookup, setEmailLookup] = useState<TypesOrgUserLookupResponse | null>(null);
  const [lookingUpEmail, setLookingUpEmail] = useState(false);
  // Pending org invitations rendered as placeholder rows in the Users
  // with access table so org admins can see who they've invited but who
  // hasn't joined yet. Refreshed after creating or revoking invitations.
  const [invitations, setInvitations] = useState<TypesOrganizationInvitation[]>([]);
  const [revokingInvitationId, setRevokingInvitationId] = useState<string | null>(null);

  // Extract roles from the organization
  const availableRoles = useMemo(() => {
    return organization?.roles || [];
  }, [organization]);

  // Filter grants into users and teams
  const userGrants = useMemo(() => {
    return accessGrants.filter(grant => grant.user_id && grant.user_id !== currentUser?.id);
  }, [accessGrants, currentUser?.id]);

  const teamGrants = useMemo(() => {
    return accessGrants.filter(grant => grant.team_id);
  }, [accessGrants]);

  // Get organization teams
  const teams = useMemo(() => {
    return organization?.teams || [];
  }, [organization]);

  const ownerDisplayName = currentUser?.full_name || currentUser?.email || 'You';
  const ownerDisplayEmail = currentUser?.email || '-';
  const hasOwnerRow = !!currentUser?.id;
  const effectiveOrganizationId = organizationId || organization?.id || orgTools.orgID;

  const existingGrantUserIds = useMemo(() => {
    return new Set(accessGrants.map(g => g.user_id).filter(Boolean));
  }, [accessGrants]);

  // Fetch pending invitations scoped to THIS app. Org-wide invitations
  // (sent from /orgs/{id}/people) are not surfaced here — they belong to
  // the org members page, not a project's access list. The backend
  // filters via the app_id query param.
  const loadInvitations = useCallback(async () => {
    if (!effectiveOrganizationId) {
      setInvitations([]);
      return;
    }
    try {
      const response = await api.getApiClient().v1OrganizationsInvitationsDetail(effectiveOrganizationId, {
        app_id: appId,
      } as any);
      setInvitations(response.data || []);
    } catch (error) {
      // Non-owners get 403 here, which is fine — they just don't see the
      // pending-invitations section.
      console.error('Failed to load invitations:', error);
      setInvitations([]);
    }
  }, [api.getApiClient, effectiveOrganizationId, appId]);

  useEffect(() => {
    loadInvitations();
  }, [loadInvitations]);

  useEffect(() => {
    if (!openUserDialog) return;

    const query = debouncedUserInputValue.trim();
    if (!effectiveOrganizationId || query.length < 2) {
      setUserSearchResults([]);
      setSearchingUsers(false);
      return;
    }

    let cancelled = false;
    setSearchingUsers(true);

    api.getApiClient().v1UsersSearchList({
      query,
      organization_id: effectiveOrganizationId,
      limit: 20,
    })
      .then((response) => {
        if (cancelled) return;
        const normalizedResults = (response.data?.users || []).map((user: TypesUser) => ({
          id: user.id,
          email: user.email || '',
          full_name: user.full_name || '',
          username: user.username || '',
        }));
        setUserSearchResults(normalizedResults);
      })
      .catch((error) => {
        console.error('Failed to search organization members:', error);
        if (!cancelled) {
          setUserSearchResults([]);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setSearchingUsers(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [api.getApiClient, debouncedUserInputValue, effectiveOrganizationId, openUserDialog]);

  const isValidEmail = (value: string) => /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value.trim());

  // When the typed input is a full email and the in-org search didn't find
  // it, hit the org-scoped lookup to learn whether a Helix account exists
  // for this address (and is just outside the org), or whether we'll need
  // to send an invitation. The org-membership search above won't tell us
  // either of those because it's filtered to org members.
  useEffect(() => {
    if (!openUserDialog || !effectiveOrganizationId) {
      setEmailLookup(null);
      setLookingUpEmail(false);
      return;
    }
    const trimmed = debouncedUserInputValue.trim();
    if (!isValidEmail(trimmed)) {
      setEmailLookup(null);
      setLookingUpEmail(false);
      return;
    }
    // If the in-org search already returned a hit for this email we don't
    // need the lookup — the user is in the org. Save the round trip.
    const lower = trimmed.toLowerCase();
    if (userSearchResults.some(u => (u.email || '').toLowerCase() === lower)) {
      setEmailLookup({ email: lower, exists: true, is_member: true });
      setLookingUpEmail(false);
      return;
    }

    let cancelled = false;
    setLookingUpEmail(true);
    api.getApiClient().v1OrganizationsUsersLookupDetail(effectiveOrganizationId, { email: trimmed })
      .then((response) => {
        if (cancelled) return;
        setEmailLookup(response.data || { email: lower, exists: false, is_member: false });
      })
      .catch((error) => {
        console.error('Failed to look up user by email:', error);
        if (!cancelled) {
          // Soft-fail: assume the user doesn't exist so we offer "Send
          // invitation"; the server will validate again on submit.
          setEmailLookup({ email: lower, exists: false, is_member: false });
        }
      })
      .finally(() => {
        if (!cancelled) setLookingUpEmail(false);
      });

    return () => { cancelled = true; };
  }, [api.getApiClient, debouncedUserInputValue, effectiveOrganizationId, openUserDialog, userSearchResults]);

  const getUserSearchMessage = () => {
    const query = userInputValue.trim();
    if (!effectiveOrganizationId) {
      return 'Select an organization before adding access.';
    }
    if (query.length < 2) {
      return 'Search for an organization member by name or email — or enter a full email to invite someone outside the org.';
    }
    if (searchingUsers) {
      return 'Searching organization members…';
    }
    if (userSearchResults.length > 0) {
      return `Showing users in this organization. Found ${userSearchResults.length} member${userSearchResults.length === 1 ? '' : 's'}.`;
    }
    if (isValidEmail(query)) {
      return 'No organization member matches this email. See the prompt below for what will happen.';
    }
    return 'No organization members found. Try a different search term or enter a full email address.';
  };

  // Filter teams that don't already have access grants
  const availableTeams = useMemo(() => {
    const teamIds = new Set(teamGrants.map(grant => grant.team_id || ''));
    return teams.filter(team => team.id && !teamIds.has(team.id));
  }, [teams, teamGrants]);

  // Handle team dialog open/close
  const handleOpenTeamDialog = () => setOpenTeamDialog(true);
  const handleCloseTeamDialog = () => {
    setOpenTeamDialog(false);
    setSelectedTeamId('');
    setSelectedRole('app_user');
  };

  // Handle user dialog open/close
  const handleOpenUserDialog = () => setOpenUserDialog(true);
  const handleCloseUserDialog = () => {
    setOpenUserDialog(false);
    setSelectedUserId('');
    setSelectedUserReference('');
    setUserInputValue('');
    setUserSearchResults([]);
    setSearchingUsers(false);
    setEmailLookup(null);
    setLookingUpEmail(false);
    setSelectedRole('app_user');
  };

  // Handle create team grant
  const handleCreateTeamGrant = async () => {
    if (!selectedTeamId) return;
    
    const request: TypesCreateAccessGrantRequest = {
      roles: [selectedRole],
      team_id: selectedTeamId
    };

    await onCreateGrant(request);
    handleCloseTeamDialog();
  };

  // Determine if the typed input is an email not matching any org member
  const isTypedEmail = useMemo(() => {
    const input = userInputValue.trim();
    const debouncedInput = debouncedUserInputValue.trim();
    if (!effectiveOrganizationId) return false;
    if (!input || selectedUserReference || searchingUsers) return false;
    if (input !== debouncedInput) return false;
    return isValidEmail(input) && userSearchResults.length === 0;
  }, [debouncedUserInputValue, effectiveOrganizationId, searchingUsers, selectedUserReference, userInputValue, userSearchResults.length]);

  // States for the typed-email branch — derived from the lookup result so
  // the button label tells the user exactly what will happen. The
  // `already_invited` case takes precedence: if there's a pending
  // invitation we disable the button so admins can't double-send.
  //  - "already_invited" : pending invitation exists; button disabled
  //  - "not_helix"  : no Helix account → submitting sends an invitation
  //  - "not_in_org" : Helix account exists, not in this org → submit adds
  //                   them to the org AND grants project access
  //  - "in_org"     : Helix account exists and is already in this org →
  //                   submit just grants project access
  //  - "loading"    : lookup in flight; button is disabled
  type EmailState = 'not_helix' | 'not_in_org' | 'in_org' | 'loading' | 'already_invited';
  const typedEmailState: EmailState | null = useMemo(() => {
    if (!isTypedEmail) return null;
    if (lookingUpEmail) return 'loading';
    if (!emailLookup) return 'loading';
    if (emailLookup.is_invited) return 'already_invited';
    if (!emailLookup.exists) return 'not_helix';
    if (emailLookup.is_member) return 'in_org';
    return 'not_in_org';
  }, [isTypedEmail, lookingUpEmail, emailLookup]);

  // Button label varies by state. For a search-list selection (not a typed
  // email) we keep the simple "Add" since they're already an org member.
  const submitButtonLabel = useMemo(() => {
    if (!isTypedEmail) return 'Add to project';
    switch (typedEmailState) {
      case 'loading': return 'Checking…';
      case 'already_invited': return 'Invitation already sent';
      case 'not_helix': return 'Send invitation';
      case 'not_in_org': return 'Add to org and project';
      case 'in_org': return 'Add to project';
      default: return 'Add';
    }
  }, [isTypedEmail, typedEmailState]);

  // Handle create user grant
  const handleCreateUserGrant = async () => {
    let userReference = '';
    let addedToOrganization = false;

    if (selectedUserReference) {
      userReference = selectedUserReference || selectedUserId;
    } else if (isTypedEmail) {
      userReference = userInputValue.trim();
    } else {
      return;
    }

    if (isTypedEmail) {
      if (!effectiveOrganizationId) return;

      try {
        // Pass app_id + grant_roles so that — if the backend creates an
        // invitation (the email doesn't belong to an existing user) —
        // accepting that invitation will also materialise the AccessGrant
        // for this app with the same role the owner picked here. Without
        // this, the invitee would join the org but show up nowhere on
        // the project until someone manually granted access.
        const response = await api.getApiClient().v1OrganizationsMembersCreate(effectiveOrganizationId, {
          user_reference: userReference,
          role: TypesOrganizationRole.OrganizationRoleMember,
          app_id: appId,
          grant_roles: [selectedRole],
        });
        // Two outcomes:
        //  - Membership: the email matched an existing user, we can proceed
        //    to grant access as before.
        //  - Invitation: no Helix account exists for this email yet. We
        //    cannot create an access grant against a non-existent user, so
        //    we stop here — the access grant will be created when they
        //    accept the invitation and we later promote them to whatever
        //    role the inviter chose. The pending invitation is now
        //    visible in the org members list.
        if (response.data?.invited) {
          setOrgAddSnackbarSeverity('info');
          setOrgAddSnackbar(`${userReference} has been invited. They will gain access once they create their account.`);
          await Promise.all([
            orgTools.loadOrganization(effectiveOrganizationId),
            loadInvitations(),
          ]);
          handleCloseUserDialog();
          return;
        }
        addedToOrganization = true;
        const newUserId = response.data?.membership?.user_id || userReference;
        setSelectedUserId(newUserId);
        setSelectedUserReference(userReference);
        await orgTools.loadOrganization(effectiveOrganizationId);
      } catch (error) {
        console.error('Failed to add organization member before granting access:', error);
        setOrgAddSnackbarSeverity('error');
        const errorMessage = extractErrorMessage(error);
        setOrgAddSnackbar(errorMessage || 'Could not add this user to the organization.');
        return;
      }
    }

    const request: TypesCreateAccessGrantRequest = {
      roles: [selectedRole],
      user_reference: userReference
    };

    const result = await onCreateGrant(request);
    if (!result) return;

    if (addedToOrganization) {
      setOrgAddSnackbarSeverity('info');
      setOrgAddSnackbar(`${userReference} was added to the organization and granted access.`);
    }
    if (result?.added_to_organization) {
      const name = result.user?.full_name || userReference;
      setOrgAddSnackbarSeverity('info');
      setOrgAddSnackbar(`${name} was also added to the organization.`);
    }
    handleCloseUserDialog();
  };

  // Handle delete confirmation
  const handleDeleteClick = (grantId: string) => {
    setDeleteGrantId(grantId);
  };

  // Revoke a pending invitation. Skips the standard delete-confirm dialog
  // because revoking is low-stakes (we can always re-invite) and the
  // "Pending invitation" label already makes the row's meaning clear.
  const handleRevokeInvitation = async (invitation: TypesOrganizationInvitation) => {
    if (!effectiveOrganizationId || !invitation.id) return;
    try {
      setRevokingInvitationId(invitation.id);
      await api.getApiClient().v1OrganizationsInvitationsDelete(effectiveOrganizationId, invitation.id);
      await loadInvitations();
      setOrgAddSnackbarSeverity('info');
      setOrgAddSnackbar(`Invitation for ${invitation.email || 'user'} revoked.`);
    } catch (error) {
      console.error('Failed to revoke invitation:', error);
      setOrgAddSnackbarSeverity('error');
      setOrgAddSnackbar(extractErrorMessage(error) || 'Failed to revoke invitation.');
    } finally {
      setRevokingInvitationId(null);
    }
  };

  const handleConfirmDelete = async () => {
    if (deleteGrantId) {
      await onDeleteGrant(deleteGrantId);
      setDeleteGrantId(null);
    }
  };

  const handleCancelDelete = () => {
    setDeleteGrantId(null);
  };

  // Get role display name
  const getRoleDisplayName = (roleName: string): string => {
    switch(roleName.toLowerCase()) {
      case 'admin':
        return 'Admin';
      case 'editor':
        return 'Editor';
      case 'app_user':
        return 'User';
      default:
        return roleName;
    }
  };

  // Get role icon based on role name
  const getRoleIcon = (roleName: string) => {
    switch(roleName.toLowerCase()) {
      case 'admin':
        return <AdminPanelSettingsIcon fontSize="small" />;
      default:
        return <LockPersonIcon fontSize="small" />;
    }
  };

  // Load available roles when component mounts
  useEffect(() => {
    if (organization?.id && !organization.roles) {
      orgTools.loadOrganization(organization.id);
    }
  }, [organization?.id]);

  return (
    <Box>      
      {isLoading ? (
        <Box sx={{ display: 'flex', justifyContent: 'center', p: 3 }}>
          <CircularProgress />
        </Box>
      ) : (
        <>
          {/* Users with access */}
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mt: 3, mb: 2 }}>
            <Typography variant="subtitle1" sx={{ display: 'flex', alignItems: 'center', color: '#F8FAFC', fontWeight: 500 }}>
              <PersonIcon sx={{ mr: 1, color: '#A0AEC0' }} /> Users with access
            </Typography>
            {!isReadOnly && (
              <Button
                variant="outlined"
                size="small"
                startIcon={<AddIcon />}
                onClick={handleOpenUserDialog}
                sx={{
                  borderColor: '#353945',
                  color: '#A0AEC0',
                  '&:hover': {
                    borderColor: '#6366F1',
                    color: '#6366F1',
                  },
                }}
              >
                Add User
              </Button>
            )}
          </Box>
          
          {!hasOwnerRow && userGrants.length === 0 && invitations.length === 0 ? (
            <Box sx={{ 
              border: '1px solid #353945', 
              borderRadius: 2, 
              p: 3, 
              textAlign: 'center',
              color: '#A0AEC0',
              mb: 3
            }}>
              <Typography variant="body1" sx={{ mb: 1 }}>
                No users have been granted access
              </Typography>
              <Typography variant="body2">
                Add users to grant them access to this agent.
              </Typography>
            </Box>
          ) : (
            <Box sx={{ 
              border: '1px solid #353945', 
              borderRadius: 2,
              overflow: 'hidden',
              mb: 3
            }}>                            
              <Box sx={{ maxHeight: '400px', overflow: 'auto' }}>
                <Box component="table" sx={{ width: '100%', borderCollapse: 'collapse' }}>
                  <Box component="thead" sx={{ bgcolor: lightTheme.panelColor }}>
                    <Box component="tr">
                      <Box component="th" sx={{ 
                        p: 2, 
                        textAlign: 'left', 
                        color: '#A0AEC0', 
                        fontSize: '0.875rem',
                        fontWeight: 500,
                        borderBottom: '1px solid #353945'
                      }}>
                        User
                      </Box>
                      <Box component="th" sx={{ 
                        p: 2, 
                        textAlign: 'left', 
                        color: '#A0AEC0', 
                        fontSize: '0.875rem',
                        fontWeight: 500,
                        borderBottom: '1px solid #353945'
                      }}>
                        Email
                      </Box>
                      <Box component="th" sx={{ 
                        p: 2, 
                        textAlign: 'left', 
                        color: '#A0AEC0', 
                        fontSize: '0.875rem',
                        fontWeight: 500,
                        borderBottom: '1px solid #353945'
                      }}>
                        Roles
                      </Box>
                      {!isReadOnly && (
                        <Box component="th" sx={{ 
                          p: 2, 
                          textAlign: 'left', 
                          color: '#A0AEC0', 
                          fontSize: '0.875rem',
                          fontWeight: 500,
                          borderBottom: '1px solid #353945',
                          width: '100px'
                        }}>
                          Actions
                        </Box>
                      )}
                    </Box>
                  </Box>
                  <Box component="tbody">
                    {hasOwnerRow && (
                      <Box
                        component="tr"
                        sx={{
                          bgcolor: '#1E2028',
                          borderBottom: (userGrants.length > 0 || invitations.length > 0) ? '1px solid #353945' : 'none'
                        }}
                      >
                        <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                          <Typography
                            sx={{
                              color: '#7D8597',
                              fontWeight: 500,
                              fontSize: '0.875rem'
                            }}
                          >
                            {ownerDisplayName}
                          </Typography>
                        </Box>
                        <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                          <Typography
                            sx={{
                              color: '#6B7280',
                              fontSize: '0.875rem'
                            }}
                          >
                            {ownerDisplayEmail}
                          </Typography>
                        </Box>
                        <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                          <Chip
                            label={projectOwnerId && currentUser?.id === projectOwnerId ? 'Owner' : 'You'}
                            size="small"
                            sx={{
                              mr: 0.5,
                              mb: 0.5,
                              backgroundColor: '#374151',
                              color: '#D1D5DB'
                            }}
                          />
                        </Box>
                        {!isReadOnly && <Box component="td" sx={{ p: 2, verticalAlign: 'top' }} />}
                      </Box>
                    )}
                    {userGrants.map((grant, index) => {
                      const user = (grant.user || {}) as any
                      return (
                        <Box component="tr" key={grant.id} sx={{
                          '&:hover': { bgcolor: lightTheme.highlightColor },
                          borderBottom: (index < userGrants.length - 1 || invitations.length > 0) ? '1px solid #353945' : 'none'
                        }}>
                          <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                            <Typography sx={{ 
                              color: '#F1F1F1',
                              fontWeight: 500,
                              fontSize: '0.875rem'
                            }}>
                              {user.full_name || 'Unknown'}
                            </Typography>
                          </Box>
                          <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                            <Typography sx={{ 
                              color: '#A0AEC0',
                              fontSize: '0.875rem'
                            }}>
                              {user.email || 'No email'}
                            </Typography>
                          </Box>
                          <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                            {grant.roles && grant.roles.map(role => (
                              <Chip 
                                key={role.id} 
                                label={getRoleDisplayName(role.name)}
                                icon={getRoleIcon(role.name)} 
                                size="small" 
                                sx={{ 
                                  mr: 0.5,
                                  mb: 0.5,
                                  backgroundColor: role.name.toLowerCase() === 'admin' ? '#EF4444' : '#6366F1',
                                  color: 'white',
                                  '& .MuiChip-icon': {
                                    color: 'white'
                                  }
                                }} 
                              />
                            ))}
                          </Box>
                          {!isReadOnly && (
                            <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                              <Tooltip title="Remove access">
                                <IconButton
                                  size="small"
                                  onClick={() => handleDeleteClick(grant.id)}
                                  sx={{ 
                                    color: '#F87171',
                                    '&:hover': {
                                      color: '#EF4444',
                                      backgroundColor: 'rgba(239, 68, 68, 0.1)'
                                    }
                                  }}
                                >
                                  <DeleteIcon fontSize="small" />
                                </IconButton>
                              </Tooltip>
                            </Box>
                          )}
                        </Box>
                      )
                    })}
                    {invitations.map((invitation, index) => (
                      <Box
                        component="tr"
                        key={invitation.id}
                        sx={{
                          '&:hover': { bgcolor: lightTheme.highlightColor },
                          borderBottom: index < invitations.length - 1 ? '1px solid #353945' : 'none'
                        }}
                      >
                        <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                          <Typography sx={{
                            color: '#A0AEC0',
                            fontWeight: 500,
                            fontSize: '0.875rem',
                            fontStyle: 'italic'
                          }}>
                            (pending)
                          </Typography>
                        </Box>
                        <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                          <Typography sx={{
                            color: '#A0AEC0',
                            fontSize: '0.875rem'
                          }}>
                            {invitation.email}
                          </Typography>
                        </Box>
                        <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                          <Chip
                            label="Invite sent"
                            size="small"
                            sx={{
                              mr: 0.5,
                              mb: 0.5,
                              backgroundColor: '#374151',
                              color: '#D1D5DB',
                              border: '1px dashed #6B7280'
                            }}
                          />
                          {(invitation.grant_roles || []).map((roleName) => (
                            <Chip
                              key={roleName}
                              label={getRoleDisplayName(roleName)}
                              icon={getRoleIcon(roleName)}
                              size="small"
                              sx={{
                                mr: 0.5,
                                mb: 0.5,
                                backgroundColor: roleName.toLowerCase() === 'admin' ? '#EF4444' : '#6366F1',
                                color: 'white',
                                opacity: 0.7,
                                '& .MuiChip-icon': { color: 'white' }
                              }}
                            />
                          ))}
                        </Box>
                        {!isReadOnly && (
                          <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                            <Tooltip title="Revoke invitation">
                              <span>
                                <IconButton
                                  size="small"
                                  onClick={() => handleRevokeInvitation(invitation)}
                                  disabled={revokingInvitationId === invitation.id}
                                  sx={{
                                    color: '#F87171',
                                    '&:hover': {
                                      color: '#EF4444',
                                      backgroundColor: 'rgba(239, 68, 68, 0.1)'
                                    }
                                  }}
                                >
                                  <DeleteIcon fontSize="small" />
                                </IconButton>
                              </span>
                            </Tooltip>
                          </Box>
                        )}
                      </Box>
                    ))}
                  </Box>
                </Box>
              </Box>
            </Box>
          )}

          {/* Teams with access */}
          <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mt: 3, mb: 2 }}>
            <Typography variant="subtitle1" sx={{ display: 'flex', alignItems: 'center', color: '#F8FAFC', fontWeight: 500 }}>
              <GroupsIcon sx={{ mr: 1, color: '#A0AEC0' }} /> Teams with access
            </Typography>
            {!isReadOnly && (
              <Button
                variant="outlined"
                size="small"
                startIcon={<AddIcon />}
                onClick={handleOpenTeamDialog}
                sx={{
                  borderColor: '#353945',
                  color: '#A0AEC0',
                  '&:hover': {
                    borderColor: '#6366F1',
                    color: '#6366F1',
                  },
                }}
              >
                Add Team
              </Button>
            )}
          </Box>
          
          {teamGrants.length === 0 ? (
            <Box sx={{ 
              border: '1px solid #353945', 
              borderRadius: 2, 
              p: 3, 
              textAlign: 'center',
              color: '#A0AEC0'
            }}>
              <Typography variant="body1" sx={{ mb: 1 }}>
                No teams have been granted access
              </Typography>
              <Typography variant="body2">
                Add teams to grant them access to this agent.
              </Typography>
            </Box>
          ) : (
            <Box sx={{ 
              border: '1px solid #353945', 
              borderRadius: 2,
              overflow: 'hidden'
            }}>              
              <Box sx={{ maxHeight: '400px', overflow: 'auto' }}>
                <Box component="table" sx={{ width: '100%', borderCollapse: 'collapse' }}>
                  <Box component="thead" sx={{ bgcolor: lightTheme.panelColor }}>
                    <Box component="tr">
                      <Box component="th" sx={{ 
                        p: 2, 
                        textAlign: 'left', 
                        color: '#A0AEC0', 
                        fontSize: '0.875rem',
                        fontWeight: 500,
                        borderBottom: '1px solid #353945'
                      }}>
                        Team
                      </Box>
                      <Box component="th" sx={{ 
                        p: 2, 
                        textAlign: 'left', 
                        color: '#A0AEC0', 
                        fontSize: '0.875rem',
                        fontWeight: 500,
                        borderBottom: '1px solid #353945'
                      }}>
                        Roles
                      </Box>
                      {!isReadOnly && (
                        <Box component="th" sx={{ 
                          p: 2, 
                          textAlign: 'left', 
                          color: '#A0AEC0', 
                          fontSize: '0.875rem',
                          fontWeight: 500,
                          borderBottom: '1px solid #353945',
                          width: '100px'
                        }}>
                          Actions
                        </Box>
                      )}
                    </Box>
                  </Box>
                  <Box component="tbody">
                    {teamGrants.map((grant, index) => {
                      // Find team name
                      const teamId = grant.team_id || '';
                      const team = teams.find(t => t.id === teamId);
                      return (
                        <Box component="tr" key={grant.id} sx={{ 
                          '&:hover': { bgcolor: lightTheme.highlightColor },
                          borderBottom: index < teamGrants.length - 1 ? '1px solid #353945' : 'none'
                        }}>
                          <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                            <Link
                              sx={{
                                textDecoration: 'none',
                                fontWeight: 500,
                                color: '#6366F1',
                                fontSize: '0.875rem',
                                '&:hover': {
                                  color: '#8B5CF6',
                                  textDecoration: 'underline'
                                }
                              }}
                              href={`/orgs/${organization?.name}/teams/${teamId}/people`}
                              onClick={(e: React.MouseEvent<HTMLAnchorElement, MouseEvent>) => {
                                e.preventDefault()
                                e.stopPropagation()
                                router.navigate('team_people', {
                                  org_id: organization?.name,
                                  team_id: teamId,
                                })
                              }}
                            >
                              {team?.name || teamId}
                            </Link>
                          </Box>
                          <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                            {grant.roles && grant.roles.map(role => (
                              <Chip 
                                key={role.id} 
                                label={getRoleDisplayName(role.name)}
                                icon={getRoleIcon(role.name)} 
                                size="small" 
                                sx={{ 
                                  mr: 0.5,
                                  mb: 0.5,
                                  backgroundColor: role.name.toLowerCase() === 'admin' ? '#EF4444' : '#6366F1',
                                  color: 'white',
                                  '& .MuiChip-icon': {
                                    color: 'white'
                                  }
                                }} 
                              />
                            ))}
                          </Box>
                          {!isReadOnly && (
                            <Box component="td" sx={{ p: 2, verticalAlign: 'top' }}>
                              <Tooltip title="Remove access">
                                <IconButton
                                  size="small"
                                  onClick={() => handleDeleteClick(grant.id)}
                                  sx={{ 
                                    color: '#F87171',
                                    '&:hover': {
                                      color: '#EF4444',
                                      backgroundColor: 'rgba(239, 68, 68, 0.1)'
                                    }
                                  }}
                                >
                                  <DeleteIcon fontSize="small" />
                                </IconButton>
                              </Tooltip>
                            </Box>
                          )}
                        </Box>
                      );
                    })}
                  </Box>
                </Box>
              </Box>
            </Box>
          )}
        </>
      )}

      {/* Add Team Access Dialog */}
      <Dialog open={openTeamDialog} onClose={handleCloseTeamDialog} maxWidth="sm" fullWidth>
        <DialogTitle>Add Team Access</DialogTitle>
        <DialogContent>
          <FormControl fullWidth sx={{ mb: 3, mt: 2 }}>
            <InputLabel id="team-select-label">Team</InputLabel>
            <Select
              labelId="team-select-label"
              value={selectedTeamId}
              label="Team"
              onChange={(e) => setSelectedTeamId(e.target.value)}
            >
              <MenuItem value="">
                <em>Select a team</em>
              </MenuItem>
              {availableTeams.map((team) => (
                <MenuItem key={team.id} value={team.id}>
                  {team.name}
                </MenuItem>
              ))}
            </Select>
          </FormControl>
          
          <FormControl component="fieldset" fullWidth>
            <FormLabel component="legend">Role</FormLabel>
            <RadioGroup
              value={selectedRole}
              onChange={(e) => setSelectedRole(e.target.value)}
            >
              {availableRoles.map((role) => (
                <FormControlLabel 
                  key={role.id} 
                  value={role.name}
                  control={<Radio />} 
                  label={
                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                      <Typography variant="body1" sx={{ fontWeight: 500 }}>
                        {role.name ? role.name.charAt(0).toUpperCase() + role.name.slice(1).replace('_', ' ') : ''}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {role.description || ''}
                      </Typography>
                    </Box>
                  }
                  sx={{ mb: 1 }}
                />
              ))}
              {availableRoles.length === 0 && (
                <FormControlLabel 
                  value="app_user"
                  control={<Radio />} 
                  label={
                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                      <Typography variant="body1" sx={{ fontWeight: 500 }}>User</Typography>
                      <Typography variant="caption" color="text.secondary">
                        Basic access privileges
                      </Typography>
                    </Box>
                  }
                  sx={{ mb: 1 }}
                />
              )}
            </RadioGroup>
          </FormControl>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseTeamDialog}>Cancel</Button>
          <Button 
            onClick={handleCreateTeamGrant} 
            variant="contained" 
            color="primary"
            disabled={!selectedTeamId || !selectedRole}
          >
            Add
          </Button>
        </DialogActions>
      </Dialog>

      {/* Add User Access Dialog */}
      <Dialog open={openUserDialog} onClose={handleCloseUserDialog} maxWidth="sm" fullWidth>
        <DialogTitle>Add User</DialogTitle>
        <DialogContent>
          <TextField
            autoFocus
            margin="dense"
            label="Search by name or email"
            type="text"
            fullWidth
            variant="outlined"
            value={userInputValue}
            onChange={(event) => {
              setUserInputValue(event.target.value);
              setSelectedUserId('');
              setSelectedUserReference('');
              setEmailLookup(null);
            }}
            InputProps={{
              startAdornment: (
                <InputAdornment position="start">
                  <SearchIcon color="action" />
                </InputAdornment>
              ),
              endAdornment: searchingUsers ? (
                <InputAdornment position="end">
                  <CircularProgress size={20} />
                </InputAdornment>
              ) : null,
            }}
            sx={{ mt: 2 }}
          />

          <Box sx={{ mt: 2, mb: 1 }}>
            <Typography variant="subtitle2" color="text.secondary">
              {getUserSearchMessage()}
            </Typography>
          </Box>

          <List sx={{ width: '100%', maxHeight: 260, overflow: 'auto', mb: 2 }}>
            {userSearchResults.map((user) => {
              const userId = user.id || '';
              const userName = user.full_name || user.email || 'Unnamed User';
              const userReference = user.email || userId;
              const isCurrentUser = !!userId && userId === currentUser?.id;
              const alreadyHasAccess = !!userId && existingGrantUserIds.has(userId);
              const isDisabled = !userReference || isCurrentUser || alreadyHasAccess;
              const isSelected = !!userReference && selectedUserReference === userReference;
              const disabledLabel = isCurrentUser ? 'You' : alreadyHasAccess ? 'Already has access' : '';
              
              return (
                <ListItem
                  key={userId || user.email}
                  divider
                  sx={{
                    borderRadius: 1,
                    bgcolor: isSelected ? 'action.selected' : undefined,
                    '&:last-child': { borderBottom: 0 },
                  }}
                >
                  <Box sx={{ display: 'flex', alignItems: 'center', mr: 2 }}>
                    <PersonIcon color={isSelected ? 'primary' : 'action'} />
                  </Box>
                  <ListItemText
                    primary={userName}
                    secondary={user.email || user.username}
                  />
                  <ListItemSecondaryAction>
                    <Tooltip title={disabledLabel}>
                      <span>
                        <Button
                          variant={isSelected ? 'contained' : 'outlined'}
                          color="primary"
                          size="small"
                          onClick={() => {
                            setSelectedUserId(userId);
                            setSelectedUserReference(userReference);
                          }}
                          disabled={isDisabled}
                        >
                          {disabledLabel || (isSelected ? 'Selected' : 'Select')}
                        </Button>
                      </span>
                    </Tooltip>
                  </ListItemSecondaryAction>
                </ListItem>
              );
            })}
          </List>

          {isTypedEmail && typedEmailState === 'loading' && (
            <Alert severity="info" sx={{ mb: 2 }} icon={<CircularProgress size={16} />}>
              Checking whether this email already has a Helix account…
            </Alert>
          )}
          {isTypedEmail && typedEmailState === 'already_invited' && (
            <Alert severity="warning" sx={{ mb: 2 }}>
              An invitation has already been sent to this email. They will join the organization automatically when they register — no need to resend.
            </Alert>
          )}
          {isTypedEmail && typedEmailState === 'not_helix' && (
            <Alert severity="info" sx={{ mb: 2 }}>
              No Helix account exists for this email yet. Confirming will email them an invitation — they will join the organization automatically when they register.
            </Alert>
          )}
          {isTypedEmail && typedEmailState === 'not_in_org' && (
            <Alert severity="info" sx={{ mb: 2 }}>
              This user has a Helix account but isn't in your organization yet. Confirming will add them to the organization and grant project access.
            </Alert>
          )}
          {isTypedEmail && typedEmailState === 'in_org' && (
            <Alert severity="info" sx={{ mb: 2 }}>
              This user is already a member of the organization. Confirming will grant them project access.
            </Alert>
          )}

          <FormControl component="fieldset" fullWidth>
            <FormLabel component="legend">Role</FormLabel>
            <RadioGroup
              value={selectedRole}
              onChange={(e) => setSelectedRole(e.target.value)}
            >
              {availableRoles.map((role) => (
                <FormControlLabel
                  key={role.id}
                  value={role.name}
                  control={<Radio />}
                  label={
                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                      <Typography variant="body1" sx={{ fontWeight: 500 }}>
                        {role.name ? role.name.charAt(0).toUpperCase() + role.name.slice(1).replace('_', ' ') : ''}
                      </Typography>
                      <Typography variant="caption" color="text.secondary">
                        {role.description || ''}
                      </Typography>
                    </Box>
                  }
                  sx={{ mb: 1 }}
                />
              ))}
              {availableRoles.length === 0 && (
                <FormControlLabel
                  value="app_user"
                  control={<Radio />}
                  label={
                    <Box sx={{ display: 'flex', flexDirection: 'column' }}>
                      <Typography variant="body1" sx={{ fontWeight: 500 }}>User</Typography>
                      <Typography variant="caption" color="text.secondary">
                        Basic access privileges
                      </Typography>
                    </Box>
                  }
                  sx={{ mb: 1 }}
                />
              )}
            </RadioGroup>
          </FormControl>
          <Typography variant="body2" color="text.secondary" sx={{ mt: 2 }}>
            Search lists members of this organization. Enter a full email to invite someone who isn't in Helix yet, or to add an existing Helix user to your org.
          </Typography>
        </DialogContent>
        <DialogActions>
          <Button onClick={handleCloseUserDialog}>Cancel</Button>
          <Button
            onClick={handleCreateUserGrant}
            variant="contained"
            color="primary"
            disabled={(!selectedUserReference && !isTypedEmail) || !selectedRole || typedEmailState === 'loading' || typedEmailState === 'already_invited'}
          >
            {submitButtonLabel}
          </Button>
        </DialogActions>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      {deleteGrantId && (
        <DeleteConfirmWindow
          title="access grant"
          onSubmit={handleConfirmDelete}
          onCancel={handleCancelDelete}
        />
      )}

      {/* Snackbar for auto-add to org notification */}
      <Snackbar
        open={!!orgAddSnackbar}
        autoHideDuration={6000}
        onClose={() => setOrgAddSnackbar(null)}
        anchorOrigin={{ vertical: 'bottom', horizontal: 'center' }}
      >
        <Alert onClose={() => setOrgAddSnackbar(null)} severity={orgAddSnackbarSeverity} sx={{ width: '100%' }}>
          {orgAddSnackbar}
        </Alert>
      </Snackbar>
    </Box>
  );
};

export default AccessManagement;
