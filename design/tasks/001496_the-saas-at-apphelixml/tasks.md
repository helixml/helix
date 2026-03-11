# Implementation Tasks

- [ ] Set `HELIX_EDITION=cloud` in the app.helix.ml SaaS deployment environment config
- [ ] In `frontend/src/pages/Login.tsx`: add `const isCloud = config?.edition === 'cloud'` alongside the existing `isRegular` check (~line 82)
- [ ] In `frontend/src/pages/Login.tsx`: change the OIDC description text (~line 426) to show `"Sign in to your Helix account."` when `isCloud`, keeping the existing `"Use your organization's single sign-on to access Helix."` otherwise
- [ ] In `frontend/src/pages/Login.tsx`: change the OIDC button label (~line 444) to show `"Sign in"` when `isCloud`, keeping `"Sign in with SSO"` otherwise
- [ ] In `frontend/src/pages/Session.tsx`: import `useGetConfig` and add `const isCloud = config?.edition === 'cloud'`
- [ ] In `frontend/src/pages/Session.tsx`: change the login prompt text (~line 1628) to show `"Sign in to your Helix account to continue."` when `isCloud`, keeping the existing text otherwise
- [ ] Verify `yarn build` passes with no errors
- [ ] Test on a local instance with `HELIX_EDITION=cloud` set — confirm SaaS copy appears
- [ ] Test on a local instance without `HELIX_EDITION` set — confirm enterprise copy is unchanged