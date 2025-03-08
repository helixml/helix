import { useState, useCallback, useEffect, useMemo } from 'react'
import { TypesOrganization, TypesOrganizationMembership, TypesTeam, TypesCreateTeamRequest, TypesUpdateTeamRequest } from '../api/api'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useAccount from './useAccount'
import useRouter from './useRouter'
import { extractErrorMessage } from './useErrorCallback'
import bluebird from 'bluebird'

export interface UserSearchResult {
  id: string;
  email: string;
  fullName?: string;
  username?: string;
}

export interface SearchUsersResponse {
  users: UserSearchResult[];
  pagination: {
    total: number;
    limit: number;
    offset: number;
  };
}

export interface IOrganizationTools {
  organizations: TypesOrganization[],
  loading: boolean,
  organization?: TypesOrganization,
  loadOrganizations: () => Promise<void>,
  createOrganization: (org: TypesOrganization) => Promise<boolean>,
  updateOrganization: (id: string, org: TypesOrganization) => Promise<boolean>,
  deleteOrganization: (id: string) => Promise<boolean>,
  loadOrganization: (id: string) => Promise<void>,
  // Organization member management methods
  addMemberToOrganization: (organizationId: string, userReference: string, role?: string) => Promise<boolean>,
  deleteMemberFromOrganization: (organizationId: string, userId: string) => Promise<boolean>,
  // Team management methods
  createTeam: (organizationId: string, team: TypesTeam) => Promise<boolean>,
  createTeamWithCreator: (organizationId: string, ownerId: string, team: TypesTeam) => Promise<boolean>,
  updateTeam: (organizationId: string, teamId: string, team: TypesTeam) => Promise<boolean>,
  deleteTeam: (organizationId: string, teamId: string) => Promise<boolean>,
  // Team member management methods
  addTeamMember: (organizationId: string, teamId: string, userReference: string) => Promise<boolean>,
  removeTeamMember: (organizationId: string, teamId: string, userId: string) => Promise<boolean>,
  // User search method
  searchUsers: (query: { email?: string, name?: string, username?: string }) => Promise<SearchUsersResponse>,
}

export const defaultOrganizationTools: IOrganizationTools = {
  organizations: [],
  loading: false,
  loadOrganizations: async () => {},
  createOrganization: async () => false,
  updateOrganization: async () => false,
  deleteOrganization: async () => false,
  loadOrganization: async () => {},
  // Default organization member methods
  addMemberToOrganization: async () => false,
  deleteMemberFromOrganization: async () => false,
  // Default team methods
  createTeam: async () => false,
  createTeamWithCreator: async () => false,
  updateTeam: async () => false,
  deleteTeam: async () => false,
  // Default team member methods
  addTeamMember: async () => false,
  removeTeamMember: async () => false,
  // Default user search method
  searchUsers: async () => ({ users: [], pagination: { total: 0, limit: 0, offset: 0 } }),
}

export default function useOrganizations(): IOrganizationTools {
  const [organizations, setOrganizations] = useState<TypesOrganization[]>([])
  const [organization, setOrganization] = useState<TypesOrganization | undefined>(undefined)
  const [loading, setLoading] = useState(false)
  const [initialized, setInitialized] = useState(false)
  const api = useApi()
  const snackbar = useSnackbar()
  const account = useAccount()
  const router = useRouter()

  // Extract org_id parameter from router
  const orgIdParam = router.params.org_id

  // Load a single organization with all its details
  const loadOrganization = useCallback(async (id: string) => {
    try {
      setLoading(true)
      // Fetch the organization details
      const orgResult = await api.getApiClient().v1OrganizationsDetail(id)

      // Fetch members for the organization
      const membersResult = await api.getApiClient().v1OrganizationsMembersDetail(id)
      
      // Fetch roles for the organization
      const rolesResult = await api.getApiClient().v1OrganizationsRolesDetail(id)
      
      // Fetch teams for the organization
      const teamsResult = await api.getApiClient().v1OrganizationsTeamsDetail(id)
      
      // Fetch team memberships in parallel for each team
      const teamsWithMemberships = await Promise.all(
        teamsResult.data.map(async (team) => {
          try {
            // Only fetch members if team has an ID
            if (team.id) {
              const teamMembersResult = await api.getApiClient().v1OrganizationsTeamsMembersDetail(id, team.id)
              return {
                ...team,
                memberships: teamMembersResult.data
              }
            }
            // Return team with empty memberships if no ID
            return {
              ...team,
              memberships: []
            }
          } catch (error) {
            console.error(`Error loading members for team ${team.id || 'unknown'}:`, error)
            return {
              ...team,
              memberships: []
            }
          }
        })
      )
      
      // Create a complete organization object with all details
      const completeOrg = {
        ...orgResult.data,
        memberships: membersResult.data,
        roles: rolesResult.data,
        teams: teamsWithMemberships
      }
      
      setOrganization(completeOrg)
    } catch (error) {
      console.error(`Error loading organization ${id}:`, error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || `Error loading organization details`)
      setOrganization(undefined)
    } finally {
      setLoading(false)
    }
  }, [])

  // this is called by the top level account context once we have a login
  // so - we can know that when 'initialized' is true, we have a user
  const loadOrganizations = useCallback(async () => {
    try {
      setLoading(true)
      const result = await api.getApiClient().v1OrganizationsList()
      
      // Fetch members for each organization in parallel
      const orgsWithMembers = await bluebird.map(result.data, async (org) => {
        try {
          // Only fetch members if org has an ID
          if (org.id) {
            // Call the API to get members for this organization
            const membersResult = await api.getApiClient().v1OrganizationsMembersDetail(org.id)
            // Create a new object with the members field populated
            return {
              ...org,
              memberships: membersResult.data
            }
          }
          return org
        } catch (error) {
          console.error(`Error fetching members for organization ${org.id}:`, error)
          // Return the original org if there was an error fetching members
          return org
        }
      })

      // Sort organizations by display_name (or name if display_name is not available)
      const sortedOrgs = [...orgsWithMembers].sort((a, b) => {
        const aName = (a.display_name || a.name || '').toLowerCase()
        const bName = (b.display_name || b.name || '').toLowerCase()
        return aName.localeCompare(bName)
      })

      setOrganizations(sortedOrgs)
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error loading organizations')
    } finally {
      setInitialized(true)
      setLoading(false)
    }
  }, [])

  const createOrganization = useCallback(async (org: TypesOrganization) => {
    try {
      await api.getApiClient().v1OrganizationsCreate(org)
      snackbar.success('Organization created')
      await loadOrganizations()
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error creating organization')
      return false
    }
  }, [])

  const updateOrganization = useCallback(async (id: string, org: TypesOrganization) => {
    try {
      await api.getApiClient().v1OrganizationsUpdate(id, org)
      snackbar.success('Organization updated')
      await loadOrganizations()
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error updating organization')
      return false
    }
  }, [])

  const deleteOrganization = useCallback(async (id: string) => {
    try {
      await api.getApiClient().v1OrganizationsDelete(id)
      snackbar.success('Organization deleted')
      await loadOrganizations()
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error deleting organization')
      return false
    }
  }, [])


  const addMemberToOrganization = useCallback(async (organizationId: string, userReference: string, role?: string) => {
    if (!organizationId) {
      snackbar.error('No active organization')
      return false
    }

    try {
      // Create a request to add a member to the organization
      const request = {
        user_reference: userReference,
        role_id: role || 'member' // Default to 'member' if no role specified
      }
      
      await api.getApiClient().v1OrganizationsMembersCreate(organizationId, request)
      snackbar.success('Member added')
      await loadOrganization(organizationId)
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error adding member')
      return false
    }
  }, [])

  const deleteMemberFromOrganization = useCallback(async (organizationId: string, userId: string) => {
    if (!organizationId) {
      snackbar.error('No active organization')
      return false
    }

    try {
      await api.getApiClient().v1OrganizationsMembersDelete(organizationId, userId)
      snackbar.success('Member removed')
      await loadOrganization(organizationId)
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error removing member')
      return false
    }
  }, [api, loadOrganization, snackbar])

  const createTeam = useCallback(async (organizationId: string, team: TypesTeam) => {
    if (!organizationId) {
      snackbar.error('No active organization')
      return false
    }

    try {
      const request: TypesCreateTeamRequest = {
        name: team.name,
        organization_id: organizationId
      }
      await api.getApiClient().v1OrganizationsTeamsCreate(organizationId, request)
      snackbar.success('Team created')
      await loadOrganization(organizationId)
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error creating team')
      return false
    }
  }, [])

  const createTeamWithCreator = useCallback(async (organizationId: string, ownerId: string, team: TypesTeam) => {
    if (!organizationId) {
      snackbar.error('No active organization')
      return false
    }

    try {
      // First, create the team
      const request: TypesCreateTeamRequest = {
        name: team.name,
        organization_id: organizationId
      }
      
      const teamResult = await api.getApiClient().v1OrganizationsTeamsCreate(organizationId, request)
      
      if (!teamResult.data || !teamResult.data.id) {
        snackbar.error('Team was created but no team ID was returned')
        return false
      }
      
      const teamId = teamResult.data.id
      
      // Use the provided ownerId instead of getting it from the account context
      if (!ownerId) {
        snackbar.info('Team created, but could not add owner as a member because owner ID is not provided')
        await loadOrganization(organizationId)
        return true
      }
      
      // Add the owner as a team member
      const memberRequest = {
        user_reference: ownerId
      }
      
      await api.getApiClient().v1OrganizationsTeamsMembersCreate(organizationId, teamId, memberRequest)
      
      // Now we need to grant the owner admin access to the team
      // We'll use the access grant system to assign the admin role
      // First, get the admin role ID from the organization roles
      const roles = await api.getApiClient().v1OrganizationsRolesDetail(organizationId)
      const adminRole = roles.data.find(role => role.name && role.name.toLowerCase() === 'admin')
      
      if (adminRole && adminRole.id) {
        // Create access grant with admin role
        const grantRequest = {
          user_reference: ownerId,
          roles: [adminRole.id]
        }
        
        try {
          // We're using the more generic API endpoint for creating access grants
          await api.post(`/api/v1/organizations/${organizationId}/teams/${teamId}/access-grants`, grantRequest)
        } catch (grantError) {
          console.error('Error assigning admin role:', grantError)
          // We'll continue since the user is at least a member of the team
        }
      }
      
      snackbar.success('Team created and owner has been added as an admin')
      await loadOrganization(organizationId)
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error creating team with creator')
      return false
    }
  }, [api, loadOrganization, snackbar])

  const updateTeam = useCallback(async (organizationId: string, teamId: string, team: TypesTeam) => {
    if (!organizationId) {
      snackbar.error('No active organization')
      return false
    }

    try {
      const request: TypesUpdateTeamRequest = {
        name: team.name
      }
      await api.getApiClient().v1OrganizationsTeamsUpdate(organizationId, teamId, request)
      snackbar.success('Team updated')
      await loadOrganization(organizationId)
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error updating team')
      return false
    }
  }, [])

  const deleteTeam = useCallback(async (organizationId: string, teamId: string) => {
    if (!organizationId) {
      snackbar.error('No active organization')
      return false
    }

    try {
      await api.getApiClient().v1OrganizationsTeamsDelete(organizationId, teamId)
      snackbar.success('Team deleted')
      await loadOrganization(organizationId)
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error deleting team')
      return false
    }
  }, [])

  // Add a member to a team
  const addTeamMember = useCallback(async (organizationId: string, teamId: string, userReference: string) => {
    if (!organizationId) {
      snackbar.error('No active organization')
      return false
    }

    try {
      const request = {
        user_reference: userReference
      }
      
      await api.getApiClient().v1OrganizationsTeamsMembersCreate(organizationId, teamId, request)
      snackbar.success('Member added to team')
      await loadOrganization(organizationId)
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error adding member to team')
      return false
    }
  }, [api, loadOrganization, snackbar])

  // Remove a member from a team
  const removeTeamMember = useCallback(async (organizationId: string, teamId: string, userId: string) => {
    if (!organizationId) {
      snackbar.error('No active organization')
      return false
    }

    try {
      // Use the delete method from the useApi hook
      await api.delete(`/api/v1/organizations/${organizationId}/teams/${teamId}/members/${userId}`)
      snackbar.success('Member removed from team')
      await loadOrganization(organizationId)
      return true
    } catch (error) {
      console.error(error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error removing member from team')
      return false
    }
  }, [])

  // Add the searchUsers function implementation
  const searchUsers = useCallback(async (query: { email?: string, name?: string, username?: string }) => {
    try {
      // Build query parameters
      const params = new URLSearchParams();
      if (query.email) params.append('email', query.email);
      if (query.name) params.append('name', query.name);
      if (query.username) params.append('username', query.username);
      
      // Call the API endpoint
      const response = await api.get(`/api/v1/users/search?${params.toString()}`);

      // Return the response data if it exists, otherwise return empty results
      if (response) {
        return response
      }
      
      return { users: [], pagination: { total: 0, limit: 0, offset: 0 } };
    } catch (error) {
      console.error('Error searching users:', error);
      const errorMessage = extractErrorMessage(error);
      snackbar.error(errorMessage || 'Error searching users');
      // Return empty result on error
      return { users: [], pagination: { total: 0, limit: 0, offset: 0 } };
    }
  }, []);

  // Effect to load organization when orgIdParam changes
  useEffect(() => {
    if (orgIdParam && initialized) {
      const useOrg = organizations.find((org) => org.id === orgIdParam || org.name === orgIdParam)
      if(!useOrg || !useOrg.id) return
      loadOrganization(useOrg.id)
    }
  }, [orgIdParam, initialized])

  return {
    organizations,
    loading,
    organization,
    loadOrganizations,
    createOrganization,
    updateOrganization,
    deleteOrganization,
    loadOrganization,
    // Include organization member methods
    addMemberToOrganization,
    deleteMemberFromOrganization,
    // Include team methods
    createTeam,
    createTeamWithCreator,
    updateTeam,
    deleteTeam,
    // Include team member methods
    addTeamMember,
    removeTeamMember,
    // Include user search method
    searchUsers,
  }
} 