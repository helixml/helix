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
import { PLACEHOLDER_PNG_BYTE_LENGTH } from '../../utils/clipboardPlaceholder'

// The 1x1 transparent PNG the desktop copy handler writes alongside copied text.
const placeholderPng = () =>
  new File([new Uint8Array(PLACEHOLDER_PNG_BYTE_LENGTH)], 'image.png', { type: 'image/png' })

const clipboard = (text: string, files: File[]): DataTransfer => ({
  files,
  items: files.map((file) => ({ kind: 'file', type: file.type, getAsFile: () => file })),
  getData: (type: string) => (type === 'text/plain' ? text : ''),
} as unknown as DataTransfer)

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

  it('drops the desktop placeholder PNG when the clipboard also carries text', () => {
    const data = clipboard('hello from the desktop', [placeholderPng()])
    expect(filesFromClipboard(data)).toEqual([])
  })

  it('keeps the placeholder-sized image when the clipboard has no text (a real image copy)', () => {
    const image = placeholderPng()
    expect(filesFromClipboard(clipboard('', [image]))).toEqual([image])
  })

  it('keeps real images and files pasted alongside text', () => {
    const screenshot = new File([new Uint8Array(4096)], 'screenshot.png', { type: 'image/png' })
    const pdf = new File([new Uint8Array(32)], 'spec.pdf', { type: 'application/pdf' })
    expect(filesFromClipboard(clipboard('see attached', [screenshot, pdf]))).toEqual([screenshot, pdf])
  })

  it('keeps real images pasted alongside the desktop placeholder', () => {
    const screenshot = new File([new Uint8Array(4096)], 'screenshot.png', { type: 'image/png' })
    expect(filesFromClipboard(clipboard('caption', [placeholderPng(), screenshot]))).toEqual([screenshot])
  })

  it('builds an encoded same-origin workspace URL', () => {
    expect(workspaceAttachmentURL('ses_1', '/home/retro/work/incoming/my image.png')).toBe(
      '/api/v1/external-agents/ses_1/file?name=my+image.png',
    )
  })
})
