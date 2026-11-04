# Outbound URL policy

Some endpoints make server-side requests to URLs supplied by users, for example
the self-hosted GitHub Enterprise / GitLab `base_url` and Azure DevOps
`organization_url` on `POST /api/v1/git/browse-remote`. These go through
`api/pkg/outbound`. It:

- allows only `http`/`https` URLs with a host and without embedded credentials
- refuses loopback, RFC1918/ULA private, CGNAT, link-local (including cloud
  metadata `169.254.169.254`), multicast and reserved addresses. It checks the
  *resolved* IP when it dials, so DNS rebinding and numeric IP encodings don't
  get around it. If a host name resolves to several addresses and any one of
  them is blocked, the request is refused.
- follows redirects only when they stay on the same scheme and host
- ignores `HTTP(S)_PROXY`, because a proxy would make the connection on Helix's
  behalf and skip the destination check

If a rejected URL is the cause, the caller gets a generic 400. The specific
reason is logged on the server.

## Allowing an internal Git server

If your GitLab / GitHub Enterprise / Azure DevOps Server runs on a private
network, allow its range explicitly:

```
HELIX_OUTBOUND_ALLOWED_PRIVATE_CIDRS=10.20.0.0/16,192.168.50.10/32
```

Helix refuses to start if any entry is not a valid CIDR.

## Known gap

The Azure DevOps SDK creates its own HTTP client. For ADO, Helix therefore checks
`organization_url` before sending the request, but it does not enforce the
policy when it dials.
