// Since Chromium 4f9d868279 (Sep 2026, "Ensure default paste behavior doesn't
// leak changed clipboard content"), Chrome records the clipboard's sequence
// number when a paste starts and silently skips the default insertion if the
// clipboard has changed by the time the `paste` event has been handled: no
// beforeinput, no input, nothing inserted. Apps that paste by writing the
// clipboard, sending Cmd+V and then restoring the previous contents (Wispr
// Flow and other dictation tools, some clipboard managers) regularly land
// their restore inside that window, so pastes vanish intermittently in every
// text field. The `paste` event's own data is the clipboard as it was when the
// paste started, so insert that and cancel the default insertion.
//
// If the clipboard changes before the event's data is read at all, Chrome
// hands the page empty data and there is nothing left to insert.

const PLAIN_TEXT_INPUT_TYPES = new Set(['', 'text', 'search', 'url', 'tel', 'email', 'password'])

function plainTextPasteTarget(element: Element | null): HTMLElement | null {
  if (element instanceof HTMLTextAreaElement) {
    return element.readOnly || element.disabled ? null : element
  }
  if (element instanceof HTMLInputElement) {
    return PLAIN_TEXT_INPUT_TYPES.has(element.type) && !element.readOnly && !element.disabled ? element : null
  }
  // Rich contenteditable editors expect the HTML flavour, so only plain-text
  // editing hosts are handled here.
  if (element instanceof HTMLElement && element.isContentEditable) {
    return element.closest('[contenteditable="plaintext-only"]') ? element : null
  }
  return null
}

// Text read at the very start of each paste, before any other handler runs.
// Once the clipboard has changed, clipboardData.getData returns '', so the
// read has to happen as early as possible.
const textAtPasteStart = new WeakMap<Event, string>()

function captureText(event: ClipboardEvent): void {
  const text = event.clipboardData?.getData('text/plain')
  if (text) textAtPasteStart.set(event, text)
}

function insertPastedText(event: ClipboardEvent): void {
  // Components that handle their own pastes (the chat composer, Monaco, the
  // terminal, the desktop stream viewer) cancel the event before it bubbles
  // here.
  if (event.defaultPrevented) return
  const target = plainTextPasteTarget(document.activeElement)
  if (!target) return
  const text = textAtPasteStart.get(event)
  if (!text) return
  event.preventDefault()
  if (document.execCommand('insertText', false, text)) return
  if (target instanceof HTMLTextAreaElement || target instanceof HTMLInputElement) {
    // The browser refused the editing command: insert at the caret directly and
    // notify listeners (React-controlled inputs update from the input event).
    target.setRangeText(text, target.selectionStart ?? target.value.length, target.selectionEnd ?? target.value.length, 'end')
    target.dispatchEvent(new InputEvent('input', { bubbles: true, inputType: 'insertFromPaste', data: text }))
  }
}

// The text is captured on window in the capture phase, the first listener to
// run for any paste. The insertion runs on window in the bubble phase, after
// every element and React handler has had the chance to handle the paste.
export function installPlainTextPasteFix(): () => void {
  window.addEventListener('paste', captureText, { capture: true })
  window.addEventListener('paste', insertPastedText)
  return () => {
    window.removeEventListener('paste', captureText, { capture: true })
    window.removeEventListener('paste', insertPastedText)
  }
}
