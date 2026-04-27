# Implementation Tasks

- [ ] Create `frontend/src/components/widgets/MaskedSecret.tsx` per the props/behaviour in design.md (fixed-length bullets, eye toggle, copy button, 30 s auto-hide, hide on `visibilitychange`).
- [ ] Replace the `Chip` + `CopyKeyButton` group in `frontend/src/components/app/APIKeysSection.tsx` with `<MaskedSecret value={apiKey.key} />`; delete the now-unused `maskKey()` and `CopyKeyButton`.
- [ ] In `frontend/src/components/app/CodeExamples.tsx`, add a `showKey` toggle (default `false`); render snippets with a masked placeholder (`hl-••••••••`) when hidden, but always copy with the real key. Auto-hide after 30 s and on `visibilitychange`.
- [ ] In `frontend/src/components/settings/OAuthSettings.tsx`, switch the OAuth 1.0a Private Key field (line 328-338) to the masked-when-not-focused pattern using `<MaskedSecret>` for display and the existing multiline TextField for editing.
- [ ] Manual test in the inner Helix at `http://localhost:8080`: register `test@helix.ml`, create an org, create an agent, add an API key, confirm both the Keys table and the Code Examples panel hide the key by default and that copy still works in both places.
- [ ] Run `cd frontend && yarn build` and fix any type errors before committing.
- [ ] Take before/after screenshots of the agent Keys tab and attach them to the PR description.
- [ ] Open PR against `helixml/helix`, link this design doc, and check Drone CI green.
