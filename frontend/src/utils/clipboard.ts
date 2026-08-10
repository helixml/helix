export async function copyTextToClipboard(text: string): Promise<void> {
  // Safari can expose `navigator.clipboard` inconsistently on plain HTTP
  // origins. Do not touch the property unless the browser says this is a
  // secure context, and retain the object so it cannot disappear between the
  // capability check and the write.
  if (window.isSecureContext) {
    try {
      const clipboard = navigator.clipboard
      if (clipboard && typeof clipboard.writeText === 'function') {
        await clipboard.writeText(text)
        return
      }
    } catch {
      // Permission and WebKit failures use the DOM fallback below.
    }
  }

  const textArea = document.createElement('textarea')
  textArea.value = text
  textArea.setAttribute('readonly', '')
  textArea.style.position = 'fixed'
  textArea.style.left = '-9999px'
  document.body.appendChild(textArea)
  textArea.select()

  try {
    if (!document.execCommand('copy')) {
      throw new Error('Clipboard write failed')
    }
  } finally {
    textArea.remove()
  }
}
