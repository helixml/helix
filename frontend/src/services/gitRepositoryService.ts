import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query';
import useApi from '../hooks/useApi';
import type { TypesUpdateGitRepositoryFileContentsRequest as UpdateGitRepositoryFileContentsRequest } from '../api/api';

// Re-export generated types for convenience
export type {
  TypesGitRepository as GitRepository,
  TypesGitRepositoryCreateRequest as GitRepositoryCreateRequest,
  TypesGitRepositoryType as GitRepositoryType,
  TypesGitRepositoryStatus as GitRepositoryStatus,
  TypesCreateSampleRepositoryRequest as CreateSampleRepositoryRequest,
  TypesUpdateGitRepositoryFileContentsRequest as UpdateGitRepositoryFileContentsRequest,
  TypesGitRepositoryFileResponse as GitRepositoryFileResponse,
} from '../api/api';

// Query keys
export const QUERY_KEYS = {
  gitRepositories: ['git-repositories'] as const,
  gitRepository: (id: string) => ['git-repositories', id] as const,
  sampleTypes: ['git-repositories', 'sample-types'] as const,
  cloneCommand: (id: string) => ['git-repositories', id, 'clone-command'] as const,
  userRepositories: (userId: string) => ['git-repositories', 'user', userId] as const,
  specTaskRepositories: (specTaskId: string) => ['git-repositories', 'spec-task', specTaskId] as const,
  repositoryTree: (id: string, path: string, branch?: string) => ['git-repositories', id, 'tree', path, branch || ''] as const,
  repositoryFile: (id: string, path: string, branch?: string) => ['git-repositories', id, 'file', path, branch || ''] as const,
};

// Custom hooks for git repository operations

export function useGitRepositories(ownerId?: string, repoType?: string) {
  const api = useApi();

  return useQuery({
    queryKey: [...QUERY_KEYS.gitRepositories, ownerId, repoType],
    queryFn: async () => {
      const response = await api.getApiClient().v1GitRepositoriesList({
        owner_id: ownerId,
        repo_type: repoType,
      });
      // Sort repositories by created_at descending (newest first)
      const repos = response.data || [];
      return repos.sort((a: any, b: any) => {
        const dateA = a.created_at ? new Date(a.created_at).getTime() : 0;
        const dateB = b.created_at ? new Date(b.created_at).getTime() : 0;
        return dateB - dateA; // Descending order
      });
    },
  });
}

export function useGitRepository(repositoryId: string) {
  const api = useApi();
  
  return useQuery({
    queryKey: QUERY_KEYS.gitRepository(repositoryId),
    queryFn: async () => {
      const response = await api.getApiClient().v1GitRepositoriesDetail(repositoryId);
      return response.data;
    },
    enabled: !!repositoryId,
  });
}

export function useSampleTypes() {
  const api = useApi();
  
  return useQuery({
    queryKey: QUERY_KEYS.sampleTypes,
    queryFn: async () => {
      const response = await api.getApiClient().v1SpecsSampleTypesList();
      return response.data;
    },
    staleTime: 5 * 60 * 1000, // Cache for 5 minutes
  });
}

export function useCloneCommand(repositoryId: string, targetDir?: string) {
  const api = useApi();
  
  return useQuery({
    queryKey: [...QUERY_KEYS.cloneCommand(repositoryId), targetDir],
    queryFn: async () => {
      const response = await api.getApiClient().v1GitRepositoriesCloneCommandDetail(repositoryId, {
        target_dir: targetDir,
      });
      return response.data;
    },
    enabled: !!repositoryId,
  });
}

export function useUserGitRepositories(userId: string) {
  const api = useApi();

  return useQuery({
    queryKey: QUERY_KEYS.userRepositories(userId),
    queryFn: async () => {
      const response = await api.getApiClient().v1GitRepositoriesList({
        owner_id: userId,
      });
      return response.data;
    },
    enabled: !!userId,
  });
}

export function useListRepositoryBranches(repositoryId: string) {
  const api = useApi();

  return useQuery({
    queryKey: ['git-repositories', repositoryId, 'branches'] as const,
    queryFn: async () => {
      const response = await api.getApiClient().listGitRepositoryBranches(repositoryId);
      return response.data;
    },
    enabled: !!repositoryId,
  });
}

export function useBrowseRepositoryTree(repositoryId: string, path: string = '.', branch: string = '') {
  const api = useApi();

  return useQuery({
    queryKey: QUERY_KEYS.repositoryTree(repositoryId, path, branch),
    queryFn: async () => {
      const params: any = { path };
      if (branch) {
        params.branch = branch;
      }
      const response = await api.getApiClient().browseGitRepositoryTree(repositoryId, params);
      return response.data;
    },
    enabled: !!repositoryId,
  });
}

export function useGetRepositoryFile(repositoryId: string, path: string, branch: string = '', enabled: boolean = false) {
  const api = useApi();

  return useQuery({
    queryKey: QUERY_KEYS.repositoryFile(repositoryId, path, branch),
    queryFn: async () => {
      const params: any = { path };
      if (branch) {
        params.branch = branch;
      }
      const response = await api.getApiClient().getGitRepositoryFile(repositoryId, params);
      return response.data;
    },
    enabled: enabled && !!repositoryId && !!path,
  });
}

// Mutation hooks

export function useCreateGitRepository() {
  const api = useApi();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (request: any) => { // ServicesGitRepositoryCreateRequest
      const response = await api.getApiClient().v1GitRepositoriesCreate(request);
      return response.data;
    },
    onSuccess: (_, variables) => {
      // Invalidate relevant queries
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.gitRepositories });
      if (variables.owner_id) {
        queryClient.invalidateQueries({ queryKey: QUERY_KEYS.userRepositories(variables.owner_id) });
      }
    },
  });
}

export function useDeleteGitRepository() {
  const api = useApi();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (repositoryId: string) => {
      await api.getApiClient().v1GitRepositoriesDelete(repositoryId);
    },
    onSuccess: () => {
      // Invalidate all repository queries
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.gitRepositories });
    },
  });
}

export function useCreateSampleRepository() {
  const api = useApi();
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: async (request: any) => { // ServerCreateSampleRepositoryRequest
      const response = await api.getApiClient().v1SamplesRepositoriesCreate(request);
      return response.data;
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.gitRepositories });
      if (variables.owner_id) {
        queryClient.invalidateQueries({ queryKey: QUERY_KEYS.userRepositories(variables.owner_id) });
      }
    },
  });
}


export function useInitializeSampleRepositories() {
  const api = useApi();
  const queryClient = useQueryClient();
  
  return useMutation({
    mutationFn: async (request: any) => { // ServerInitializeSampleRepositoriesRequest
      const response = await api.getApiClient().v1SamplesInitializeCreate(request);
      return response.data;
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ queryKey: QUERY_KEYS.gitRepositories });
      if (variables.owner_id) {
        queryClient.invalidateQueries({ queryKey: QUERY_KEYS.userRepositories(variables.owner_id) });
      }
    },
  });
}

export function useCreateOrUpdateRepositoryFile() {
  const api = useApi();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ repositoryId, request }: { repositoryId: string; request: UpdateGitRepositoryFileContentsRequest }) => {
      const response = await api.getApiClient().createOrUpdateGitRepositoryFileContents(repositoryId, request);
      return response.data;
    },
    onSuccess: (_, variables) => {
      // Invalidate the specific file query to refresh the file content
      queryClient.invalidateQueries({ 
        queryKey: QUERY_KEYS.repositoryFile(variables.repositoryId, variables.request.path || '', variables.request.branch || '') 
      });
      // Calculate parent directory of the file being created/updated
      const filePath = variables.request.path || ''
      const filePathParts = filePath.split('/').filter(p => p)
      const parentDir = filePathParts.length > 1 
        ? filePathParts.slice(0, -1).join('/')
        : '.'
      // Invalidate the tree query for the parent directory to refresh the file listing
      queryClient.invalidateQueries({ 
        queryKey: QUERY_KEYS.repositoryTree(variables.repositoryId, parentDir, variables.request.branch || '') 
      });
    },
  });
}

export function usePushPullGitRepository() {
  const api = useApi();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ repositoryId, branch }: { repositoryId: string; branch?: string }) => {
      const response = await api.getApiClient().pushPullGitRepository(repositoryId, branch ? { branch } : undefined);
      return response.data;
    },
    onSuccess: (_, variables) => {
      queryClient.invalidateQueries({ 
        queryKey: QUERY_KEYS.gitRepository(variables.repositoryId) 
      });
      queryClient.invalidateQueries({ 
        queryKey: QUERY_KEYS.repositoryTree(variables.repositoryId, '.', variables.branch || '') 
      });
    },
  });
}

// Helper functions

export function getRepositoryTypeColor(repoType: string): string {
  switch (repoType) {
    case 'project':
      return 'blue';
    case 'spec_task':
      return 'green';
    case 'sample':
      return 'orange';
    case 'template':
      return 'purple';
    default:
      return 'gray';
  }
}

export function getRepositoryStatusColor(status: string): string {
  switch (status) {
    case 'active':
      return 'green';
    case 'archived':
      return 'gray';
    case 'deleted':
      return 'red';
    default:
      return 'gray';
  }
}

export function getSampleTypeIcon(sampleType: string): string {
  switch (sampleType) {
    case 'empty':
      return '📄';
    case 'nodejs-todo':
      return '⚡';
    case 'python-api':
      return '🐍';
    case 'react-dashboard':
      return '⚛️';
    case 'linkedin-outreach':
      return '💼';
    case 'helix-blog-posts':
      return '📝';
    default:
      return '📦';
  }
}

export function getSampleTypeCategory(sampleType: string): 'development' | 'business' | 'content' | 'other' {
  switch (sampleType) {
    case 'nodejs-todo':
    case 'python-api':
    case 'react-dashboard':
    case 'empty':
      return 'development';
    case 'linkedin-outreach':
      return 'business';
    case 'helix-blog-posts':
      return 'content';
    default:
      return 'other';
  }
}

export function formatRepositoryName(repo: any): string {
  if (!repo) return 'Unknown Repository';
  return repo.name || repo.id || 'Unnamed Repository';
}

export function formatLastActivity(lastActivity: string | undefined): string {
  if (!lastActivity) return 'No activity';
  return new Date(lastActivity).toLocaleString();
}

export function isBusinessTask(sampleType: string): boolean {
  return ['linkedin-outreach', 'helix-blog-posts'].includes(sampleType);
}

export function getBusinessTaskDescription(sampleType: string): string {
  switch (sampleType) {
    case 'linkedin-outreach':
      return 'Multi-session campaign to reach out to 100 qualified prospects using Helix LinkedIn integration. Includes prospect research, personalized messaging, and follow-up tracking.';
    case 'helix-blog-posts':
      return 'Write 10 technical blog posts about the Helix system by analyzing the actual GitHub repository. Includes codebase analysis, content planning, and technical writing.';
    default:
      return '';
  }
}

export function getCloneInstructionsForZedAgent(repository: any, apiKey?: string): string {
  if (!repository) return '';
  
  const cloneUrl = repository.clone_url;
  const repoName = repository.name || repository.id;
  
  if (!apiKey) {
    return `# Clone Repository
    
git clone ${cloneUrl} /workspace/${repoName}
cd /workspace/${repoName}

# Note: API key authentication required
# Get your API key from Account Settings → API Keys
`;
  }

  const authenticatedUrl = cloneUrl.replace('://', `://api:${apiKey}@`);
  
  return `# Clone Repository with Authentication

git clone ${authenticatedUrl} /workspace/${repoName}
cd /workspace/${repoName}

# After cloning, you can find:
${repository.repo_type === 'spec_task' ? `
- docs/specs/requirements.md - User requirements
- docs/specs/design.md - Technical design  
- docs/specs/tasks.md - Implementation plan
- docs/specs/coordination.md - Multi-session strategy
` : `
- src/ - Source code
- README.md - Project documentation
- tests/ - Test files
`}

# Working with the repository:
git checkout -b feature/your-feature-name
# ... make changes ...
git add .
git commit -m "[SessionID] Description of changes"
git push origin feature/your-feature-name
`;
}

// Default export for the service
const gitRepositoryService = {
  // Query functions
  useGitRepositories,
  useGitRepository,
  useSampleTypes,
  useCloneCommand,
  useUserGitRepositories,
  useBrowseRepositoryTree,
  useGetRepositoryFile,

  // Mutation functions
  useCreateGitRepository,
  useCreateSampleRepository,
  useInitializeSampleRepositories,
  useCreateOrUpdateRepositoryFile,
  usePushPullGitRepository,

  // Helper functions
  getRepositoryTypeColor,
  getRepositoryStatusColor,
  getSampleTypeIcon,
  getSampleTypeCategory,
  formatRepositoryName,
  formatLastActivity,
  isBusinessTask,
  getBusinessTaskDescription,
  getCloneInstructionsForZedAgent,

  // Query keys for external use
  QUERY_KEYS,
};

export default gitRepositoryService;