import { describe, expect, it } from 'vitest'

import {
  buildMessageWithAttachments,
  PendingChatAttachment,
  validateChatAttachmentFiles,
} from './chatAttachments'

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
  it('keeps the user prompt first and adds explicit workspace paths', () => {
    expect(buildMessageWithAttachments('Review these', [
      uploaded({ type: 'image', path: '/home/retro/work/incoming/screenshot.png' }),
      uploaded({ id: 'attachment-2', path: '/home/retro/work/incoming/spec.pdf' }),
    ])).toBe([
      'Review these',
      '',
      'Attachments available in the agent workspace:',
      '- Image: `/home/retro/work/incoming/screenshot.png`',
      '- File: `/home/retro/work/incoming/spec.pdf`',
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
})
