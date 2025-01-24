// Minimal process polyfill
export const minproc = {
  env: {},
  cwd: () => '/',
  platform: 'browser'
};

export default minproc; 