import React, { useState, useEffect, useMemo } from 'react'
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
// Import icons
import AddIcon from '@mui/icons-material/Add'
import DeleteIcon from '@mui/icons-material/Delete'
import PersonIcon from '@mui/icons-material/Person'
import GroupsIcon from '@mui/icons-material/Groups'
import AdminPanelSettingsIcon from '@mui/icons-material/AdminPanelSettings'
import LockPersonIcon from '@mui/icons-material/LockPerson'
import useAccount from '../../hooks/useAccount'
import { TypesAccessGrant, TypesCreateAccessGrantRequest, TypesRole } from '../../api/api'
import DeleteConfirmWindow from '../widgets/DeleteConfirmWindow'
import useRouter from '../../hooks/useRouter'
import useTheme from '@mui/material/styles/useTheme'

interface AccessManagementProps {
  appId: string;
  accessGrants: TypesAccessGrant[];
  isLoading: boolean;
  isReadOnly: boolean;
  onCreateGrant: (request: TypesCreateAccessGrantRequest) => Promise<TypesAccessGrant | null>;
  onDeleteGrant: (grantId: string) => Promise<boolean>;
}

const AccessManagement: React.FC<AccessManagementProps> = ({
  appId,
  accessGrants,
  isLoading,
  isReadOnly,
  onCreateGrant,
  onDeleteGrant
}) => {
  // Get organization data from account context
  const account = useAccount();
  const orgTools = account.organizationTools;
  const organization = orgTools.organization;
  const router = useRouter();
  const theme = useTheme();

  // Local state
  const [openTeamDialog, setOpenTeamDialog] = useState(false);
  const [openUserDialog, setOpenUserDialog] = useState(false);
  const [selectedTeamId, setSelectedTeamId] = useState('');
  const [selectedUserId, setSelectedUserId] = useState('');
  const [selectedRole, setSelectedRole] = useState('app_user'); // Default role
  const [deleteGrantId, setDeleteGrantId] = useState<string | null>(null);

  // Extract roles from the organization
  const availableRoles = useMemo(() => {
    return organization?.roles || [];
  }, [organization]);

  // Filter grants into users and teams
  const userGrants = useMemo(() => {
    return accessGrants.filter(grant => grant.user_id);
  }, [accessGrants]);

  const teamGrants = useMemo(() => {
    return accessGrants.filter(grant => grant.team_id);
  }, [accessGrants]);

  // Get organization teams
  const teams = useMemo(() => {
    return organization?.teams || [];
  }, [organization]);

  // Get organization members 
  const members = useMemo(() => {
    if (!organization?.memberships) return [];
    return organization.memberships.map(membership => {
      const user = (membership.user || {}) as any
      return {
        id: membership.user_id,
        name: user.full_name || 'Unknown',
        email: user.email || 'No email'
      }
    })
  }, [organization]);

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

  // Handle create user grant
  const handleCreateUserGrant = async () => {
    if (!selectedUserId) return;

    // Find the user's email from the members list
    const user = members.find(m => m.id === selectedUserId);
    if (!user) return;
    
    const request: TypesCreateAccessGrantRequest = {
      roles: [selectedRole],
      user_reference: user.email
    };

    await onCreateGrant(request);
    handleCloseUserDialog();
  };

  // Handle delete confirmation
  const handleDeleteClick = (grantId: string) => {
    setDeleteGrantId(grantId);
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
      <Box sx={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', mb: 2 }}>
        <Typography variant="h6">Access Management</Typography>
      </Box>

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
                Add User Access
              </Button>
            )}
          </Box>
          
          {userGrants.length === 0 ? (
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
              <Box sx={{ 
                bgcolor: '#23262F', 
                p: 2, 
                borderBottom: '1px solid #353945',
                display: 'flex',
                alignItems: 'center',
                gap: 1
              }}>
                <PersonIcon fontSize="small" sx={{ color: '#A0AEC0' }} />
                <Typography variant="subtitle2" sx={{ color: '#A0AEC0', fontWeight: 500 }}>
                  Users with Access ({userGrants.length})
                </Typography>
              </Box>
              
              <Box sx={{ maxHeight: '400px', overflow: 'auto' }}>
                <Box component="table" sx={{ width: '100%', borderCollapse: 'collapse' }}>
                  <Box component="thead" sx={{ bgcolor: '#181A20' }}>
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
                    {userGrants.map((grant, index) => {
                      const user = (grant.user || {}) as any
                      return (
                        <Box component="tr" key={grant.id} sx={{ 
                          '&:hover': { bgcolor: '#23262F' },
                          borderBottom: index < userGrants.length - 1 ? '1px solid #353945' : 'none'
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
                Add Team Access
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
              <Box sx={{ 
                bgcolor: '#23262F', 
                p: 2, 
                borderBottom: '1px solid #353945',
                display: 'flex',
                alignItems: 'center',
                gap: 1
              }}>
                <GroupsIcon fontSize="small" sx={{ color: '#A0AEC0' }} />
                <Typography variant="subtitle2" sx={{ color: '#A0AEC0', fontWeight: 500 }}>
                  Teams with Access ({teamGrants.length})
                </Typography>
              </Box>
              
              <Box sx={{ maxHeight: '400px', overflow: 'auto' }}>
                <Box component="table" sx={{ width: '100%', borderCollapse: 'collapse' }}>
                  <Box component="thead" sx={{ bgcolor: '#181A20' }}>
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
                          '&:hover': { bgcolor: '#23262F' },
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
        <DialogTitle>Add User Access</DialogTitle>
        <DialogContent>
          <FormControl fullWidth sx={{ mb: 3, mt: 2 }}>
            <InputLabel id="user-select-label">User</InputLabel>
            <Select
              labelId="user-select-label"
              value={selectedUserId}
              label="User"
              onChange={(e) => setSelectedUserId(e.target.value)}
            >
              <MenuItem value="">
                <em>Select a user</em>
              </MenuItem>
              {members.map((user) => (
                <MenuItem key={user.id} value={user.id}>
                  {user.name} ({user.email})
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
          <Button onClick={handleCloseUserDialog}>Cancel</Button>
          <Button 
            onClick={handleCreateUserGrant} 
            variant="contained" 
            color="primary"
            disabled={!selectedUserId || !selectedRole}
          >
            Add
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
    </Box>
  );
};

export default AccessManagement; 