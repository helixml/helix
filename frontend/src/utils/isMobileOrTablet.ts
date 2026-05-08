export function isMobileOrTablet(): boolean {
  if (typeof navigator === 'undefined') return false
  // Standard mobile UA detection
  if (/iPhone|iPad|iPod|Android/i.test(navigator.userAgent)) return true
  // Modern iPads report as "Macintosh" — detect via touch + no fine pointer
  if (navigator.maxTouchPoints > 1 && !window.matchMedia('(pointer: fine)').matches) return true
  return false
}
