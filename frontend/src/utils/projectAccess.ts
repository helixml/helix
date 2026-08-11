import {
  TypesAccessGrant,
  TypesAction,
  TypesEffect,
  TypesResource,
} from '../api/api'

type UserSummary = { id?: string; admin?: boolean }

function grantAllowsAccessGrantManagement(grant: TypesAccessGrant): boolean {
  return (grant.roles || []).some((role) => {
    if (role.name?.toLowerCase() === 'admin') {
      return true
    }

    return (role.config?.rules || []).some((rule) => {
      if (rule.effect !== TypesEffect.EffectAllow) {
        return false
      }

      const resources = rule.resource || []
      const actions = rule.actions || []
      return (
        resources.includes(TypesResource.ResourceAccessGrants)
        || resources.includes(TypesResource.ResourceAny)
      ) && actions.includes(TypesAction.ActionCreate)
        && actions.includes(TypesAction.ActionDelete)
    })
  })
}

export function canManageProjectAccess(
  currentUser: UserSummary | undefined,
  projectOwnerId: string | undefined,
  organizationOwnerId: string | undefined,
  accessGrants: TypesAccessGrant[],
): boolean {
  if (!currentUser?.id) {
    return false
  }

  return currentUser.id === projectOwnerId
    || currentUser.id === organizationOwnerId
    || !!currentUser.admin
    || accessGrants.some((grant) => (
      grant.user_id === currentUser.id && grantAllowsAccessGrantManagement(grant)
    ))
}
