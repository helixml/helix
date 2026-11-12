import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import lavender from '../../assets/bots/lavender.webp'
import pearl from '../../assets/bots/pearl.webp'
import BotPortrait from './BotPortrait'

describe('BotPortrait', () => {
  it('uses persisted assignments to resolve the Chief and Joke Writer collision', () => {
    const view = render(<BotPortrait portraitIndex={5} />)
    expect(view.container.querySelector('img')).toHaveAttribute('src', pearl)

    view.rerender(<BotPortrait portraitIndex={0} />)
    expect(view.container.querySelector('img')).toHaveAttribute('src', lavender)
  })

  it('renders as decorative identity artwork', () => {
    const { container } = render(<BotPortrait portraitIndex={0} size={32} />)
    const portrait = container.querySelector('img')
    expect(portrait).toHaveAttribute('alt', '')
    expect(portrait).toHaveAttribute('aria-hidden', 'true')
  })

  it('does not guess a portrait while the assignment is unavailable', () => {
    const { container } = render(<BotPortrait />)
    expect(container.querySelector('img')).not.toBeInTheDocument()
  })
})
