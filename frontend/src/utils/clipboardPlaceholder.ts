// Minimal valid 1x1 transparent PNG. On WebKit (Safari, all iOS browsers) the
// gesture-anchored ClipboardItem in the desktop copy handler (DesktopStreamViewer)
// must declare both text/plain and image/png up front (we don't know which the
// remote produced until the async fetch resolves). Chromium and Firefox write
// after the fetch and never produce this placeholder. Chrome runs every image written to the clipboard through a
// decode/sanitize step and REJECTS the entire navigator.clipboard.write() if any
// image/png representation fails to decode — so the "no image this time"
// fallback must be a fully decodable PNG, not a zero-byte Blob (which silently
// broke all text copy on Chrome). Generated with Pillow (RGBA 1x1, alpha 0) and
// verified: 70 bytes, valid signature, IHDR/IDAT/IEND with correct CRC-32s.
export const PLACEHOLDER_PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNgYGBgAAAABQABeqhXUAAAAABJRU5ErkJggg=="

// Decoded byte length of PLACEHOLDER_PNG_BASE64. Paste handlers must decide
// whether to preventDefault() synchronously, and File.arrayBuffer() is async, so
// MIME type + exact byte length is the only fingerprint available at that point.
export const PLACEHOLDER_PNG_BYTE_LENGTH = 70

// True when a pasted clipboard file looks like the placeholder written alongside
// a text copy. Callers must additionally require a non-empty text/plain before
// discarding the file: a genuine image copy out of the desktop carries an EMPTY
// text/plain, so that guard is what keeps real images (including real 1x1 PNGs)
// attaching normally.
export function isPlaceholderClipboardImage(file: { type: string; size: number }): boolean {
  return file.type === 'image/png' && file.size === PLACEHOLDER_PNG_BYTE_LENGTH
}
