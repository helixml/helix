# Implementation Tasks

- [ ] Add `sessionStopped?: boolean` prop to `DesktopStreamViewer.tsx` interface
- [ ] Update screenshot polling effect in `DesktopStreamViewer.tsx` to return early if `sessionStopped` is true
- [ ] Add `sessionStopped` to the useEffect dependency array
- [ ] Add `sessionStopped?: boolean` prop to `ScreenshotViewer.tsx` interface
- [ ] Update auto-refresh effect in `ScreenshotViewer.tsx` to skip polling when `sessionStopped` is true
- [ ] Pass `sessionStopped={isPaused}` from `ExternalAgentDesktopViewer.tsx` to `DesktopStreamViewer` in stream mode
- [ ] Pass `sessionStopped={isPaused}` from `ExternalAgentDesktopViewer.tsx` to `ScreenshotViewer` in screenshot mode
- [ ] Test: Stop a session and verify no screenshot warnings in console
- [ ] Test: Resume a session and verify screenshot polling works again