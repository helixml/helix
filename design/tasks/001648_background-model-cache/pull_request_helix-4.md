# Warm model cache after user creates a provider endpoint

## Summary

User-created Ollama endpoints were never pre-cached because the background refresh is intentionally scoped to system/global providers (polling all user providers every minute would be excessive with hundreds of users). The existing on-demand caching via `getProviderModels` works correctly when `?with_models=true` is requested, but the first call after creating a provider is always a cold fetch.

This change adds a single async cache warm immediately after `CreateProviderEndpoint` succeeds, so the first `?with_models=true` request returns from cache instead of hitting the provider live.

## Changes

- `api/pkg/server/provider_handlers.go`: after `Store.CreateProviderEndpoint` returns, spin up a goroutine that calls `getProviderModels` with a 10-second detached context. Errors are logged at debug level (provider may not be reachable at the moment of creation).
