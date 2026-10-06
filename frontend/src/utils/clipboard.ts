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
  textArea.style.top = '0'
  textArea.style.opacity = '0'
  document.body.appendChild(textArea)
  textArea.focus({ preventScroll: true })
  textArea.select()
  textArea.setSelectionRange(0, text.length)

  try {
    if (!document.execCommand('copy')) {
      throw new Error('Clipboard write failed')
    }
  } finally {
    textArea.remove()
  }
}

function hasFullClipboardApi(): boolean {
  if (!window.isSecureContext) return false

  try {
    const clipboard = navigator.clipboard
    return Boolean(
      clipboard &&
      typeof clipboard.write === 'function' &&
      typeof clipboard.writeText === 'function'
    )
  } catch {
    return false
  }
}

function createMonacoClipboardService() {
  const typedText = new Map<string, string>()
  let findText = ''

  return {
    triggerPaste: () => undefined,
    writeText: async (text: string, type?: string) => {
      if (type) {
        typedText.set(type, text)
        return
      }
      await copyTextToClipboard(text)
    },
    readText: async (type?: string) => {
      if (type) return typedText.get(type) ?? ''
      if (!window.isSecureContext) return ''

      try {
        const clipboard = navigator.clipboard
        if (clipboard && typeof clipboard.readText === 'function') {
          return await clipboard.readText()
        }
      } catch {
        // Browser paste handling remains available when programmatic reads fail.
      }

      return ''
    },
    readFindText: async () => findText,
    writeFindText: async (text: string) => {
      findText = text
    },
    readResources: async () => [],
    clearInternalState: () => {
      typedText.clear()
    },
  }
}

export function getMonacoClipboardOverrideServices(): Record<string, unknown> {
  if (hasFullClipboardApi()) return {}
  return { clipboardService: createMonacoClipboardService() }
}
