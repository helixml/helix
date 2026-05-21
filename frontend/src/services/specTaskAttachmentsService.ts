import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import axios from 'axios'
import {
  TypesSpecTaskAttachment,
} from '../api/api'
import useApi from '../hooks/useApi'

const SPEC_TASK_ATTACHMENTS_KEY = 'spec-task-attachments'

export const SPEC_TASK_ATTACHMENT_MAX_BYTES = 10 * 1024 * 1024 // 10 MB
export const SPEC_TASK_ATTACHMENT_MAX_PER_TASK = 10

export const SPEC_TASK_ATTACHMENT_ACCEPTED_MIME: Record<string, string[]> = {
  'image/png': ['.png'],
  'image/jpeg': ['.jpg', '.jpeg'],
  'image/gif': ['.gif'],
  'image/webp': ['.webp'],
  'image/svg+xml': ['.svg'],
  'application/pdf': ['.pdf'],
  'text/plain': ['.txt'],
  'text/markdown': ['.md', '.markdown'],
  'text/csv': ['.csv'],
}

// useSpecTaskAttachments fetches the attachment list for a task.
export function useSpecTaskAttachments(taskId: string | undefined, enabled = true) {
  const api = useApi()
  return useQuery({
    queryKey: [SPEC_TASK_ATTACHMENTS_KEY, taskId],
    enabled: !!taskId && enabled,
    queryFn: async (): Promise<TypesSpecTaskAttachment[]> => {
      if (!taskId) return []
      const res = await api.getApiClient().v1SpecTasksAttachmentsDetail(taskId)
      return res.data || []
    },
  })
}

// useUploadSpecTaskAttachments uploads one or more files to a task. The backend
// accepts an array under the field name 'files' in a single multipart request,
// so we bundle them all up here to minimise HTTP overhead.
export function useUploadSpecTaskAttachments() {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (args: { taskId: string; files: File[]; caption?: string }) => {
      const fd = new FormData()
      for (const f of args.files) {
        fd.append('files', f)
      }
      if (args.caption) fd.append('caption', args.caption)
      const res = await axios.post<TypesSpecTaskAttachment[]>(
        `/api/v1/spec-tasks/${args.taskId}/attachments`,
        fd,
        { headers: { 'Content-Type': 'multipart/form-data' } },
      )
      return res.data
    },
    onSuccess: (_data, args) => {
      queryClient.invalidateQueries({ queryKey: [SPEC_TASK_ATTACHMENTS_KEY, args.taskId] })
    },
  })
}

// useDeleteSpecTaskAttachment deletes one attachment by id.
export function useDeleteSpecTaskAttachment() {
  const api = useApi()
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: async (args: { taskId: string; attachmentId: string }) => {
      await api.getApiClient().v1SpecTasksAttachmentsDelete(args.taskId, args.attachmentId)
    },
    onSuccess: (_data, args) => {
      queryClient.invalidateQueries({ queryKey: [SPEC_TASK_ATTACHMENTS_KEY, args.taskId] })
    },
  })
}

// attachmentContentURL returns the public URL for streaming an attachment's bytes.
// Used as the src of <img> tags / for opening PDFs and text in a new tab.
export function attachmentContentURL(taskId: string, attachmentId: string): string {
  return `/api/v1/spec-tasks/${taskId}/attachments/${attachmentId}/content`
}
