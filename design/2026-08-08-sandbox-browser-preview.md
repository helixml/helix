# Sandbox browser preview

## Goal

Add a Browser surface to the spec-task workspace that can open an HTTP service
running inside the task sandbox, for example `http://localhost:8080`.

## Request path

```text
user iframe
  -> https://share-<random>.<DEV_SUBDOMAIN>/path
  -> Helix vhost middleware
  -> API reverse proxy
  -> RevDial device hydra-<session SandboxID / sandbox host id>
  -> Hydra dev-container proxy
  -> <session container ip>:<selected port>
```

The Browser surface reuses the existing session preview-token API and
`vhost_routes` rows. It reuses an existing route for the same session and port,
or mints one on first navigation. The submitted localhost path, query, and
fragment are preserved on the browser-facing preview URL.

The Browser surface supports multiple tabs. Each tab owns its address, history,
error, and reload state. Loaded iframes stay mounted while inactive so switching
tabs preserves the embedded application's client state. The tab context menu
matches the Files surface: close, close others, close to the right, and close
all.

These routes are intentionally shareable and unauthenticated. The UI says so
before the first navigation, and routes remain visible/revocable in the task's
Details view. Session authorization is required to list, mint, rotate, or
revoke them.

## Domain and TLS requirements

`DEV_SUBDOMAIN` must resolve to the Helix API ingress. In production this means
a wildcard DNS record for `*.<DEV_SUBDOMAIN>` and TLS coverage for those dynamic
hostnames. Helix's existing vhost middleware and certmagic integration handle
dispatch and certificate issuance; no Browser-specific domain is required.

`PREVIEW_URL_HTTPS` controls the public scheme returned by the preview-token API
and used by the Browser and Share Preview UI. It defaults to `true`. Set it to
`false` for an HTTP-only local deployment. This is intentionally independent of
`HELIX_VHOST_TLS_MODE`: TLS may be terminated by an upstream ingress instead of
the embedded Helix listener.

For local development, a full value such as `dev.localhost` can use browser
loopback resolution. Set `PREVIEW_URL_HTTPS=false` when the local ingress is
HTTP-only.

Both settings are returned by `GET /api/v1/config`. When `DEV_SUBDOMAIN` is
empty, the Browser surface shows the required environment configuration and
does not query or create preview tokens.

The preview iframe uses the dark `color-scheme` and its containing viewport
clips parent overflow. This gives cross-origin embedded documents a dark native
scrollbar without injecting CSS into the sandbox application.

## WebSockets

Vite and similar dev servers rely on a WebSocket upgrade for HMR. The RevDial
HTTP transport must expose a `101 Switching Protocols` response body as an
`io.ReadWriteCloser`; otherwise `httputil.ReverseProxy` rejects the upgrade.
The upgraded body retains the buffered reader so frames received with the
handshake are not lost, and writes directly to the RevDial connection.

## Scope

The address bar accepts only HTTP loopback hosts (`localhost`, loopback IPv4,
and loopback IPv6). It does not act as a general public-web iframe browser.
Navigation history covers submitted URLs; cross-origin iframe navigation cannot
be observed by the parent page under the browser same-origin policy.
