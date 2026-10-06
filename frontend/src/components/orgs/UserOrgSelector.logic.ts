export function isNavigationRouteActive(
  routeName: string,
  path: string | string[],
): boolean {
  const paths = Array.isArray(path) ? path : [path]
  return paths.some((candidate) =>
    routeName === candidate ||
    routeName === `org_${candidate}` ||
    routeName.startsWith(`${candidate}-`) ||
    routeName.startsWith(`org_${candidate}-`)
  )
}

export function isOrgProjectSettingsRoute(
  routeName: string,
  tab?: string,
): boolean {
  return (
    routeName === 'org_projects' &&
    (tab === 'repositories' || tab === 'guidelines')
  )
}
