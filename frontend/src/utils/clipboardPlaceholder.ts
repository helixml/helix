// Minimal valid 1x1 transparent PNG. On WebKit (Safari, all iOS browsers) the
// gesture-anchored ClipboardItem in the desktop copy handler (DesktopStreamViewer)
// must declare both text/plain and image/png up front (we don't know which the
// remote produced until the async fetch resolves). Chromium and Firefox write
// after the fetch and never produce this placeholder. Browsers run every image
// written to the clipboard through a decode/sanitize step and REJECT the entire
// navigator.clipboard.write() if any image/png representation fails to decode —
// so the "no image this time" fallback must be a fully decodable PNG, not a
// zero-byte Blob (which silently broke all text copy on Chrome). Generated with
// Pillow (RGBA 1x1, alpha 0): 70 bytes, valid signature, IHDR/IDAT/IEND with
// correct CRC-32s.
export const PLACEHOLDER_PNG_BASE64 =
  "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNgYGBgAAAABQABeqhXUAAAAABJRU5ErkJggg=="

const PNG_SIGNATURE = [0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a]

// True when a pasted clipboard file is a 1x1 PNG, i.e. the placeholder written
// alongside a text copy. Matched on dimensions rather than bytes because the
// browser re-encodes clipboard images on the way through the OS (Chrome on
// Linux turns the 70-byte placeholder into 88 bytes), while the IHDR chunk that
// carries width/height is always the first chunk, at a fixed offset.
//
// Callers must additionally require a non-empty text/plain before discarding
// the file: a genuine image copy out of the desktop carries an EMPTY
// text/plain, so that guard is what keeps real images (including real 1x1
// PNGs) attaching normally.
export async function isPlaceholderClipboardImage(file: Blob): Promise<boolean> {
  if (file.type !== 'image/png') return false
  const header = new Uint8Array(await file.slice(0, 24).arrayBuffer())
  if (header.length < 24) return false
  if (PNG_SIGNATURE.some((byte, i) => header[i] !== byte)) return false
  const view = new DataView(header.buffer)
  return view.getUint32(16) === 1 && view.getUint32(20) === 1
}
