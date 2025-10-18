// Loader script to expose moonlight-web modules to window object
import { getApi } from './api.js';
import { Stream } from './stream/index.js';

// Expose to window for React component usage
window.MoonlightApi = { getApi };
window.MoonlightStream = { Stream };

console.log('[Moonlight] Modules loaded and exposed to window');
