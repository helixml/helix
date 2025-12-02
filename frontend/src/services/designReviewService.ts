import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useApi } from '../hooks/useApi'

// Query keys
export const designReviewKeys = {
  all: ['design-reviews'] as const,
  lists: () => [...designReviewKeys.all, 'list'] as const,
  list: (specTaskId: string) => [...designReviewKeys.lists(), specTaskId] as const,
  details: () => [...designReviewKeys.all, 'detail'] as const,
  detail: (specTaskId: string, reviewId: string) => [...designReviewKeys.details(), specTaskId, reviewId] as const,
  comments: (specTaskId: string, reviewId: string) => [...designReviewKeys.detail(specTaskId, reviewId), 'comments'] as const,
}

// Types
export interface DesignReview {
  id: string
  spec_task_id: string
  reviewer_id?: string
  status: 'pending' | 'in_review' | 'changes_requested' | 'approved' | 'superseded'
  git_commit_hash: string
  git_branch: string
  git_pushed_at: string
  requirements_spec: string
  technical_design: string
  implementation_plan: string
  overall_comment?: string
  approved_at?: string
  rejected_at?: string
  created_at: string
  updated_at: string
}

export interface DesignReviewComment {
  id: string
  review_id: string
  commented_by: string
  document_type: 'requirements' | 'technical_design' | 'implementation_plan'
  section_path?: string
  line_number?: number
  quoted_text?: string
  start_offset?: number
  end_offset?: number
  comment_text: string
  comment_type?: 'general' | 'question' | 'suggestion' | 'critical' | 'praise' // Made optional
  // Agent integration fields
  agent_response?: string
  agent_response_at?: string
  interaction_id?: string
  // Resolution fields
  resolved: boolean
  resolved_by?: string
  resolved_at?: string
  resolution_reason?: 'manual' | 'auto_text_removed' | 'agent_updated'
  created_at: string
  updated_at: string
  replies?: DesignReviewCommentReply[]
}

export interface DesignReviewCommentReply {
  id: string
  comment_id: string
  replied_by: string
  reply_text: string
  is_agent: boolean
  created_at: string
}

export interface DesignReviewDetailResponse {
  review: DesignReview
  comments: DesignReviewComment[]
  spec_task: any
}

// Query hooks

export function useDesignReviews(specTaskId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: designReviewKeys.list(specTaskId),
    queryFn: async () => {
      const response = await apiClient.get(`/api/v1/spec-tasks/${specTaskId}/design-reviews`)
      return response.data
    },
    enabled: !!specTaskId,
  })
}

export function useDesignReview(specTaskId: string, reviewId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: designReviewKeys.detail(specTaskId, reviewId),
    queryFn: async () => {
      const response = await apiClient.v1SpecTasksDesignReviewsDetail2(specTaskId, reviewId)
      return response.data
    },
    enabled: !!specTaskId && !!reviewId,
  })
}

export function useDesignReviewComments(specTaskId: string, reviewId: string, options?: { refetchInterval?: number }) {
  const api = useApi()
  const apiClient = api.getApiClient()

  return useQuery({
    queryKey: designReviewKeys.comments(specTaskId, reviewId),
    queryFn: async () => {
      const response = await apiClient.v1SpecTasksDesignReviewsCommentsDetail(specTaskId, reviewId)
      return response.data
    },
    enabled: !!specTaskId && !!reviewId,
    refetchInterval: options?.refetchInterval,
  })
}

// Mutation hooks

export function useSubmitReview(specTaskId: string, reviewId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (data: { decision: 'approve' | 'request_changes'; overall_comment?: string }) => {
      const response = await apiClient.v1SpecTasksDesignReviewsSubmitCreate(specTaskId, reviewId, data)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: designReviewKeys.detail(specTaskId, reviewId) })
      queryClient.invalidateQueries({ queryKey: designReviewKeys.list(specTaskId) })
      // Also invalidate spec task to update status
      queryClient.invalidateQueries({ queryKey: ['spec-tasks', specTaskId] })
    },
  })
}

export function useCreateComment(specTaskId: string, reviewId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (data: {
      document_type: 'requirements' | 'technical_design' | 'implementation_plan'
      section_path?: string
      line_number?: number
      quoted_text?: string
      start_offset?: number
      end_offset?: number
      comment_text: string
      comment_type?: 'general' | 'question' | 'suggestion' | 'critical' | 'praise'
    }) => {
      const response = await apiClient.v1SpecTasksDesignReviewsCommentsCreate(specTaskId, reviewId, data)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: designReviewKeys.comments(specTaskId, reviewId) })
      queryClient.invalidateQueries({ queryKey: designReviewKeys.detail(specTaskId, reviewId) })
    },
  })
}

export function useResolveComment(specTaskId: string, reviewId: string) {
  const api = useApi()
  const apiClient = api.getApiClient()
  const queryClient = useQueryClient()

  return useMutation({
    mutationFn: async (commentId: string) => {
      const response = await apiClient.v1SpecTasksDesignReviewsCommentsResolveCreate(specTaskId, reviewId, commentId)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: designReviewKeys.comments(specTaskId, reviewId) })
      queryClient.invalidateQueries({ queryKey: designReviewKeys.detail(specTaskId, reviewId) })
    },
  })
}

// Helper functions

export function getCommentTypeColor(type: DesignReviewComment['comment_type']): string {
  switch (type) {
    case 'critical':
      return '#f44336'
    case 'question':
      return '#ff9800'
    case 'suggestion':
      return '#9c27b0'
    case 'praise':
      return '#4caf50'
    case 'general':
    default:
      return '#2196f3'
  }
}

export function getCommentTypeIcon(type: DesignReviewComment['comment_type']): string {
  switch (type) {
    case 'critical':
      return '⚠️'
    case 'question':
      return '❓'
    case 'suggestion':
      return '💡'
    case 'praise':
      return '✅'
    case 'general':
    default:
      return '💬'
  }
}

export function getUnresolvedCount(comments: DesignReviewComment[]): number {
  return comments.filter(c => !c.resolved).length
}
