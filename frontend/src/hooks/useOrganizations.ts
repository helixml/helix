import { useState, useCallback, useEffect, useMemo } from 'react'
import { TypesOrganization, TypesOrganizationMembership, TypesTeam, TypesCreateTeamRequest, TypesUpdateTeamRequest, TypesOrganizationRole, TypesAccessGrant, TypesCreateAccessGrantRequest, TypesUserSearchResponse } from '../api/api'
import useApi from './useApi'
import useSnackbar from './useSnackbar'
import useRouter from './useRouter'
import { extractErrorMessage } from './useErrorCallback'
import bluebird from 'bluebird'

export interface IOrganizationTools {
  organizations: TypesOrganization[],
  loading: boolean,
  orgID: string,
  organization?: TypesOrganization,
  // Access grants state
  appAccessGrants: TypesAccessGrant[],
  loadingAccessGrants: boolean,
  // Organization methods
  loadOrganizations: () => Promise<void>,
  createOrganization: (org: TypesOrganization) => Promise<boolean>,
  updateOrganization: (id: string, org: TypesOrganization) => Promise<boolean>,
  deleteOrganization: (id: string) => Promise<boolean>,
  loadOrganization: (id: string) => Promise<void>,
  // Organization member management methods
  addMemberToOrganization: (organizationId: string, userReference: string, role?: string) => Promise<boolean>,
  deleteMemberFromOrganization: (organizationId: string, userId: string) => Promise<boolean>,
  updateOrganizationMemberRole: (organizationId: string, userId: string, role: TypesOrganizationRole) => Promise<boolean>,
  // Team management methods
  createTeam: (organizationId: string, team: TypesTeam) => Promise<boolean>,
  createTeamWithCreator: (organizationId: string, ownerId: string, team: TypesTeam) => Promise<boolean>,
  updateTeam: (organizationId: string, teamId: string, team: TypesTeam) => Promise<boolean>,
  deleteTeam: (organizationId: string, teamId: string) => Promise<boolean>,
  // Team member management methods
  addTeamMember: (organizationId: string, teamId: string, userReference: string) => Promise<boolean>,
  removeTeamMember: (organizationId: string, teamId: string, userId: string) => Promise<boolean>,
  // User search method
  searchUsers: (query: { query: string, organizationId?: string }) => Promise<TypesUserSearchResponse>,
  // App access grants methods
  listAppAccessGrants: (appId: string) => Promise<TypesAccessGrant[]>,
  createAppAccessGrant: (appId: string, grant: TypesCreateAccessGrantRequest) => Promise<boolean>,
  updateAppAccessGrant: (appId: string, grantId: string, grant: TypesCreateAccessGrantRequest) => Promise<boolean>,
  deleteAppAccessGrant: (appId: string, grantId: string) => Promise<boolean>,
}

// Default implementation
export const defaultOrganizationTools: IOrganizationTools = {
  organizations: [],
  loading: false,
  orgID: '',
  organization: undefined,
  appAccessGrants: [],
  loadingAccessGrants: false,
  loadOrganizations: async () => {},
  createOrganization: async () => false,
  updateOrganization: async () => false,
  deleteOrganization: async () => false,
  loadOrganization: async () => {},
  // Organization member management methods
  addMemberToOrganization: async () => false,
  deleteMemberFromOrganization: async () => false,
  updateOrganizationMemberRole: async () => false,
  // Team management methods
  createTeam: async () => false,
  createTeamWithCreator: async () => false,
  updateTeam: async () => false,
  deleteTeam: async () => false,
  // Team member management methods
  addTeamMember: async () => false,
  removeTeamMember: async () => false,
  // User search method
  searchUsers: async () => ({ users: [], pagination: { total: 0, limit: 0, offset: 0 } }),
  // Access grants methods
  listAppAccessGrants: async () => [],
  createAppAccessGrant: async () => false,
  updateAppAccessGrant: async () => false,
  deleteAppAccessGrant: async () => false,
}

/*

  WARNING: you cannot use `useAccount` inside this hook
  because this hook is used inside `useAccount`
  
*/
export default function useOrganizations(): IOrganizationTools {
  const api = useApi()
  const snackbar = useSnackbar()
  const router = useRouter()
  const [loading, setLoading] = useState<boolean>(false)
  const [organizations, setOrganizations] = useState<TypesOrganization[]>([])
  const [organization, setOrganization] = useState<TypesOrganization>()
  const [initialized, setInitialized] = useState(false)
  
  // State for app access grants
  const [appAccessGrants, setAppAccessGrants] = useState<TypesAccessGrant[]>([])
  const [loadingAccessGrants, setLoadingAccessGrants] = useState<boolean>(false)

  // Extract org_id parameter from router
  const orgID = router.params.org_id || ''

  // Load a single organization with all its details
  const loadOrganization = async (id: string) => {
    try {
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

              // Sort team members by name in a case-insensitive manner
              const sortedTeamMembers = [...teamMembersResult.data].sort((a, b) => {
                // Handle any type issues by casting to any
                const aUser = a.user as any
                const bUser = b.user as any

                // Get the name from FullName, fullName, or email as fallback
                const aName = ((aUser?.full_name || aUser?.full_name  || a.user?.email || '')).toLowerCase()
                const bName = ((bUser?.full_name || bUser?.full_name || b.user?.email || '')).toLowerCase()

                return aName.localeCompare(bName)
              })

              return {
                ...team,
                memberships: sortedTeamMembers
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

      // Sort organization members by name in a case-insensitive manner
      const sortedOrgMembers = [...membersResult.data].sort((a, b) => {
        // Handle any type issues by casting to any
        const aUser = a.user as any
        const bUser = b.user as any

        // Get the name from FullName, fullName, or email as fallback
        const aName = ((aUser?.full_name || aUser?.full_name || a.user?.email || '')).toLowerCase()
        const bName = ((bUser?.full_name || bUser?.full_name || b.user?.email || '')).toLowerCase()

        return aName.localeCompare(bName)
      })

      // Create a complete organization object with all details
      const completeOrg = {
        ...orgResult.data,
        memberships: sortedOrgMembers,
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
  }

  // this is called by the top level account context once we have a login
  // so - we can know that when 'initialized' is true, we have a user
  const loadOrganizations = useCallback(async () => {
    try {
      // setLoading(true)
      const result = await api.getApiClient().v1OrganizationsList()

      // Fetch members for each organization in parallel
      const orgsWithMembers = await bluebird.map(result.data, async (org) => {
        try {
          // Only fetch members if org has an ID
          if (org.id) {
            // Call the API to get members for this organization
            const membersResult = await api.getApiClient().v1OrganizationsMembersDetail(org.id)

            // Sort organization members by name in a case-insensitive manner
            const sortedMembers = [...membersResult.data].sort((a, b) => {
              // Handle any type issues by casting to any
              const aUser = a.user as any
              const bUser = b.user as any

              // Get the name from FullName, fullName, or email as fallback
              const aName = ((aUser?.FullName || aUser?.fullName || a.user?.email || '')).toLowerCase()
              const bName = ((bUser?.FullName || bUser?.fullName || b.user?.email || '')).toLowerCase()

              return aName.localeCompare(bName)
            })

            // Create a new object with the members field populated
            return {
              ...org,
              memberships: sortedMembers
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
      setLoading(false)
      setInitialized(true)
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
  const searchUsers = useCallback(async (query: { query: string, organizationId?: string }) => {
    try {
      const searchResult = await api.getApiClient().v1UsersSearchList({
        query: query.query,
        organization_id: query.organizationId
      })

      // Return the response data if it exists, otherwise return empty results
      if (searchResult.data) {
        return searchResult.data
      }

      return { users: [], pagination: { total: 0, limit: 0, offset: 0 } };
    } catch (error) {
      console.error('Error searching users:', error);
      const errorMessage = extractErrorMessage(error);
      snackbar.error(errorMessage || 'Error searching users');

      return { users: [], pagination: { total: 0, limit: 0, offset: 0 } };
    }
  }, [api, snackbar]);

  // Add this method to the useOrganizations hook
  const updateOrganizationMemberRole = useCallback(async (organizationId: string, userId: string, role: TypesOrganizationRole) => {
    if (!organizationId || !userId) {
      snackbar.error('Missing organization ID or user ID')
      return false
    }

    try {
      // Create a request to update the member's role
      const request = {
        role: role
      }

      // Call the API to update the member's role
      await api.getApiClient().v1OrganizationsMembersUpdate(organizationId, userId, request)

      // Reload the organization to get the updated memberships
      await loadOrganization(organizationId)

      return true
    } catch (error) {
      console.error(`Error updating member role in organization ${organizationId}:`, error)
      const errorMessage = extractErrorMessage(error)
      snackbar.error(errorMessage || 'Error updating member role')
      return false
    }
  }, [api, snackbar, loadOrganization])

  // App access grants methods
  const listAppAccessGrants = useCallback(async (appId: string): Promise<TypesAccessGrant[]> => {
    setLoadingAccessGrants(true)
    try {
      const response = await api.getApiClient().v1AppsAccessGrantsDetail(appId)
      setAppAccessGrants(response.data)
      return response.data
    } catch (error) {
      console.error('Error listing app access grants:', error)
      snackbar.error(`Failed to load app access grants: ${extractErrorMessage(error)}`)
      return []
    } finally {
      setLoadingAccessGrants(false)
    }
  }, [api, snackbar])

  const createAppAccessGrant = useCallback(async (appId: string, grant: TypesCreateAccessGrantRequest): Promise<boolean> => {
    try {
      await api.getApiClient().v1AppsAccessGrantsCreate(appId, grant)
      // Refresh the list to get updated data
      await listAppAccessGrants(appId)
      snackbar.success('Access grant created successfully')
      return true
    } catch (error) {
      console.error('Error creating app access grant:', error)
      snackbar.error(`Failed to create access grant: ${extractErrorMessage(error)}`)
      return false
    }
  }, [api, listAppAccessGrants, snackbar])

  const updateAppAccessGrant = useCallback(async (appId: string, grantId: string, grant: TypesCreateAccessGrantRequest): Promise<boolean> => {
    try {
      // Since there's no direct update endpoint, we need to delete and recreate
      // First delete the existing grant
      const deleteSuccess = await deleteAppAccessGrant(appId, grantId)
      if (!deleteSuccess) return false

      // Then create a new one with the updated roles
      const createSuccess = await createAppAccessGrant(appId, grant)
      if (createSuccess) {
        snackbar.success('Access grant updated successfully')
        return true
      }
      return false
    } catch (error) {
      console.error('Error updating app access grant:', error)
      snackbar.error(`Failed to update access grant: ${extractErrorMessage(error)}`)
      return false
    }
  }, [api, createAppAccessGrant, snackbar])

  const deleteAppAccessGrant = useCallback(async (appId: string, grantId: string): Promise<boolean> => {
    try {
      // The API client doesn't have this endpoint defined, so we'll make a direct axios call
      await api.delete(`/api/v1/apps/${appId}/access-grants/${grantId}`)
      
      // Update the local state by filtering out the deleted grant
      setAppAccessGrants(prev => prev.filter(grant => grant.id !== grantId))
      
      snackbar.success('Access grant removed successfully')
      return true
    } catch (error) {
      console.error('Error deleting app access grant:', error)
      snackbar.error(`Failed to remove access grant: ${extractErrorMessage(error)}`)
      return false
    }
  }, [api, snackbar])

  // Effect to load organization when orgIdParam changes
  useEffect(() => {
    if(!orgID) {
      setOrganization(undefined)
      return
    }

    if (orgID && initialized) {  
      const useOrg = organizations.find((org) => org.id === orgID || org.name === orgID)
      if (!useOrg || !useOrg.id) {
        setOrganization(undefined)
        return
      } else {
        loadOrganization(useOrg.id)
      }
    }
  }, [orgID, initialized])

  return {
    organizations,
    loading,
    orgID,
    organization,
    // Access grants state
    appAccessGrants,
    loadingAccessGrants,
    // Organization methods
    loadOrganizations,
    createOrganization,
    updateOrganization,
    deleteOrganization,
    loadOrganization,
    // Org member methods
    addMemberToOrganization,
    deleteMemberFromOrganization,
    updateOrganizationMemberRole,
    // Team methods
    createTeam,
    createTeamWithCreator,
    updateTeam,
    deleteTeam,
    // Team member methods
    addTeamMember,
    removeTeamMember,
    // User search
    searchUsers,
    // App access grants methods
    listAppAccessGrants,
    createAppAccessGrant,
    updateAppAccessGrant,
    deleteAppAccessGrant,
  }
} 