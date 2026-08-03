export const CHAT_ATTACHMENT_MAX_COUNT = 10
export const CHAT_ATTACHMENT_MAX_BYTES = 500 * 1024 * 1024

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

const TEXT_FILE_EXTENSION = /\.(txt|md|json|xml|csv|log|js|jsx|ts|tsx|py|java|c|cpp|h|hpp|css|html|yaml|yml|toml|ini|sql|sh)$/i

export function classifyChatAttachment(file: File): ChatAttachmentType {
  if (file.type.startsWith('image/')) return 'image'
  if (file.type.startsWith('text/') || TEXT_FILE_EXTENSION.test(file.name)) return 'text'
  return 'file'
}

export function createPendingChatAttachment(file: File): PendingChatAttachment {
  const type = classifyChatAttachment(file)
  return {
    id: crypto.randomUUID(),
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
): { accepted: File[]; rejected: RejectedChatAttachment[] } {
  const accepted: File[] = []
  const rejected: RejectedChatAttachment[] = []
  let availableSlots = Math.max(0, CHAT_ATTACHMENT_MAX_COUNT - existingCount)

  for (const file of files) {
    if (file.size > CHAT_ATTACHMENT_MAX_BYTES) {
      rejected.push({ name: file.name, reason: 'exceeds the 500 MB upload limit' })
      continue
    }
    if (availableSlots === 0) {
      rejected.push({ name: file.name, reason: `only ${CHAT_ATTACHMENT_MAX_COUNT} files can be attached` })
      continue
    }
    accepted.push(file)
    availableSlots -= 1
  }

  return { accepted, rejected }
}

export function filesFromClipboard(data: DataTransfer): File[] {
  const directFiles = Array.from(data.files)
  if (directFiles.length > 0) return directFiles

  return Array.from(data.items)
    .filter((item) => item.kind === 'file')
    .flatMap((item) => {
      const file = item.getAsFile()
      return file ? [file] : []
    })
}

function escapeInlineCode(value: string): string {
  return value.replace(/`/g, '\\`')
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
    'Attachments available in the agent workspace:',
    ...uploaded.map((attachment) =>
      `- ${attachment.type === 'image' ? 'Image' : 'File'}: \`${escapeInlineCode(attachment.path)}\``,
    ),
  ].join('\n')

  return content ? `${content}\n\n${attachmentBlock}` : attachmentBlock
}
