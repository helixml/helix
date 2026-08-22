# Local vhost public URLs

Project web-service links were assembled in the frontend as
`https://<hostname>/`. This bypassed the server's vhost URL configuration and
made every link invalid in HTTP development deployments whose Helix API is
published on a non-default port.

The API now populates `VHostRoute.URL` for both sandbox previews and project
web-service domains using the same rules:

- `PREVIEW_URL_HTTPS` selects HTTP or HTTPS.
- The explicit port in `SERVER_URL` is preserved.
- No port is added when `SERVER_URL` does not specify one.

The project web-service UI opens and copies that API-provided URL instead of
reconstructing it from the hostname.
