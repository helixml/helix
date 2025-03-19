import { TypesOrganization, TypesOrganizationRole, TypesOrganizationMembership } from '../api/api'

/**
 * Gets a user's membership in an organization
 * @param organization - An organization object with memberships
 * @param userId - ID of the user to check
 * @returns The user's membership or undefined if not found
 */
export function getUserMembership(
  organization: TypesOrganization,
  userId: string
): TypesOrganizationMembership | undefined {
  // Return undefined if organization has no memberships
  if (!organization.memberships) {
    return undefined
  }
  
  // Find and return the user's membership
  return organization.memberships.find(
    membership => membership.user_id === userId
  )
}

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
  // Get user membership and check if role is owner
  const userMembership = getUserMembership(organization, userId)
  return userMembership?.role === TypesOrganizationRole.OrganizationRoleOwner
}

/**
 * Checks if a user is a member of a specific organization
 * @param organization - An organization object with memberships
 * @param userId - ID of the user to check
 * @returns True if the user is a member of the organization, false otherwise
 */
export function isUserMemberOfOrganization(
  organization: TypesOrganization,
  userId: string
): boolean {
  // User is a member if they have any membership role
  return getUserMembership(organization, userId) !== undefined
} 