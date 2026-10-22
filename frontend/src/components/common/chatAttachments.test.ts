import { describe, expect, it } from 'vitest'

import {
  buildMessageWithAttachments,
  createPendingChatAttachment,
  parseMessageWithAttachments,
  PendingChatAttachment,
  validateChatAttachmentFiles,
  withoutPlaceholderImages,
  workspaceAttachmentURL,
} from './chatAttachments'
import { PLACEHOLDER_PNG_BASE64 } from '../../utils/clipboardPlaceholder'

// The exact 1x1 transparent PNG the desktop copy handler writes on Safari.
const placeholderPng = () =>
  new File([Uint8Array.from(atob(PLACEHOLDER_PNG_BASE64), (c) => c.charCodeAt(0))], 'image.png', { type: 'image/png' })

// A PNG with the given IHDR dimensions, padded to `size` bytes — e.g. the
// placeholder after the browser re-encodes it on the way through the OS.
const pngFile = (width: number, height: number, size: number, name = 'image.png') => {
  const bytes = new Uint8Array(size)
  bytes.set([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, 0, 0, 0, 13, 0x49, 0x48, 0x44, 0x52])
  const view = new DataView(bytes.buffer)
  view.setUint32(16, width)
  view.setUint32(20, height)
  return new File([bytes], name, { type: 'image/png' })
}

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

  it('drops the desktop placeholder PNG', async () => {
    expect(await withoutPlaceholderImages([placeholderPng()])).toEqual([])
  })

  it('drops the placeholder after the browser re-encodes it to a different size', async () => {
    // Chrome on Linux hands the 70-byte placeholder back as 88 bytes.
    expect(await withoutPlaceholderImages([pngFile(1, 1, 88)])).toEqual([])
  })

  it('keeps real images and non-PNG files', async () => {
    const screenshot = pngFile(64, 64, 4096, 'screenshot.png')
    const tall = pngFile(1, 40, 120, 'tall.png')
    const pdf = new File([new Uint8Array(32)], 'spec.pdf', { type: 'application/pdf' })
    expect(await withoutPlaceholderImages([screenshot, tall, pdf])).toEqual([screenshot, tall, pdf])
  })

  it('keeps real images pasted alongside the desktop placeholder', async () => {
    const screenshot = pngFile(64, 64, 4096, 'screenshot.png')
    expect(await withoutPlaceholderImages([placeholderPng(), screenshot])).toEqual([screenshot])
  })

  it('keeps truncated or mislabelled PNG data', async () => {
    const garbage = new File([new Uint8Array(70)], 'image.png', { type: 'image/png' })
    expect(await withoutPlaceholderImages([garbage])).toEqual([garbage])
  })

  it('builds an encoded same-origin workspace URL', () => {
    expect(workspaceAttachmentURL('ses_1', '/home/retro/work/incoming/my image.png')).toBe(
      '/api/v1/external-agents/ses_1/file?name=my+image.png',
    )
  })
})
