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

export type ChatRailAction = {
  /** Move to the chat section. */
  navigate: boolean
  /** Leave the nav drawer open rather than closing it behind the navigation. */
  keepDrawerOpen: boolean
}

/**
 * What tapping "Chat" in the nav rail should do.
 *
 * Every other rail entry opens a page in the main area, so closing the drawer
 * behind it is right. Chat is the exception on a phone: the chat list lives IN
 * the drawer, so closing it lands the user on an empty composer with the list
 * they were reaching for hidden. And if they are already reading a thread,
 * navigating would throw it away to show them a blank one — so the tap only
 * opens the list.
 */
export function chatRailAction(
  isPhone: boolean,
  onChatRoute: boolean,
): ChatRailAction {
  if (!isPhone) return { navigate: true, keepDrawerOpen: false }
  return { navigate: !onChatRoute, keepDrawerOpen: true }
}
