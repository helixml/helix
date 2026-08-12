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
      '<div class="chat-markdown-table-container" data-expanded="false">',
      '<table><thead><tr><th>was</th><th>now</th></tr></thead>',
      '<tbody><tr><td>nginx</td><td>nginx-unprivileged</td></tr></tbody></table>',
      '<div class="chat-markdown-table-footer"></div></div>',
      '</div>',
    ].join('')

    const inlineCode = container.querySelector<HTMLElement>('code')
    const tableContainer = container.querySelector<HTMLElement>('.chat-markdown-table-container')
    const tableFooter = container.querySelector<HTMLElement>('.chat-markdown-table-footer')
    const table = container.querySelector<HTMLElement>('table')
    const header = container.querySelector<HTMLElement>('th')
    const cell = container.querySelector<HTMLElement>('td')

    expect(inlineCode).toHaveStyle({
      backgroundColor: 'rgba(255, 255, 255, 0.04)',
      color: '#f5f5f5',
      border: '1px solid rgba(255, 255, 255, 0.06)',
      padding: '0.1rem 0.35rem',
      borderRadius: '0.375rem',
      fontSize: '0.75rem',
    })
    expect(table).toHaveStyle({
      borderCollapse: 'collapse',
      minWidth: 'max-content',
      fontSize: '0.75rem',
    })
    expect(tableContainer).toHaveStyle({ margin: '1rem 0' })
    expect(tableFooter).toHaveStyle({
      display: 'flex',
      justifyContent: 'space-between',
      marginTop: '0.5rem',
    })
    expect(header).toHaveStyle({
      borderBottom: '1px solid rgba(255, 255, 255, 0.07)',
      padding: '0.55rem 0.75rem 0.55rem 0.75rem',
      fontWeight: '600',
    })
    expect(cell).toHaveStyle({
      borderBottom: '1px solid rgba(255, 255, 255, 0.07)',
      padding: '0.45rem 0.75rem',
      maxWidth: '24rem',
      textOverflow: 'ellipsis',
      whiteSpace: 'nowrap',
    })
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
