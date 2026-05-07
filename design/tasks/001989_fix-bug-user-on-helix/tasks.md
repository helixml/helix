# Implementation Tasks

- [x] In `frontend/src/contexts/account.tsx`, extend the `orgNavigate` fallback chain to also try `organizationToolsRef.current.organizations?.[0]?.name`, and add a final guard that redirects to `/orgs` (with a `console.warn`) instead of calling `router.navigate` with `org_id: undefined`.
- [~] In `frontend/src/components/system/TokenUsageDisplay.tsx`, hide the "Add my own API Keys" button when there is no org context at all (`!organizationTools.organization && !organizationTools.organizations?.[0]`).
- [ ] Manually verify in the inner Helix: from `/files` (non-org URL), the providers-button code path no longer throws the router5 error overlay; it either navigates to `/orgs/<slug>/providers` or to `/orgs` if there is no org.
- [ ] Regression check: clicking "Providers" in the org sidebar from `/orgs/<slug>/projects` still works.
- [ ] `cd frontend && yarn build` passes.
- [ ] Open PR against `helixml/helix` `main`, link this design doc, get CI green, merge.
