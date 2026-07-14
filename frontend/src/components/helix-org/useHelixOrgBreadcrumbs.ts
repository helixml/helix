// useHelixOrgBreadcrumbs is the single source of truth for the breadcrumb
// trail on helix-org pages. It returns the leading crumbs to hand to
// Page's `breadcrumbs` prop: the org name (linking to the chart) followed
// by an optional section crumb (linking to that section's list page). The
// page supplies the leaf via `breadcrumbTitle`.
//
// Examples:
//   Chart:     [org]                         (no leaf — org is current)
//   Settings:  [org] + title "Settings"      →  org / Settings
//   Bots list: [org] + title "Bots"          →  org / Bots
//   Bot detail:[org, Bots→list] + bot name   →  org / Bots / bot-name
//   Topic det: [org, Topics→list] + topic    →  org / Topics / topic-id
//
// Every crumb navigates via the plain router (useOrgRouter: false).
// helix-org routes are named `helix_org_*`, which account.orgNavigate
// would mangle by prepending `org_` (turning `helix_org_chart` into the
// non-existent `org_helix_org_chart`) — so the org router must not be used.
//
// Because this hook supplies the org crumb itself, pages using it should
// NOT also set `orgBreadcrumbs` (which would inject a second, plain-text
// org crumb).

import { IPageBreadcrumb } from '../../types'
import useAccount from '../../hooks/useAccount'
import useRouter from '../../hooks/useRouter'

export interface HelixOrgBreadcrumbSection {
  // Display label, e.g. "Bots".
  title: string
  // Route name of the section's list page, e.g. "helix_org_bots".
  routeName: string
}

export function useHelixOrgBreadcrumbs(section?: HelixOrgBreadcrumbSection): IPageBreadcrumb[] {
  const account = useAccount()
  const { params } = useRouter()
  const orgSlug = (params.org_id as string) || ''
  const org = account.organizationTools.organization
  // Prefer human display name, then slug/name, then URL segment.
  const orgName =
    (org as { display_name?: string } | undefined)?.display_name
    || org?.name
    || orgSlug

  const crumbs: IPageBreadcrumb[] = [
    {
      title: orgName,
      routeName: 'helix_org_chart',
      params: { org_id: orgSlug },
      useOrgRouter: false,
    },
  ]
  if (section) {
    crumbs.push({
      title: section.title,
      routeName: section.routeName,
      params: { org_id: orgSlug },
      useOrgRouter: false,
    })
  }
  return crumbs
}

export default useHelixOrgBreadcrumbs
