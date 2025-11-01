import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import useApi from '../hooks/useApi';
import { TypesQuestionSet } from '../api/api';

export const questionSetQueryKey = (id: string) => [
  "question-set",
  id
];

export const questionSetsListQueryKey = (orgId?: string) => [
  "question-sets",
  "list",
  ...(orgId ? [orgId] : [])
];

export function useListQuestionSets(orgId?: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: questionSetsListQueryKey(orgId),
    queryFn: async () => {
      const result = await apiClient.v1QuestionSetsList(orgId ? { org_id: orgId } : undefined)
      return result.data
    },
    enabled: options?.enabled ?? true,
  });
}

export function useQuestionSet(id: string, options?: { enabled?: boolean }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: questionSetQueryKey(id),
    queryFn: async () => {
      const result = await apiClient.v1QuestionSetsDetail(id)
      return result.data
    },
    enabled: (options?.enabled ?? true) && !!id,
  });
}

export function useCreateQuestionSet() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: { questionSet: TypesQuestionSet; orgId?: string }) => {
      const { questionSet, orgId } = data;
      const result = await apiClient.v1QuestionSetsCreate(
        questionSet,
        orgId ? { params: { org_id: orgId } } as any : undefined
      );
      return result.data
    },
    onSuccess: (data, variables) => {
      queryClient.invalidateQueries({ queryKey: questionSetsListQueryKey(variables.orgId) })
      queryClient.invalidateQueries({ queryKey: questionSetsListQueryKey() })
      if (data.id) {
        queryClient.invalidateQueries({ queryKey: questionSetQueryKey(data.id) })
      }
    }
  });
}

export function useUpdateQuestionSet() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (data: { id: string; questionSet: TypesQuestionSet }) => {
      const { id, questionSet } = data;
      const result = await apiClient.v1QuestionSetsUpdate(id, questionSet)
      return result.data
    },
    onSuccess: (data) => {
      if (data.id) {
        queryClient.invalidateQueries({ queryKey: questionSetQueryKey(data.id) })
      }
      queryClient.invalidateQueries({ queryKey: questionSetsListQueryKey() })
      if (data.organization_id) {
        queryClient.invalidateQueries({ queryKey: questionSetsListQueryKey(data.organization_id) })
      }
    }
  });
}

export function useDeleteQuestionSet() {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (id: string) => {
      const result = await apiClient.v1QuestionSetsDelete(id)
      return result.data
    },
    onSuccess: (_, id) => {
      queryClient.removeQueries({ queryKey: questionSetQueryKey(id) })
      queryClient.invalidateQueries({ queryKey: questionSetsListQueryKey() })
    }
  });
}

