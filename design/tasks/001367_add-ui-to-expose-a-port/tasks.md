# Implementation Tasks

## Setup
- [ ] Create new file `frontend/src/components/external-agent/PortExposureDialog.tsx`

## PortExposureDialog Component
- [ ] Create React Query hook `useExposedPorts(sessionId)` to fetch exposed ports via `api.v1SessionsExposeDetail`
- [ ] Create mutation hook `useExposePort(sessionId)` to call `api.v1SessionsExposeCreate`
- [ ] Create mutation hook `useUnexposePort(sessionId)` to call `api.v1SessionsExposeDelete`
- [ ] Build dialog UI with MUI `Dialog`, `DialogTitle`, `DialogContent`, `DialogActions`
- [ ] Add form with port number input (required) and name input (optional)
- [ ] Add "Expose" button that calls mutation and invalidates query
- [ ] Display list of exposed ports with URL, name, status
- [ ] Add copy-to-clipboard button for each URL (use `navigator.clipboard.writeText`)
- [ ] Add delete button for each exposed port with confirmation or snackbar undo
- [ ] Handle loading and error states

## Integration with DesktopStreamViewer
- [ ] Import `PortExposureDialog` in `DesktopStreamViewer.tsx`
- [ ] Add state: `const [portDialogOpen, setPortDialogOpen] = useState(false)`
- [ ] Add toolbar button (after fullscreen button) with Wifi or Share icon
- [ ] Render `<PortExposureDialog open={portDialogOpen} onClose={() => setPortDialogOpen(false)} sessionId={sessionId} />`

## Testing
- [ ] Manual test: expose port 3000, verify URL works
- [ ] Manual test: copy URL to clipboard
- [ ] Manual test: unexpose port, verify URL stops working
- [ ] Manual test: expose multiple ports, verify list shows all