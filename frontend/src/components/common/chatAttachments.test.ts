import { describe, expect, it } from 'vitest'

import {
  buildMessageWithAttachments,
  createPendingChatAttachment,
  filesFromClipboard,
  parseMessageWithAttachments,
  PendingChatAttachment,
  validateChatAttachmentFiles,
  workspaceAttachmentURL,
} from './chatAttachments'
import { PLACEHOLDER_PNG_BASE64 } from './clipboardPlaceholder'

const uploaded = (overrides: Partial<PendingChatAttachment>): PendingChatAttachment => ({
  id: 'attachment-1',
  name: 'file.pdf',
  path: '/home/retro/work/incoming/file.pdf',
  type: 'file',
  mimeType: 'application/pdf',
  sizeBytes: 42,
  uploadStatus: 'uploaded',
  ...overrides,
})

describe('chat attachments', () => {
  it('creates attachment IDs when randomUUID is unavailable on an HTTP origin', () => {
    const originalRandomUUID = globalThis.crypto.randomUUID
    Object.defineProperty(globalThis.crypto, 'randomUUID', {
      configurable: true,
      value: undefined,
    })
    try {
      const attachment = createPendingChatAttachment(
        new File(['notes'], 'notes.txt', { type: 'text/plain' }),
      )
      expect(attachment.id).not.toBe('')
    } finally {
      Object.defineProperty(globalThis.crypto, 'randomUUID', {
        configurable: true,
        value: originalRandomUUID,
      })
    }
  })

  it('keeps the user prompt first and adds explicit workspace paths', () => {
    expect(buildMessageWithAttachments('Review these', [
      uploaded({ type: 'image', path: '/home/retro/work/incoming/screenshot.png' }),
      uploaded({ id: 'attachment-2', path: '/home/retro/work/incoming/spec.pdf' }),
    ])).toBe([
      'Review these',
      '',
      'Attachments available in the agent workspace:',
      '- Image: "/home/retro/work/incoming/screenshot.png"',
      '- File: "/home/retro/work/incoming/spec.pdf"',
    ].join('\n'))
  })

  it('rejects files beyond the count and size limits without dropping accepted files', () => {
    const tooLarge = new File(['x'], 'huge.bin')
    Object.defineProperty(tooLarge, 'size', { value: 500 * 1024 * 1024 + 1 })
    const result = validateChatAttachmentFiles(
      [new File(['ok'], 'ok.pdf'), tooLarge, new File(['extra'], 'extra.txt')],
      9,
    )

    expect(result.accepted.map((file) => file.name)).toEqual(['ok.pdf'])
    expect(result.rejected.map(({ name }) => name)).toEqual(['huge.bin', 'extra.txt'])
  })

  it('separates a valid workspace manifest from user-visible prose', () => {
    const parsed = parseMessageWithAttachments([
      'What is in this screenshot?',
      '',
      'Attachments available in the agent workspace:',
      '- Image: "/home/retro/work/incoming/image.png"',
      '- File: "/home/retro/work/incoming/requirements.pdf"',
    ].join('\n'))

    expect(parsed.message).toBe('What is in this screenshot?')
    expect(parsed.attachments).toEqual([
      { type: 'image', path: '/home/retro/work/incoming/image.png', name: 'image.png' },
      { type: 'file', path: '/home/retro/work/incoming/requirements.pdf', name: 'requirements.pdf' },
    ])
  })

  it('does not hide malformed or out-of-workspace manifests', () => {
    const content = [
      'Keep this visible',
      '',
      'Attachments available in the agent workspace:',
      '- Image: "/etc/passwd"',
    ].join('\n')
    expect(parseMessageWithAttachments(content)).toEqual({ message: content, attachments: [] })

    const nested = content.replace('/etc/passwd', '/home/retro/work/incoming/nested/image.png')
    expect(parseMessageWithAttachments(nested)).toEqual({ message: nested, attachments: [] })
  })

  it('builds an encoded same-origin workspace URL', () => {
    expect(workspaceAttachmentURL('ses_1', '/home/retro/work/incoming/my image.png')).toBe(
      '/api/v1/external-agents/ses_1/file?name=my+image.png',
    )
  })
})

const clipboard = (files: File[], items: File[] = []): DataTransfer => ({
  files,
  items: items.map((file) => ({ kind: 'file', type: file.type, getAsFile: () => file })),
} as unknown as DataTransfer)

const placeholderFile = (name = 'image.png'): File => {
  const binary = atob(PLACEHOLDER_PNG_BASE64)
  const bytes = new Uint8Array(binary.length)
  for (let i = 0; i < binary.length; i++) bytes[i] = binary.charCodeAt(i)
  return new File([bytes], name, { type: 'image/png' })
}

describe('filesFromClipboard', () => {
  const realImage = new File([new Uint8Array(2048)], 'screenshot.png', { type: 'image/png' })
  const pdf = new File(['pdf'], 'spec.pdf', { type: 'application/pdf' })

  it('drops the desktop-stream sentinel PNG from the file list', () => {
    expect(filesFromClipboard(clipboard([placeholderFile()]))).toEqual([])
  })

  it('drops the sentinel when it arrives via clipboard items', () => {
    expect(filesFromClipboard(clipboard([], [placeholderFile()]))).toEqual([])
  })

  it('keeps real images and non-image files', () => {
    expect(filesFromClipboard(clipboard([realImage, pdf]))).toEqual([realImage, pdf])
    expect(filesFromClipboard(clipboard([], [realImage]))).toEqual([realImage])
  })

  it('keeps a real image pasted alongside the sentinel', () => {
    expect(filesFromClipboard(clipboard([placeholderFile(), realImage]))).toEqual([realImage])
  })
})
