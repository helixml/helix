import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { installPlainTextPasteFix } from './plainTextPaste'

const paste = (target: HTMLElement, text: string): ClipboardEvent => {
  const event = new Event('paste', { bubbles: true, cancelable: true }) as ClipboardEvent
  Object.defineProperty(event, 'clipboardData', {
    value: { getData: (type: string) => (type === 'text/plain' ? text : '') },
  })
  target.dispatchEvent(event)
  return event
}

describe('installPlainTextPasteFix', () => {
  let uninstall: () => void
  let insertText: ReturnType<typeof vi.fn>

  beforeEach(() => {
    insertText = vi.fn(() => true)
    Object.defineProperty(document, 'execCommand', { configurable: true, value: insertText })
    uninstall = installPlainTextPasteFix()
  })

  afterEach(() => {
    uninstall()
    document.body.replaceChildren()
  })

  const mount = <T extends HTMLElement>(element: T): T => {
    document.body.append(element)
    element.focus()
    return element
  }

  it('inserts the paste event text into a focused textarea', () => {
    const textarea = mount(document.createElement('textarea'))

    const event = paste(textarea, 'hello from wispr')

    expect(event.defaultPrevented).toBe(true)
    expect(insertText).toHaveBeenCalledWith('insertText', false, 'hello from wispr')
  })

  it('handles text inputs and plain-text contenteditable', () => {
    mount(document.createElement('input'))
    paste(document.activeElement as HTMLElement, 'a')

    const editor = document.createElement('div')
    editor.setAttribute('contenteditable', 'plaintext-only')
    Object.defineProperty(editor, 'isContentEditable', { value: true })
    editor.tabIndex = 0
    mount(editor)
    paste(editor, 'b')

    expect(insertText.mock.calls.map((call) => call[2])).toEqual(['a', 'b'])
  })

  it('uses the text from the start of the paste even if the clipboard changes during it', () => {
    const textarea = mount(document.createElement('textarea'))
    let clipboard = 'hello from wispr'
    const event = new Event('paste', { bubbles: true, cancelable: true }) as ClipboardEvent
    Object.defineProperty(event, 'clipboardData', {
      value: { getData: (type: string) => (type === 'text/plain' ? clipboard : '') },
    })
    // Chrome returns '' from getData once the clipboard has changed.
    textarea.addEventListener('paste', () => { clipboard = '' })

    textarea.dispatchEvent(event)

    expect(event.defaultPrevented).toBe(true)
    expect(insertText).toHaveBeenCalledWith('insertText', false, 'hello from wispr')
  })

  it('leaves pastes another handler already took alone', () => {
    const textarea = mount(document.createElement('textarea'))
    textarea.addEventListener('paste', (event) => event.preventDefault())

    paste(textarea, 'owned elsewhere')

    expect(insertText).not.toHaveBeenCalled()
  })

  it('leaves rich editors, non-text inputs and read-only fields to the browser', () => {
    const rich = document.createElement('div')
    rich.setAttribute('contenteditable', 'true')
    Object.defineProperty(rich, 'isContentEditable', { value: true })
    rich.tabIndex = 0
    mount(rich)
    expect(paste(rich, 'x').defaultPrevented).toBe(false)

    const number = document.createElement('input')
    number.type = 'number'
    mount(number)
    expect(paste(number, '1').defaultPrevented).toBe(false)

    const readOnly = document.createElement('textarea')
    readOnly.readOnly = true
    mount(readOnly)
    expect(paste(readOnly, 'x').defaultPrevented).toBe(false)

    expect(insertText).not.toHaveBeenCalled()
  })

  it('inserts at the caret directly when the browser refuses insertText', () => {
    insertText.mockReturnValue(false)
    const textarea = mount(document.createElement('textarea'))
    textarea.value = 'ab'
    textarea.setSelectionRange(1, 1)
    const onInput = vi.fn()
    textarea.addEventListener('input', onInput)

    paste(textarea, 'X')

    expect(textarea.value).toBe('aXb')
    expect(onInput).toHaveBeenCalledTimes(1)
  })
})
