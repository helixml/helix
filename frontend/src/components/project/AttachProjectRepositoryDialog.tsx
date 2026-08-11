import { FC } from 'react'

import {
  TypesExternalRepositoryType,
  TypesGitRepositoryType,
} from '../../api/api'
import type {
  TypesAzureDevOps,
  TypesGitRepository,
  TypesRepositoryInfo,
} from '../../api/api'
import useAccount from '../../hooks/useAccount'
import useSnackbar from '../../hooks/useSnackbar'
import { useAttachRepositoryToProject } from '../../services'
import {
  useCreateGitRepository,
  useGitRepositories,
} from '../../services/gitRepositoryService'
import BrowseProvidersDialog from './BrowseProvidersDialog'

interface AttachProjectRepositoryDialogProps {
  open: boolean
  onClose: () => void
  projectId: string
  attachedRepositories: TypesGitRepository[]
}

const normalizeRepositoryUrl = (url?: string) =>
  (url || '').trim().replace(/\.git$/, '').replace(/\/$/, '').toLowerCase()

const AttachProjectRepositoryDialog: FC<AttachProjectRepositoryDialogProps> = ({
  open,
  onClose,
  projectId,
  attachedRepositories,
}) => {
  const account = useAccount()
  const snackbar = useSnackbar()
  const organizationId = account.organizationTools.organization?.id || ''
  const userId = account.user?.id || ''
  const { data: repositories = [], isLoading: repositoriesLoading } =
    useGitRepositories({
      organizationId,
      enabled: open && !!organizationId,
    })
  const createRepository = useCreateGitRepository()
  const attachRepository = useAttachRepositoryToProject(projectId)

  const attachedRepositoryIds = new Set(
    attachedRepositories.map(repository => repository.id).filter(Boolean),
  )
  const availableRepositories = repositories.filter(
    repository =>
      repository.repo_type !== TypesGitRepositoryType.GitRepositoryTypeInternal
      && !attachedRepositoryIds.has(repository.id),
  )

  const attach = async (repositoryId: string) => {
    try {
      await attachRepository.mutateAsync(repositoryId)
      snackbar.success('Repository attached successfully')
      onClose()
    } catch (error) {
      snackbar.error('Failed to attach repository')
    }
  }

  const handleSelectExistingRepository = async (
    repository: TypesGitRepository,
  ) => {
    if (!repository.id) return
    await attach(repository.id)
  }

  const handleSelectExternalRepository = async (
    repository: TypesRepositoryInfo,
    providerTypeOrCredentials: string,
    oauthConnectionId?: string,
    gitProviderConnectionId?: string,
  ) => {
    const repositoryUrl = repository.clone_url || repository.html_url || ''
    const normalizedUrl = normalizeRepositoryUrl(repositoryUrl)
    const existingRepository = repositories.find(candidate =>
      candidate.is_external
      && normalizeRepositoryUrl(candidate.external_url) === normalizedUrl,
    )

    if (existingRepository?.id) {
      await attach(existingRepository.id)
      return
    }

    let providerType = providerTypeOrCredentials
    let credentials: {
      pat?: string
      username?: string
      orgUrl?: string
    } | null = null
    if (providerTypeOrCredentials.startsWith('{')) {
      credentials = JSON.parse(providerTypeOrCredentials)
      providerType = (credentials as { type?: string }).type || 'github'
    }

    const externalTypes: Record<string, TypesExternalRepositoryType> = {
      github: TypesExternalRepositoryType.ExternalRepositoryTypeGitHub,
      gitlab: TypesExternalRepositoryType.ExternalRepositoryTypeGitLab,
      'azure-devops': TypesExternalRepositoryType.ExternalRepositoryTypeADO,
      bitbucket: TypesExternalRepositoryType.ExternalRepositoryTypeBitbucket,
    }
    const externalType = externalTypes[providerType]
    if (!externalType || !userId || !organizationId) {
      snackbar.error('Failed to link repository')
      return
    }

    try {
      const linkedRepository = await createRepository.mutateAsync({
        name:
          repository.name
          || repository.full_name?.split('/').pop()
          || 'repository',
        description: repository.description || `External ${providerType} repository`,
        owner_id: userId,
        organization_id: organizationId,
        repo_type: TypesGitRepositoryType.GitRepositoryTypeCode,
        default_branch: repository.default_branch || 'main',
        is_external: true,
        external_url: repositoryUrl,
        external_type: externalType,
        username: credentials?.username,
        password: credentials?.pat,
        azure_devops:
          providerType === 'azure-devops'
            ? ({
                organization_url: credentials?.orgUrl || '',
                personal_access_token: credentials?.pat || '',
              } satisfies TypesAzureDevOps)
            : undefined,
        oauth_connection_id: oauthConnectionId,
        git_provider_connection_id: gitProviderConnectionId,
      })
      if (!linkedRepository?.id) {
        snackbar.error('Failed to link repository')
        return
      }
      await attach(linkedRepository.id)
    } catch (error) {
      snackbar.error('Failed to link repository')
    }
  }

  return (
    <BrowseProvidersDialog
      open={open}
      onClose={onClose}
      onSelectRepository={handleSelectExternalRepository}
      onSelectHelixRepository={handleSelectExistingRepository}
      helixRepositories={availableRepositories}
      helixRepositoriesLoading={repositoriesLoading}
      repositorySourceLabel="Existing repositories"
      isLinking={createRepository.isPending || attachRepository.isPending}
      organizationName={
        account.organizationTools.organization?.display_name
        || account.organizationTools.organization?.name
      }
    />
  )
}

export default AttachProjectRepositoryDialog
