// Minimal URL polyfill
export function urlToPath(url) {
  return url.replace('file://', '');
}

export function pathToFileURL(path) {
  return `file://${path}`;
}

export function isUrl(value) {
  if (typeof value !== 'string') return false;
  try {
    new URL(value);
    return true;
  } catch {
    return false;
  }
}

export default {
  urlToPath,
  pathToFileURL,
  isUrl
}; 