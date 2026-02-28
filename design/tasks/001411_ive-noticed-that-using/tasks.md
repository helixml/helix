# Implementation Tasks

- [ ] Modify canvas element style in `DesktopStreamViewer.tsx` to use CSS width/height scaling instead of transform scale
  - Change `width: canvasDisplaySize.width * zoomLevel` instead of `transform: scale(zoomLevel)`
  - Change `height: canvasDisplaySize.height * zoomLevel` similarly
  - Update transform to only handle centering and pan offset (no scale)
- [ ] Ensure container has `overflow: hidden` to clip the enlarged canvas when zoomed
- [ ] Update pan offset logic to work with the new CSS-size-based zoom approach
- [ ] Apply same changes to screenshot img element for consistency in screenshot mode
- [ ] Test on iOS Safari - verify zoomed text is sharper than before
- [ ] Test on Android Chrome - verify zoomed text is sharper than before
- [ ] Verify pan gestures work correctly with new implementation
- [ ] Check GPU/performance impact at high zoom levels on mobile devices