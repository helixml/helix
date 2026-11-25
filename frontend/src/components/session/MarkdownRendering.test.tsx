import { render, waitFor, within } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { afterEach, describe, expect, it, vi } from 'vitest'

import { TypesSession } from '../../api/api'
import Markdown from './Markdown'

vi.unmock('react-markdown')

const mocks = vi.hoisted(() => ({ embedAccessToken: null as string | null }))

vi.mock('../../hooks/useApi', () => ({
  getEmbedAccessToken: () => mocks.embedAccessToken,
}))

const session = { id: 'session-1', config: {} } as TypesSession
const theme = createTheme({ palette: { mode: 'dark' } })
const quote = [
  '> “Webhook Relay lets you receive webhooks on your local machine.”',
  '>',
  '> — from <https://webhookrelay.com>',
].join('\n')

describe('Markdown rendering', () => {
  afterEach(() => {
    mocks.embedAccessToken = null
  })

  it('renders blockquotes in Agent Chat and markdown artifacts', async () => {
    const { getByTestId } = render(
      <ThemeProvider theme={theme}>
        <div data-testid="agent-chat">
          <Markdown text={quote} session={session} getFileURL={() => ''} />
        </div>
        <div data-testid="artifact">
          <Markdown text={quote} session={null} renderThinkingWidget={false} />
        </div>
      </ThemeProvider>,
    )

    await waitFor(() => {
      expect(within(getByTestId('agent-chat')).getByText(/Webhook Relay/).closest('blockquote')).not.toBeNull()
      expect(within(getByTestId('artifact')).getByText(/Webhook Relay/).closest('blockquote')).not.toBeNull()
    })

    for (const surface of ['agent-chat', 'artifact']) {
      const container = within(getByTestId(surface))
      const link = container.getByRole('link', { name: 'https://webhookrelay.com' })
      expect(link).toHaveAttribute('href', 'https://webhookrelay.com')
      expect(getByTestId(surface)).not.toHaveTextContent('> “Webhook Relay')
    }
  })

  it('sanitizes unsafe raw HTML after parsing markdown', async () => {
    const { container, getByText } = render(
      <ThemeProvider theme={theme}>
        <Markdown
          text={'<script>alert("xss")</script>\n\n<iframe src="https://example.com"></iframe>\n\nSafe content'}
          session={null}
        />
      </ThemeProvider>,
    )

    await waitFor(() => expect(getByText('Safe content')).toBeInTheDocument())
    expect(container.querySelector('script')).toBeNull()
    expect(container.querySelector('iframe')).toBeNull()
  })

  it('carries embed access only to same-origin artifact links', async () => {
    mocks.embedAccessToken = 'embed secret'
    const sameOriginArtifact = `${window.location.origin}/artifacts/art_absolute`
    const { getByRole } = render(
      <ThemeProvider theme={theme}>
        <Markdown
          text={[
            '[relative](/artifacts/art_relative)',
            `[absolute](${sameOriginArtifact})`,
            '[foreign](https://example.com/artifacts/art_foreign)',
            '[lookalike](/artifacts/art_relative/download)',
            '<a target="_blank" href="/artifacts/art_explicit">explicit</a>',
          ].join('\n\n')}
          session={session}
          getFileURL={() => ''}
        />
      </ThemeProvider>,
    )

    await waitFor(() => expect(getByRole('link', { name: 'relative' })).toBeInTheDocument())
    expect(getByRole('link', { name: 'relative' })).toHaveAttribute(
      'href',
      '/artifacts/art_relative?access_token=embed+secret',
    )
    expect(getByRole('link', { name: 'absolute' })).toHaveAttribute(
      'href',
      `${sameOriginArtifact}?access_token=embed+secret`,
    )
    expect(getByRole('link', { name: 'foreign' })).toHaveAttribute(
      'href',
      'https://example.com/artifacts/art_foreign',
    )
    expect(getByRole('link', { name: 'lookalike' })).toHaveAttribute(
      'href',
      '/artifacts/art_relative/download',
    )
    expect(getByRole('link', { name: 'explicit' })).toHaveAttribute(
      'href',
      '/artifacts/art_explicit?access_token=embed+secret',
    )
    expect(getByRole('link', { name: 'explicit' })).toHaveAttribute('rel', 'noopener noreferrer')
  })

  it('leaves artifact links unchanged outside an embed', async () => {
    const { getByRole } = render(
      <ThemeProvider theme={theme}>
        <Markdown text="[report](/artifacts/art_report)" session={session} getFileURL={() => ''} />
      </ThemeProvider>,
    )

    await waitFor(() => expect(getByRole('link', { name: 'report' })).toBeInTheDocument())
    expect(getByRole('link', { name: 'report' })).toHaveAttribute('href', '/artifacts/art_report')
  })
})
