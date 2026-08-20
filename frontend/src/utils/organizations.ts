import { TypesOrganization, TypesOrganizationRole, TypesOrganizationMembership } from '../api/api'

export function orgLandingRoute(): string {
  return 'org_chat'
}

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

/**
 * Embed routes are a fullscreen, single-purpose view of one task or session,
 * often iframed on someone else's page. A scoped embed key deliberately
 * returns an empty org list, so org-recovery redirects must never fire there.
 */
export function isEmbedRouteName(routeName?: string): boolean {
  return Boolean(routeName?.startsWith('embed_'))
}

export type OrgAccessResolution =
  /** The org list hasn't loaded yet — decide nothing, clear nothing. */
  | { status: 'pending' }
  /** No org in the URL, or the URL org is one the API says we can see. */
  | { status: 'ok' }
  /**
   * The URL names an org that is not in our list: it was deleted, our
   * membership was revoked, or (the common case) a `selected_org` value left
   * in localStorage by a previously logged-in user on this browser.
   * `fallbackOrgSlug` is where to send the user instead; when undefined the
   * caller should fall back to the org picker.
   */
  | { status: 'inaccessible'; fallbackOrgSlug?: string }

/**
 * Decides whether the org referenced by the current URL is usable by the
 * signed-in user. The org list from `GET /organizations` is authoritative: for
 * a normal user it contains exactly their memberships; for an admin it
 * contains every org (with `member: false` on the ones they don't belong to),
 * and the API treats admins as members — so "present in the list" is the
 * correct test for both.
 */
export function resolveOrgAccess({
  orgID,
  organizations,
  orgListLoaded,
  routeName,
}: {
  orgID?: string
  organizations: TypesOrganization[]
  /**
   * True only once `GET /organizations` has actually succeeded. This is NOT
   * the same as the hook's `initialized`, which is set in a `finally` and so
   * is equally true after a failed list load — at which point `organizations`
   * is an empty array that means "we don't know", not "you have no orgs".
   * Treating those the same would evict a user from an org they can fully
   * access whenever the list request hits a 500 or a network blip.
   */
  orgListLoaded: boolean
  routeName?: string
}): OrgAccessResolution {
  // Never act on a list we haven't successfully loaded.
  if (!orgListLoaded) return { status: 'pending' }
  if (isEmbedRouteName(routeName)) return { status: 'ok' }
  if (!orgID) return { status: 'ok' }

  const match = organizations.find(org => org.id === orgID || org.name === orgID)
  if (match) return { status: 'ok' }

  return {
    status: 'inaccessible',
    fallbackOrgSlug: firstAccessibleOrgSlug(organizations, [orgID]),
  }
}

/**
 * The org to fall back to when the current one turns out to be unusable.
 * Admins see orgs they aren't a member of (`member === false`); those are a
 * poor landing choice, so prefer a real membership.
 *
 * `excludeOrgRefs` (ids or slugs) are skipped. It matters when the org list
 * still contains an org we already failed on — membership revoked between the
 * list load and the detail load — because landing back on it would reload it,
 * fail again, and redirect again. Excluding every org already known to be bad,
 * not just the current one, is what stops two such orgs ping-ponging forever.
 */
export function firstAccessibleOrgSlug(
  organizations: TypesOrganization[],
  excludeOrgRefs: string[] = []
): string | undefined {
  return organizations.find(org =>
    org.member !== false &&
    org.name &&
    !excludeOrgRefs.includes(org.id ?? '') &&
    !excludeOrgRefs.includes(org.name)
  )?.name
}

/**
 * True for the two API answers that mean "this org is not usable by you":
 * 403 (not a member) and 404 (org gone). Anything else — 401, 500, a network
 * blip — is transient and must NOT evict the user from their org.
 */
export function isOrgAccessDeniedError(error: unknown): boolean {
  const status = (error as { response?: { status?: number } })?.response?.status
  return status === 403 || status === 404
}
