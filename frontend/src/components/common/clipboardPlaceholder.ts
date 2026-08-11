// Minimal valid 1x1 transparent PNG. The gesture-anchored ClipboardItem in the
// desktop-stream copy handler (DesktopStreamViewer) must declare both
// text/plain and image/png up front — we don't know which the remote produced
// until the async clipboard fetch resolves, and Safari requires the
// navigator.clipboard.write() call itself to happen inside the user gesture.
// Chrome runs every image written to the clipboard through a decode/sanitize
// step and REJECTS the entire write() if any image/png representation fails to
// decode — so the "no image this time" fallback must be a fully decodable PNG,
// not a zero-byte Blob (which silently broke all text copy on Chrome; see
// commit 2161143e2). Generated with Pillow (RGBA 1x1, alpha 0) and verified:
// 70 bytes, valid signature, IHDR/IDAT/IEND with correct CRC-32s.
//
// The consequence is that after a *text* copy from the desktop the system
// clipboard holds the real text AND this sentinel image. Paste destinations
// that read files before text would attach a transparent pixel instead of the
// text, so the clipboard->attachment path strips it (see filesFromClipboard).
export const PLACEHOLDER_PNG_BASE64 =
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNgYGBgAAAABQABeqhXUAAAAABJRU5ErkJggg=='

// Derived from the constant above so the byte-length match below can never
// drift from the bytes we actually write.
export const PLACEHOLDER_PNG_BYTE_LENGTH = atob(PLACEHOLDER_PNG_BASE64).length

// Cheap synchronous match for the sentinel. Paste handlers must decide whether
// to preventDefault() synchronously, so an async byte-for-byte comparison
// cannot gate that decision. This is keyed to the exact byte length of the
// sentinel we produce — not a vague "small PNG" heuristic — and a PNG of that
// size can only encode a 1x1 single-colour pixel, so a false positive discards
// nothing a user could see.
export function isPlaceholderPng(file: File): boolean {
  return file.type === 'image/png' && file.size === PLACEHOLDER_PNG_BYTE_LENGTH
}
