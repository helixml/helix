import { createRandomId } from '../../utils/randomId'
import { isPlaceholderPng } from './clipboardPlaceholder'

export const CHAT_ATTACHMENT_MAX_COUNT = 10
export const CHAT_ATTACHMENT_MAX_BYTES = 500 * 1024 * 1024
export const CHAT_ATTACHMENT_MANIFEST_HEADER = 'Attachments available in the agent workspace:'

export type ChatAttachmentType = 'image' | 'text' | 'file'
export type ChatAttachmentUploadStatus = 'pending' | 'uploading' | 'uploaded' | 'failed'

export interface PendingChatAttachment {
  id: string
  name: string
  path?: string
  file?: File
  type: ChatAttachmentType
  mimeType: string
  sizeBytes: number
  previewUrl?: string
  uploadStatus: ChatAttachmentUploadStatus
  error?: string
}

export interface ChatWorkspaceAttachment {
  type: 'image' | 'file'
  path: string
  name: string
}

const TEXT_FILE_EXTENSION = /\.(txt|md|json|xml|csv|log|js|jsx|ts|tsx|py|java|c|cpp|h|hpp|css|html|yaml|yml|toml|ini|sql|sh)$/i

export function classifyChatAttachment(file: File): ChatAttachmentType {
  if (file.type.startsWith('image/')) return 'image'
  if (file.type.startsWith('text/') || TEXT_FILE_EXTENSION.test(file.name)) return 'text'
  return 'file'
}

export function createPendingChatAttachment(file: File): PendingChatAttachment {
  const type = classifyChatAttachment(file)
  return {
    id: createRandomId(),
    name: file.name || 'attachment',
    file,
    type,
    mimeType: file.type || 'application/octet-stream',
    sizeBytes: file.size,
    previewUrl: type === 'image' ? URL.createObjectURL(file) : undefined,
    uploadStatus: 'pending',
  }
}

export interface RejectedChatAttachment {
  name: string
  reason: string
}

export function validateChatAttachmentFiles(
  files: File[],
  existingCount: number,
  limits: { maxCount?: number; maxBytes?: number } = {},
): { accepted: File[]; rejected: RejectedChatAttachment[] } {
  const maxCount = limits.maxCount ?? CHAT_ATTACHMENT_MAX_COUNT
  const maxBytes = limits.maxBytes ?? CHAT_ATTACHMENT_MAX_BYTES
  const accepted: File[] = []
  const rejected: RejectedChatAttachment[] = []
  let availableSlots = Math.max(0, maxCount - existingCount)

  for (const file of files) {
    if (file.size > maxBytes) {
      const maxMegabytes = Math.round(maxBytes / 1024 / 1024)
      rejected.push({ name: file.name, reason: `exceeds the ${maxMegabytes} MB upload limit` })
      continue
    }
    if (availableSlots === 0) {
      rejected.push({ name: file.name, reason: `only ${maxCount} files can be attached` })
      continue
    }
    accepted.push(file)
    availableSlots -= 1
  }

  return { accepted, rejected }
}

// Every copy from the streamed desktop leaves a sentinel 1x1 transparent PNG on
// the clipboard alongside the text (see clipboardPlaceholder.ts for why it has
// to be there). Strip it here so it can never reach an attachment tray — doing
// it in this helper rather than at each call site means every consumer of the
// clipboard is protected by construction.
export function filesFromClipboard(data: DataTransfer): File[] {
  const directFiles = Array.from(data.files)
  const files = directFiles.length > 0
    ? directFiles
    : Array.from(data.items)
        .filter((item) => item.kind === 'file')
        .flatMap((item) => {
          const file = item.getAsFile()
          return file ? [file] : []
        })

  return files.filter((file) => !isPlaceholderPng(file))
}

export function buildMessageWithAttachments(
  content: string,
  attachments: PendingChatAttachment[],
): string {
  const uploaded = attachments.filter(
    (attachment): attachment is PendingChatAttachment & { path: string } =>
      attachment.uploadStatus === 'uploaded' && !!attachment.path,
  )
  if (uploaded.length === 0) return content

  const attachmentBlock = [
    CHAT_ATTACHMENT_MANIFEST_HEADER,
    ...uploaded.map((attachment) =>
      `- ${attachment.type === 'image' ? 'Image' : 'File'}: ${JSON.stringify(attachment.path)}`,
    ),
  ].join('\n')

  return content ? `${content}\n\n${attachmentBlock}` : attachmentBlock
}

export function parseMessageWithAttachments(content: string): {
  message: string
  attachments: ChatWorkspaceAttachment[]
} {
  const markerIndex = content.lastIndexOf(CHAT_ATTACHMENT_MANIFEST_HEADER)
  if (markerIndex < 0) return { message: content, attachments: [] }

  const prefix = content.slice(0, markerIndex)
  if (prefix && !prefix.endsWith('\n\n')) return { message: content, attachments: [] }

  const lines = content.slice(markerIndex + CHAT_ATTACHMENT_MANIFEST_HEADER.length).trim().split('\n')
  if (lines.length === 0 || lines.some((line) => !line.trim())) {
    return { message: content, attachments: [] }
  }

  const attachments: ChatWorkspaceAttachment[] = []
  for (const line of lines) {
    const match = line.match(/^- (Image|File): (.+)$/)
    if (!match) return { message: content, attachments: [] }

    let path: unknown
    try {
      path = JSON.parse(match[2])
    } catch {
      return { message: content, attachments: [] }
    }
    if (typeof path !== 'string' || !path.startsWith('/home/retro/work/incoming/')) {
      return { message: content, attachments: [] }
    }

    const name = path.slice('/home/retro/work/incoming/'.length)
    if (!name || name.includes('/') || name.includes('\\')) {
      return { message: content, attachments: [] }
    }

    attachments.push({
      type: match[1] === 'Image' ? 'image' : 'file',
      path,
      name,
    })
  }

  return { message: prefix.trimEnd(), attachments }
}

export function workspaceAttachmentURL(sessionId: string, path: string): string {
  const name = path.split('/').pop() || ''
  const query = new URLSearchParams({ name })
  return `/api/v1/external-agents/${encodeURIComponent(sessionId)}/file?${query.toString()}`
}
