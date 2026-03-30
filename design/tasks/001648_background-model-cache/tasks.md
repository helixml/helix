# Implementation Tasks

- [ ] In `refreshAllProviderModels()` (`api/pkg/server/provider_handlers.go`), change the `ListProviderEndpoints` call to use `All: true` instead of `Owner: "system", WithGlobal: true`
- [ ] Verify with `go build ./pkg/server/` that no compilation errors are introduced
- [ ] Manually confirm (or write a test) that user-created provider endpoints are now included in the refresh by checking that `getProviderModels` is called for a user-owned endpoint
