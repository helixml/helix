export function isMobileOrTablet(): boolean {
  if (typeof navigator === 'undefined') return false
  // Standard mobile UA detection
  if (/iPhone|iPad|iPod|Android/i.test(navigator.userAgent)) return true
  // Modern iPads report as "Macintosh" — detect via touch + no fine pointer
  if (navigator.maxTouchPoints > 1 && !window.matchMedia('(pointer: fine)').matches) return true
  return false
}

/**
 * True on Safari and every iOS browser (all are WebKit with an Apple vendor).
 * These do NOT GPU-accelerate 2D-canvas drawImage(VideoFrame) — at 4K it's a
 * per-frame CPU copy that caps presentation at ~4fps — so the video stream uses a
 * WebGL2 texture path there. Chrome/Firefox keep the faster zero-copy 2D path
 * (drawImage on a desynchronized canvas is a video-overlay scanout there).
 * navigator.vendor is 'Apple Computer, Inc.' on WebKit, 'Google Inc.' on Chrome,
 * '' on Firefox — a reliable engine signal for this purpose.
 */
export function isAppleWebKit(): boolean {
  if (typeof navigator === 'undefined') return false
  return (navigator.vendor || '').includes('Apple')
}
