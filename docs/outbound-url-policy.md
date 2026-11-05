# Git provider URL policy

Some endpoints make server-side requests to git host URLs supplied by users.
One example is the self-hosted GitHub Enterprise / GitLab `base_url` and the
Azure DevOps `organization_url` on `POST /api/v1/git/browse-remote`. These
requests go through `api/pkg/netguard`, the same validator and guarded transport
used for provider endpoints (TST-01-270). It:

- allows only `http`/`https` URLs with a host
- blocks link-local addresses (including cloud metadata `169.254.169.254`),
  multicast, reserved and documentation ranges. Loopback, RFC1918/ULA and CGNAT
  are also blocked unless private networks are allowed. The check runs on the
  *resolved* IP when the connection is made, and the request goes only to the
  addresses that were checked, so DNS rebinding and numeric IP encodings don't
  get around it.
- does not follow redirects
- keeps working with operator `HTTP(S)_PROXY` settings, but checks the
  destination before handing the request to the proxy

Requests to the public SaaS hosts (no `base_url`) use the library default client.

A rejected URL returns a generic 400. The specific reason is logged on the server.

## Allowing an internal Git server

If your GitLab / GitHub Enterprise / Azure DevOps Server runs on a private
network, set:

```
GIT_PROVIDERS_ALLOW_PRIVATE_NETWORKS=true
```

Link-local and metadata addresses stay blocked even then. This setting works the
same way as `PROVIDER_ENDPOINTS_ALLOW_PRIVATE_NETWORKS` does for model provider
endpoints.

## Known gap

The Azure DevOps SDK creates its own HTTP client. For ADO, Helix therefore checks
`organization_url` before sending the request, but it does not enforce the policy
when it connects.
