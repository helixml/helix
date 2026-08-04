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
