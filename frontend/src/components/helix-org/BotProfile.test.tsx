import { render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import BotProfile from './BotProfile'

const runningBot = {
  id: 'chief-of-staff',
  name: 'Chief of Staff',
  status: 'running',
  sandbox_status: 'running',
  portrait_index: 5,
} as any

describe('BotProfile', () => {
  it('renders the expanded Chief profile', () => {
    render(<BotProfile bot={runningBot} expanded />)

    expect(screen.getByTestId('bot-profile-expanded')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Chief of Staff' })).toBeInTheDocument()
    expect(screen.getByText('I help you set priorities and coordinate your team.')).toBeInTheDocument()
    expect(screen.getByText('Ready to chat')).toBeInTheDocument()
  })

  it('keeps long names in the compact profile', () => {
    render(<BotProfile bot={{ ...runningBot, id: 'bot_01', name: 'A very long operations coordination bot name' }} />)

    expect(screen.getByTestId('bot-profile-compact')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'A very long operations coordination bot name' })).toBeInTheDocument()
  })
})
