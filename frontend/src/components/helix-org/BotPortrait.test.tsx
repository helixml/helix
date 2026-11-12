import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import lavender from '../../assets/bots/lavender.webp'
import BotPortrait, { botPortraitForId } from './BotPortrait'

describe('BotPortrait', () => {
  it('keeps a known bot id on its assigned portrait', () => {
    expect(botPortraitForId('bot_01')).toBe(lavender)
  })

  it('distributes opaque bot ids across the portrait set', () => {
    const portraits = new Set(
      ['bot_01', 'bot_02', 'bot_03', 'bot_04', 'bot_05', 'bot_06'].map(botPortraitForId),
    )
    expect(portraits.size).toBe(6)
  })

  it('renders as decorative identity artwork', () => {
    const { container } = render(<BotPortrait botId="bot_01" size={32} />)
    const portrait = container.querySelector('img')
    expect(portrait).toHaveAttribute('alt', '')
    expect(portrait).toHaveAttribute('aria-hidden', 'true')
  })
})
