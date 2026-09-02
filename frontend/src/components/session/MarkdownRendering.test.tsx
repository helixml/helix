import { render, waitFor, within } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { describe, expect, it, vi } from 'vitest'

import { TypesSession } from '../../api/api'
import Markdown from './Markdown'

vi.unmock('react-markdown')

const session = { id: 'session-1', config: {} } as TypesSession
const theme = createTheme({ palette: { mode: 'dark' } })
const quote = [
  '> “Webhook Relay lets you receive webhooks on your local machine.”',
  '>',
  '> — from <https://webhookrelay.com>',
].join('\n')

describe('Markdown rendering', () => {
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
})
