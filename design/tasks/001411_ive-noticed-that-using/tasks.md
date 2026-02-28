# Implementation Tasks

- [ ] Add `imageRendering: zoomLevel > 1 ? "pixelated" : "auto"` to canvas element style in `DesktopStreamViewer.tsx` (around line 4882)
- [ ] Add `imageRendering: zoomLevel > 1 ? "pixelated" : "auto"` to screenshot img element style in `DesktopStreamViewer.tsx` (around line 4921)
- [ ] Test pinch-to-zoom on iOS Safari - verify text appears sharp when zoomed
- [ ] Test pinch-to-zoom on Android Chrome - verify text appears sharp when zoomed
- [ ] Verify no visual regression at 1x zoom (normal viewing without zoom)
- [ ] Test both video mode and screenshot mode to confirm fix applies to both