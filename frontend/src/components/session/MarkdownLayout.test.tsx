import { render } from '@testing-library/react'
import { createTheme, ThemeProvider } from '@mui/material/styles'
import { describe, expect, it } from 'vitest'

import { TypesSession } from '../../api/api'
import Markdown from './Markdown'

const session = { id: 'session-1', config: {} } as TypesSession
const theme = createTheme({ palette: { mode: 'dark' } })

describe('Markdown chat spacing', () => {
  it('keeps block spacing stable across adjacent streamed response entries', () => {
    const { container } = render(
      <ThemeProvider theme={theme}>
        <Markdown text={'First entry\n\nFirst continuation'} session={session} getFileURL={() => ''} isStreaming />
        <Markdown text={'Second entry\n\nSecond continuation'} session={session} getFileURL={() => ''} isStreaming />
      </ThemeProvider>,
    )

    const roots = container.querySelectorAll<HTMLElement>('[data-chat-markdown]')
    roots.forEach((root) => {
      root.setAttribute('data-chat-markdown-visible', 'true')
    })
    roots.forEach((root) => {
      root.innerHTML = '<div class="interactionMessage"><p>First paragraph</p><p>Last paragraph</p></div>'
    })
    const messages = container.querySelectorAll<HTMLElement>('.interactionMessage')

    expect(messages[0].firstElementChild).toHaveStyle({ marginTop: '0px' })
    expect(messages[0].lastElementChild).toHaveStyle({ marginBottom: '0px' })
    expect(messages[0].lastElementChild).toHaveStyle({ marginTop: '14px' })
    expect(messages[1].firstElementChild).toHaveStyle({ marginTop: '0px' })
    expect(messages[1].lastElementChild).toHaveStyle({ marginBottom: '0px' })
    expect(roots[1]).toHaveStyle({ marginTop: '14px' })
  })

  it('uses restrained T3-style inline code and table surfaces', () => {
    const { container } = render(
      <ThemeProvider theme={theme}>
        <Markdown
          text={'Use `npm ci`\n\n| was | now |\n| --- | --- |\n| nginx | nginx-unprivileged |'}
          session={session}
          getFileURL={() => ''}
          isStreaming={false}
        />
      </ThemeProvider>,
    )

    const root = container.querySelector<HTMLElement>('[data-chat-markdown]')
    if (!root) throw new Error('markdown root not found')
    root.innerHTML = [
      '<div class="interactionMessage">',
      '<p>Use <code>npm ci</code></p>',
      '<table><thead><tr><th>was</th><th>now</th></tr></thead>',
      '<tbody><tr><td>nginx</td><td>nginx-unprivileged</td></tr></tbody></table>',
      '</div>',
    ].join('')

    const inlineCode = container.querySelector<HTMLElement>('code')
    const table = container.querySelector<HTMLElement>('table')
    const header = container.querySelector<HTMLElement>('th')
    const cell = container.querySelector<HTMLElement>('td')

    expect(inlineCode).toHaveStyle({
      backgroundColor: '#303033',
      color: 'rgba(245, 245, 245, 0.78)',
      padding: '0.08em 0.28em',
      borderRadius: '4px',
    })
    expect(table).toHaveStyle({
      border: '1px solid rgba(255, 255, 255, 0.18)',
      borderRadius: '8px',
    })
    expect(header).toHaveStyle({
      backgroundColor: '#25272e',
      color: 'rgba(245, 245, 245, 0.8)',
    })
    expect(cell).toHaveStyle({ backgroundColor: '#0c0c0d' })
  })

  it('does not add spacing to empty response entries', () => {
    const { container } = render(
      <ThemeProvider theme={theme}>
        <Markdown text={'Visible entry'} session={session} getFileURL={() => ''} isStreaming />
        <Markdown
          text={'<think>Hidden entry</think>'}
          session={session}
          getFileURL={() => ''}
          isStreaming={false}
          renderThinkingWidget={false}
        />
      </ThemeProvider>,
    )

    const roots = container.querySelectorAll<HTMLElement>('[data-chat-markdown]')
    expect(roots[0]).toHaveAttribute('data-chat-markdown-visible', 'true')
    expect(roots[1]).not.toHaveAttribute('data-chat-markdown-visible')
    expect(roots[1]).not.toHaveStyle({ marginTop: '14px' })
  })
})
