import { TypesOrganization, TypesOrganizationRole } from '../api/api'

/**
 * Checks if a user is an owner of a specific organization
 * @param organization - An organization object with memberships
 * @param userId - ID of the user to check
 * @returns True if the user is an owner of the organization, false otherwise
 */
export function isUserOwnerOfOrganization(
  organization: TypesOrganization,
  userId: string
): boolean {
  // If organization not found or has no memberships, return false
  if (!organization.memberships) {
    return false
  }
  
  // Find the user's membership in the organization
  const userMembership = organization.memberships.find(
    membership => membership.user_id === userId
  )
  
  // Check if the user has the owner role
  return userMembership?.role === TypesOrganizationRole.OrganizationRoleOwner
} 