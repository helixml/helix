import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor, act } from '@testing-library/react'
import Onboarding from './Onboarding'

const mockNavigateReplace = vi.fn()
const mockNavigate = vi.fn()
const mockSnackbarError = vi.fn()
const mockSnackbarSuccess = vi.fn()
const mockCreateAgent = vi.fn()
const mockLoadOrganizations = vi.fn()

const mockV1UsersMeOnboardingCreate = vi.fn().mockResolvedValue({})
const mockV1GitRepositoriesCreate = vi.fn()
const mockV1ProjectsCreate = vi.fn()
const mockV1SpecTasksFromPromptCreate = vi.fn()
const mockApiGet = vi.fn()
const mockCreateOrgMutateAsync = vi.fn()

let mockAccountValue: any = {
  user: { id: 'user-1', name: 'Test User', email: 'test@example.com' },
  organizationTools: {
    organizations: [],
    loading: false,
    orgID: '',
    loadOrganizations: mockLoadOrganizations,
  },
}

vi.mock('../hooks/useAccount', () => ({
  default: () => mockAccountValue,
}))

vi.mock('../hooks/useApi', () => ({
  default: () => ({
    get: mockApiGet,
    getApiClient: () => ({
      v1UsersMeOnboardingCreate: mockV1UsersMeOnboardingCreate,
      v1GitRepositoriesCreate: mockV1GitRepositoriesCreate,
      v1ProjectsCreate: mockV1ProjectsCreate,
      v1SpecTasksFromPromptCreate: mockV1SpecTasksFromPromptCreate,
    }),
  }),
}))

vi.mock('../hooks/useApps', () => ({
  default: () => ({
    apps: [],
    createAgent: mockCreateAgent,
  }),
}))

vi.mock('../hooks/useSnackbar', () => ({
  default: () => ({
    error: mockSnackbarError,
    success: mockSnackbarSuccess,
    info: vi.fn(),
  }),
}))

vi.mock('../hooks/useRouter', () => ({
  default: () => ({
    name: 'onboarding',
    params: {},
    meta: {},
    navigate: mockNavigate,
    navigateReplace: mockNavigateReplace,
    setParams: vi.fn(),
    mergeParams: vi.fn(),
    replaceParams: vi.fn(),
    removeParams: vi.fn(),
  }),
}))

vi.mock('../services/orgService', () => ({
  useCreateOrg: () => ({
    mutateAsync: mockCreateOrgMutateAsync,
    isPending: false,
  }),
}))

vi.mock('../components/create/AdvancedModelPicker', () => ({
  AdvancedModelPicker: ({ onSelectModel }: any) => (
    <button data-testid="model-picker" onClick={() => onSelectModel('test-provider', 'claude-sonnet-4-5-20250929')}>
      Pick Model
    </button>
  ),
}))

vi.mock('../components/project/BrowseProvidersDialog', () => ({
  default: () => null,
}))

vi.mock('../contexts/apps', () => ({
  CodeAgentRuntime: 'zed_agent',
  generateAgentName: (model: string, runtime: string) => `${model} in ${runtime}`,
}))

vi.mock('lucide-react', () => ({
  Bot: () => <span data-testid="bot-icon" />,
}))

function setAccountWithOrgs(orgs: any[]) {
  mockAccountValue = {
    user: { id: 'user-1', name: 'Test User', email: 'test@example.com' },
    organizationTools: {
      organizations: orgs,
      loading: false,
      orgID: '',
      loadOrganizations: mockLoadOrganizations,
    },
  }
}

async function selectExistingOrgAndGoToStep2() {
  fireEvent.click(screen.getByRole('button', { name: /continue with this organization/i }))
  await waitFor(() => {
    expect(screen.getByLabelText(/project name/i)).toBeInTheDocument()
  })
}

async function fillProjectAndCreateWithModel(projectName: string) {
  fireEvent.change(screen.getByLabelText(/project name/i), { target: { value: projectName } })
  fireEvent.click(screen.getByTestId('model-picker'))
  await act(async () => {
    fireEvent.click(screen.getByRole('button', { name: /create project/i }))
  })
}

describe('Onboarding', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockApiGet.mockResolvedValue([])
    setAccountWithOrgs([])
  })

  describe('selecting an existing organization', () => {
    it('should set org ID when selecting an existing org and pass it to subsequent API calls', async () => {
      setAccountWithOrgs([
        { id: 'org-123', name: 'my-org', display_name: 'My Org' },
        { id: 'org-456', name: 'other-org', display_name: 'Other Org' },
      ])

      render(<Onboarding />)

      await selectExistingOrgAndGoToStep2()

      mockV1GitRepositoriesCreate.mockResolvedValue({ data: { id: 'repo-1' } })
      mockV1ProjectsCreate.mockResolvedValue({ data: { id: 'project-1' } })
      mockCreateAgent.mockResolvedValue({ id: 'agent-1' })

      await fillProjectAndCreateWithModel('test-project')

      await waitFor(() => {
        expect(mockCreateAgent).toHaveBeenCalledWith(
          expect.objectContaining({
            organizationId: 'org-123',
          })
        )
      })

      expect(mockV1GitRepositoriesCreate).toHaveBeenCalledWith(
        expect.objectContaining({
          organization_id: 'org-123',
        })
      )

      expect(mockV1ProjectsCreate).toHaveBeenCalledWith(
        expect.objectContaining({
          organization_id: 'org-123',
        })
      )
    })
  })

  describe('creating a new organization', () => {
    it('should set org ID from newly created org and pass it to subsequent API calls', async () => {
      setAccountWithOrgs([])

      mockCreateOrgMutateAsync.mockResolvedValue({
        id: 'new-org-id',
        name: 'new-org-slug',
        display_name: 'New Org',
      })

      render(<Onboarding />)

      const orgNameInput = screen.getByLabelText(/organization name/i)
      fireEvent.change(orgNameInput, { target: { value: 'New Org' } })

      const createOrgBtn = screen.getByRole('button', { name: /create organization/i })
      await act(async () => {
        fireEvent.click(createOrgBtn)
      })

      await waitFor(() => {
        expect(mockCreateOrgMutateAsync).toHaveBeenCalledWith({
          display_name: 'New Org',
        })
      })

      await waitFor(() => {
        expect(screen.getByLabelText(/project name/i)).toBeInTheDocument()
      })

      mockV1GitRepositoriesCreate.mockResolvedValue({ data: { id: 'repo-2' } })
      mockV1ProjectsCreate.mockResolvedValue({ data: { id: 'project-2' } })
      mockCreateAgent.mockResolvedValue({ id: 'agent-2' })

      await fillProjectAndCreateWithModel('my-project')

      await waitFor(() => {
        expect(mockCreateAgent).toHaveBeenCalledWith(
          expect.objectContaining({
            organizationId: 'new-org-id',
          })
        )
      })

      expect(mockV1GitRepositoriesCreate).toHaveBeenCalledWith(
        expect.objectContaining({
          organization_id: 'new-org-id',
        })
      )

      expect(mockV1ProjectsCreate).toHaveBeenCalledWith(
        expect.objectContaining({
          organization_id: 'new-org-id',
        })
      )
    })
  })

  describe('org ID propagation to all API calls', () => {
    it('should pass org ID to repository creation', async () => {
      setAccountWithOrgs([{ id: 'org-abc', name: 'test-org', display_name: 'Test Org' }])

      mockV1GitRepositoriesCreate.mockResolvedValue({ data: { id: 'repo-id' } })
      mockV1ProjectsCreate.mockResolvedValue({ data: { id: 'proj-id' } })
      mockCreateAgent.mockResolvedValue({ id: 'agent-id' })

      render(<Onboarding />)

      await selectExistingOrgAndGoToStep2()
      await fillProjectAndCreateWithModel('repo-test')

      await waitFor(() => {
        expect(mockV1GitRepositoriesCreate).toHaveBeenCalledWith(
          expect.objectContaining({
            name: 'repo-test',
            organization_id: 'org-abc',
            owner_id: 'user-1',
          })
        )
      })
    })

    it('should pass org ID to project creation', async () => {
      setAccountWithOrgs([{ id: 'org-xyz', name: 'proj-org', display_name: 'Proj Org' }])

      mockV1GitRepositoriesCreate.mockResolvedValue({ data: { id: 'repo-99' } })
      mockV1ProjectsCreate.mockResolvedValue({ data: { id: 'proj-99' } })
      mockCreateAgent.mockResolvedValue({ id: 'agent-99' })

      render(<Onboarding />)

      await selectExistingOrgAndGoToStep2()
      await fillProjectAndCreateWithModel('proj-test')

      await waitFor(() => {
        expect(mockV1ProjectsCreate).toHaveBeenCalledWith(
          expect.objectContaining({
            name: 'proj-test',
            organization_id: 'org-xyz',
          })
        )
      })
    })

    it('should pass org ID to agent creation', async () => {
      setAccountWithOrgs([{ id: 'org-agent', name: 'agent-org', display_name: 'Agent Org' }])

      mockV1GitRepositoriesCreate.mockResolvedValue({ data: { id: 'r-1' } })
      mockV1ProjectsCreate.mockResolvedValue({ data: { id: 'p-1' } })
      mockCreateAgent.mockResolvedValue({ id: 'a-1' })

      render(<Onboarding />)

      await selectExistingOrgAndGoToStep2()
      await fillProjectAndCreateWithModel('agent-test')

      await waitFor(() => {
        expect(mockCreateAgent).toHaveBeenCalledWith(
          expect.objectContaining({
            organizationId: 'org-agent',
            model: 'claude-sonnet-4-5-20250929',
          })
        )
      })
    })
  })

  describe('completion and localStorage', () => {
    it('should store org name in localStorage on completion', async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true })

      setAccountWithOrgs([{ id: 'org-ls', name: 'ls-org', display_name: 'LS Org' }])

      mockV1GitRepositoriesCreate.mockResolvedValue({ data: { id: 'repo-ls' } })
      mockV1ProjectsCreate.mockResolvedValue({ data: { id: 'proj-ls' } })
      mockCreateAgent.mockResolvedValue({ id: 'agent-ls' })
      mockV1SpecTasksFromPromptCreate.mockResolvedValue({ data: { id: 'task-1' } })

      render(<Onboarding />)

      await selectExistingOrgAndGoToStep2()
      await fillProjectAndCreateWithModel('ls-project')

      await waitFor(() => {
        expect(screen.getByLabelText(/what would you like to build/i)).toBeInTheDocument()
      })

      fireEvent.change(screen.getByLabelText(/what would you like to build/i), {
        target: { value: 'Build a REST API' },
      })

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /create task/i }))
      })

      await waitFor(() => {
        expect(mockV1SpecTasksFromPromptCreate).toHaveBeenCalledWith(
          expect.objectContaining({
            prompt: 'Build a REST API',
            project_id: 'proj-ls',
          })
        )
      })

      await act(async () => {
        vi.advanceTimersByTime(2000)
      })

      await waitFor(() => {
        expect(localStorage.getItem('selected_org')).toBe('ls-org')
      })

      vi.useRealTimers()
    })

    it('should navigate to org projects on completion with created project', async () => {
      vi.useFakeTimers({ shouldAdvanceTime: true })

      setAccountWithOrgs([{ id: 'org-nav', name: 'nav-org', display_name: 'Nav Org' }])

      mockV1GitRepositoriesCreate.mockResolvedValue({ data: { id: 'repo-nav' } })
      mockV1ProjectsCreate.mockResolvedValue({ data: { id: 'proj-nav' } })
      mockCreateAgent.mockResolvedValue({ id: 'agent-nav' })
      mockV1SpecTasksFromPromptCreate.mockResolvedValue({ data: { id: 'task-nav' } })

      render(<Onboarding />)

      await selectExistingOrgAndGoToStep2()
      await fillProjectAndCreateWithModel('nav-project')

      await waitFor(() => {
        expect(screen.getByLabelText(/what would you like to build/i)).toBeInTheDocument()
      })

      fireEvent.change(screen.getByLabelText(/what would you like to build/i), {
        target: { value: 'Build something' },
      })

      await act(async () => {
        fireEvent.click(screen.getByRole('button', { name: /create task/i }))
      })

      await waitFor(() => {
        expect(mockV1SpecTasksFromPromptCreate).toHaveBeenCalled()
      })

      await act(async () => {
        vi.advanceTimersByTime(2000)
      })

      await waitFor(() => {
        expect(mockV1UsersMeOnboardingCreate).toHaveBeenCalled()
      })

      expect(mockNavigateReplace).toHaveBeenCalledWith('org_projects', { org_id: 'nav-org' })

      vi.useRealTimers()
    })
  })

  describe('skip onboarding', () => {
    it('should mark onboarding complete and navigate to projects when skipping', async () => {
      setAccountWithOrgs([])

      render(<Onboarding />)

      const closeButtons = screen.getAllByRole('button')
      const skipBtn = closeButtons[0]
      await act(async () => {
        fireEvent.click(skipBtn)
      })

      await waitFor(() => {
        expect(mockV1UsersMeOnboardingCreate).toHaveBeenCalled()
      })

      expect(mockNavigateReplace).toHaveBeenCalledWith('projects')
    })
  })

  describe('error handling without org ID', () => {
    it('should not show create project button when not on step 2', async () => {
      setAccountWithOrgs([])

      render(<Onboarding />)

      expect(screen.queryByRole('button', { name: /create project/i })).not.toBeInTheDocument()
    })
  })

  describe('fetching org apps after org selection', () => {
    it('should fetch apps for the selected org when moving to step 2', async () => {
      setAccountWithOrgs([{ id: 'org-apps', name: 'apps-org', display_name: 'Apps Org' }])

      mockApiGet.mockResolvedValue([])

      render(<Onboarding />)

      await selectExistingOrgAndGoToStep2()

      await waitFor(() => {
        expect(mockApiGet).toHaveBeenCalledWith(
          '/api/v1/apps',
          expect.objectContaining({
            params: { organization_id: 'org-apps' },
          }),
          expect.anything()
        )
      })
    })
  })
})
