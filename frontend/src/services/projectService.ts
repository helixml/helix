import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesProject, TypesProjectCreateRequest, TypesProjectUpdateRequest, TypesSampleProject, TypesSampleProjectInstantiateRequest, StoreDBGitRepository } from '../api/api';

// Query keys
export const projectsListQueryKey = () => ['projects'];
export const projectQueryKey = (id: string) => ['project', id];
export const projectRepositoriesQueryKey = (projectId: string) => ['project-repositories', projectId];
export const sampleProjectsListQueryKey = () => ['sample-projects'];
export const sampleProjectQueryKey = (id: string) => ['sample-project', id];

/**
 * Hook to list all projects for the current user
 */
export const useListProjects = () => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesProject[]>({
    queryKey: projectsListQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsList();
      return response.data || [];
    },
  });
};

/**
 * Hook to get a specific project by ID
 */
export const useGetProject = (projectId: string, enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesProject>({
    queryKey: projectQueryKey(projectId),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsDetail(projectId);
      return response.data;
    },
    enabled: enabled && !!projectId,
  });
};

/**
 * Hook to create a new project
 */
export const useCreateProject = () => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (request: TypesProjectCreateRequest) => {
      const response = await apiClient.v1ProjectsCreate(request);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectsListQueryKey() });
    },
  });
};

/**
 * Hook to update a project
 */
export const useUpdateProject = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (request: TypesProjectUpdateRequest) => {
      const response = await apiClient.v1ProjectsUpdate(projectId, request);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectQueryKey(projectId) });
      queryClient.invalidateQueries({ queryKey: projectsListQueryKey() });
    },
  });
};

/**
 * Hook to delete a project
 */
export const useDeleteProject = () => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (projectId: string) => {
      const response = await apiClient.v1ProjectsDelete(projectId);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectsListQueryKey() });
    },
  });
};

/**
 * Hook to get repositories for a project
 */
export const useGetProjectRepositories = (projectId: string, enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<StoreDBGitRepository[]>({
    queryKey: projectRepositoriesQueryKey(projectId),
    queryFn: async () => {
      const response = await apiClient.v1ProjectsRepositoriesList(projectId);
      return (response.data as StoreDBGitRepository[]) || [];
    },
    enabled: enabled && !!projectId,
  });
};

/**
 * Hook to set a repository as primary for a project
 */
export const useSetProjectPrimaryRepository = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (repoId: string) => {
      const response = await apiClient.v1ProjectsRepositoriesPrimaryUpdate(projectId, repoId);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectQueryKey(projectId) });
      queryClient.invalidateQueries({ queryKey: projectRepositoriesQueryKey(projectId) });
    },
  });
};

/**
 * Hook to attach a repository to a project
 */
export const useAttachRepositoryToProject = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (repoId: string) => {
      const response = await apiClient.v1ProjectsRepositoriesAttachUpdate(projectId, repoId);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectQueryKey(projectId) });
      queryClient.invalidateQueries({ queryKey: projectRepositoriesQueryKey(projectId) });
    },
  });
};

/**
 * Hook to detach a repository from a project
 */
export const useDetachRepositoryFromProject = (projectId: string) => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (repoId: string) => {
      const response = await apiClient.v1ProjectsRepositoriesDetachUpdate(projectId, repoId);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectQueryKey(projectId) });
      queryClient.invalidateQueries({ queryKey: projectRepositoriesQueryKey(projectId) });
    },
  });
};

/**
 * Hook to list all sample projects
 */
export const useListSampleProjects = () => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesSampleProject[]>({
    queryKey: sampleProjectsListQueryKey(),
    queryFn: async () => {
      const response = await apiClient.v1SampleProjectsV2List();
      return response.data || [];
    },
  });
};

/**
 * Hook to get a specific sample project
 */
export const useGetSampleProject = (sampleId: string, enabled = true) => {
  const api = useApi();
  const apiClient = api.getApiClient();

  return useQuery<TypesSampleProject>({
    queryKey: sampleProjectQueryKey(sampleId),
    queryFn: async () => {
      const response = await apiClient.v1SampleProjectsV2Detail(sampleId);
      return response.data;
    },
    enabled: enabled && !!sampleId,
  });
};

/**
 * Hook to instantiate a sample project
 */
export const useInstantiateSampleProject = () => {
  const api = useApi();
  const apiClient = api.getApiClient();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ sampleId, request }: { sampleId: string; request: TypesSampleProjectInstantiateRequest }) => {
      const response = await apiClient.v1SampleProjectsV2InstantiateCreate(sampleId, request);
      return response.data;
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: projectsListQueryKey() });
    },
  });
};
