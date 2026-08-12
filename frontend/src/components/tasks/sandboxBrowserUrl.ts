export interface SandboxBrowserTarget {
  displayUrl: string
  port: number
  path: string
}

const LOOPBACK_HOSTS = new Set(['localhost', '127.0.0.1', '0.0.0.0', '::1', '[::1]'])

export function parseSandboxBrowserTarget(input: string): SandboxBrowserTarget {
  const trimmed = input.trim()
  if (!trimmed) {
    throw new Error('Enter a localhost URL')
  }

  let url: URL
  try {
    url = new URL(trimmed.includes('://') ? trimmed : `http://${trimmed}`)
  } catch {
    throw new Error('Enter a valid localhost URL')
  }

  if (url.protocol !== 'http:') {
    throw new Error('Sandbox browser previews currently support HTTP localhost URLs')
  }
  if (!LOOPBACK_HOSTS.has(url.hostname)) {
    throw new Error('Only localhost URLs running inside this sandbox can be previewed')
  }
  if (url.username || url.password) {
    throw new Error('Credentials are not allowed in preview URLs')
  }

  const port = url.port ? Number.parseInt(url.port, 10) : 80
  if (!Number.isInteger(port) || port < 1 || port > 65535) {
    throw new Error('Port must be between 1 and 65535')
  }

  return {
    displayUrl: url.href,
    port,
    path: `${url.pathname}${url.search}${url.hash}`,
  }
}

export function sandboxPreviewUrl(
  baseUrl: string,
  path: string,
  https: boolean,
): string {
  const base = new URL(sandboxPreviewURLWithScheme(baseUrl, https))
  const normalizedPath = path.startsWith('/') ? path : `/${path}`
  return new URL(normalizedPath, `${base.origin}/`).toString()
}

export function sandboxPreviewURLWithScheme(url: string, https: boolean): string {
  const base = new URL(url)
  if (base.protocol !== 'http:' && base.protocol !== 'https:') {
    throw new Error('Preview URL must use HTTP or HTTPS')
  }
  base.protocol = https ? 'https:' : 'http:'
  return base.toString()
}
