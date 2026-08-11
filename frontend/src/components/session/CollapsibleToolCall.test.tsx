import { fireEvent, render, screen } from '@testing-library/react'
import { ThemeProvider, createTheme } from '@mui/material/styles'
import { describe, expect, it } from 'vitest'

import {
  CollapsibleToolCall,
  getToolCallExpandedBody,
  getToolCallPresentation,
} from './CollapsibleToolCall'

describe('CollapsibleToolCall', () => {
  it('presents shell calls as a compact command row', () => {
    const presentation = getToolCallPresentation(
      'Bash',
      '| | |\n|---|---|\n| Command | `git status --short` |\n| Exit | 0 |',
    )

    expect(presentation).toEqual({
      kind: 'command',
      label: 'Ran command',
      preview: 'git status --short',
    })
  })

  it('removes transport metadata from expanded command output', () => {
    const body = [
      '**Tool Call: git status --short**',
      'Status: Completed',
      '',
      'Terminal:',
      '```',
      ' M file.ts',
      '```',
    ].join('\n')

    expect(getToolCallExpandedBody('git status --short', body)).toBe(
      'git status --short\n\nM file.ts',
    )
  })

  it('recognizes raw ACP shell titles from terminal output', () => {
    const presentation = getToolCallPresentation(
      "git status --short",
      "**Tool Call: git status --short**\nStatus: Completed\nTerminal:\n```\n M file.ts\n```",
    )

    expect(presentation).toEqual({
      kind: 'command',
      label: 'Ran command',
      preview: 'git status --short',
    })
  })

  it('presents MCP calls as provider and tool', () => {
    expect(getToolCallPresentation('mcp.codex_apps.github.fetch_pr', '')).toEqual({
      kind: 'tool',
      label: 'GitHub · fetch_pr',
      preview: '',
    })
    expect(getToolCallPresentation('mcp__t3_code__preview_snapshot', '')).toEqual({
      kind: 'tool',
      label: 'T3-code · preview_snapshot',
      preview: '',
    })
  })

  it('marks disclosure growth so the chat keeps the header in place', () => {
    const { container } = render(
      <div data-session-scroll-container>
        <ThemeProvider theme={createTheme()}>
          <CollapsibleToolCall toolName="inspect_topic" status="Completed" body="topic details" />
        </ThemeProvider>
      </div>,
    )

    fireEvent.click(screen.getByText('inspect_topic'))

    expect(screen.getByText('topic details')).toBeInTheDocument()
    expect(container.firstElementChild).toHaveAttribute('data-preserve-disclosure-expansion', 'true')
  })
})
