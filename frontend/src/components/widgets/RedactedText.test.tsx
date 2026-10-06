import { fireEvent, render, screen } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import RedactedText from './RedactedText'

const EMAIL = 'karolis.rusenas@gmail.com'

describe('RedactedText', () => {
  it('never puts the real value in the DOM while hidden', () => {
    // The whole point: a CSS blur over the real text still leaks it to
    // selection, copy-paste and the accessibility tree.
    const { container } = render(<RedactedText value={EMAIL} />)
    expect(container.textContent).not.toContain(EMAIL)
    expect(container.textContent).not.toContain('karolis')
  })

  it('keeps the shape so a hidden email still reads as an email', () => {
    const { container } = render(<RedactedText value={EMAIL} />)
    const shown = container.textContent || ''
    expect(shown).toHaveLength(EMAIL.length)
    expect(shown).toContain('@')
    // Separators are preserved in place.
    expect(shown.indexOf('@')).toBe(EMAIL.indexOf('@'))
  })

  it('redacts the same value identically every time, so it does not flicker', () => {
    const first = render(<RedactedText value={EMAIL} />).container.textContent
    const second = render(<RedactedText value={EMAIL} />).container.textContent
    expect(first).toBe(second)
  })

  it('redacts different values differently', () => {
    const a = render(<RedactedText value="alice@example.com" />).container.textContent
    const b = render(<RedactedText value="bob@example.com" />).container.textContent
    expect(a).not.toBe(b)
  })

  it('reveals and hides again on click', () => {
    render(<RedactedText value={EMAIL} ariaLabel="Claude account email" />)
    const button = screen.getByRole('button', { name: 'Claude account email' })

    expect(button.textContent).not.toBe(EMAIL)
    fireEvent.click(button)
    expect(button.textContent).toBe(EMAIL)
    fireEvent.click(button)
    expect(button.textContent).not.toBe(EMAIL)
  })

  it('does not trigger the row it sits inside', () => {
    // These live inside clickable cards; revealing must not also open one.
    let rowClicked = false
    render(
      <div onClick={() => { rowClicked = true }}>
        <RedactedText value={EMAIL} ariaLabel="email" />
      </div>,
    )
    fireEvent.click(screen.getByRole('button', { name: 'email' }))
    expect(rowClicked).toBe(false)
  })

  it('renders nothing when there is no value', () => {
    expect(render(<RedactedText value={undefined} />).container).toBeEmptyDOMElement()
    expect(render(<RedactedText value="   " />).container).toBeEmptyDOMElement()
  })
})
