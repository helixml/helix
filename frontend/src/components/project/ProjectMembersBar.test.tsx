import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import ProjectMembersBar from './ProjectMembersBar'
import { TypesAction, TypesEffect, TypesResource } from '../../api/api'

const mockV1OrganizationsInvitationsDetail = vi.fn()
const mockLoadOrganization = vi.fn()

const stableApiClient = {
  v1OrganizationsInvitationsDetail: mockV1OrganizationsInvitationsDetail,
}
const stableGetApiClient = () => stableApiClient

vi.mock('../../hooks/useApi', () => ({
  default: () => ({
    getApiClient: stableGetApiClient,
  }),
}))

vi.mock('../../hooks/useAccount', () => ({
  default: () => ({
    organizationTools: {
      orgID: 'org_1',
      loadOrganization: mockLoadOrganization,
      organization: {
        id: 'org_1',
        roles: [],
        teams: [],
      },
    },
  }),
}))

vi.mock('../../hooks/useRouter', () => ({
  default: () => ({
    params: {},
    navigate: vi.fn(),
  }),
}))

vi.mock('../../hooks/useLightTheme', () => ({
  default: () => ({
    panelColor: '#20232b',
    highlightColor: '#272b35',
  }),
}))

vi.mock('../../hooks/useDebounce', () => ({
  default: <T,>(value: T) => value,
}))

vi.mock('../widgets/DeleteConfirmWindow', () => ({
  default: () => null,
}))

describe('ProjectMembersBar access dialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockV1OrganizationsInvitationsDetail.mockResolvedValue({ data: [] })
  })

  it('shows the project owner separately from an invited admin who is viewing the dialog', async () => {
    render(
      <ProjectMembersBar
        currentUser={{ id: 'user_invited', full_name: 'nessie', email: 'karolis+0604@helix.ml' }}
        projectOwnerId="user_owner"
        projectOwner={{ id: 'user_owner', full_name: 'karolis', email: 'karolis@helix.ml' }}
        projectId="proj_1"
        organizationId="org_1"
        accessGrants={[
          {
            id: 'grant_invited_admin',
            organization_id: 'org_1',
            resource_id: 'proj_1',
            user_id: 'user_invited',
            user: { id: 'user_invited', full_name: 'nessie', email: 'karolis+0604@helix.ml' },
            roles: [
              {
                id: 'role_admin',
                name: 'admin',
                config: {
                  rules: [
                    {
                      effect: TypesEffect.EffectAllow,
                      resource: [TypesResource.ResourceAny],
                      actions: [
                        TypesAction.ActionGet,
                        TypesAction.ActionList,
                        TypesAction.ActionCreate,
                        TypesAction.ActionDelete,
                      ],
                    },
                  ],
                },
              },
            ],
          },
        ]}
        inviteOpen
        onOpenInvite={vi.fn()}
        onCloseInvite={vi.fn()}
        onCreateGrant={vi.fn()}
        onDeleteGrant={vi.fn()}
      />,
    )

    await waitFor(() => {
      expect(mockV1OrganizationsInvitationsDetail).toHaveBeenCalledWith('org_1', { app_id: 'proj_1' })
    })

    expect(screen.getByText('karolis')).toBeInTheDocument()
    expect(screen.getByText('karolis@helix.ml')).toBeInTheDocument()
    expect(screen.getByText('nessie')).toBeInTheDocument()
    expect(screen.getByText('karolis+0604@helix.ml')).toBeInTheDocument()
    expect(screen.getByText('Owner')).toBeInTheDocument()
    expect(screen.getByText('Admin')).toBeInTheDocument()
    expect(screen.getByText('Actions')).toBeInTheDocument()
  })
})
