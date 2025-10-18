import CONFIG from "./config"

export function buildUrl(path: string): string {
    return `${window.location.origin}${CONFIG?.path_prefix ?? ""}${path}`
}