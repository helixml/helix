// Minimal path polyfill
export const minpath = {
  join(...parts) {
    return parts.join('/').replace(/\/+/g, '/');
  },
  
  resolve(...parts) {
    return this.join(...parts);
  },
  
  dirname(path) {
    return path.replace(/\/[^/]*$/, '');
  },
  
  basename(path) {
    return path.split('/').pop();
  }
};

export default minpath; 