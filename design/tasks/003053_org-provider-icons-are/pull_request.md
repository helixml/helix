# Fix provider icons in light mode

## Summary

- Give shared provider brand marks an explicit theme-aware foreground color so monochrome SVG icons remain visible in light mode.
- Apply the same foreground color to the generic provider fallback icon.
- Fix icon rendering in both Organization → Providers and Admin Panel → Provider Endpoints.
- Add regression coverage for recognized and unknown providers in the light theme.

## Screenshots

### Organization providers

![Organization providers in light mode](https://raw.githubusercontent.com/helixml/helix/feature/003053-org-provider-icons-are/design/screenshots/003053-org-providers-light-mode.png)

### Admin provider endpoints

![Admin provider endpoints in light mode](https://raw.githubusercontent.com/helixml/helix/feature/003053-org-provider-icons-are/design/screenshots/003053-admin-providers-light-mode.png)

## Testing

- `yarn test ProviderEndpointIcon` — 5 tests passed.
- `yarn tsc` — passed.
- `yarn build` — production frontend build passed.
