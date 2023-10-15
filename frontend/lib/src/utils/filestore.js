"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.isPathReadonly = exports.getRelativePath = void 0;
const getRelativePath = (config, file) => {
    const { user_prefix } = config;
    const { path } = file;
    if (path.startsWith(user_prefix)) {
        return path.substring(user_prefix.length);
    }
    return path;
};
exports.getRelativePath = getRelativePath;
const isPathReadonly = (config, path) => {
    const parts = path.split('/');
    const rootFolder = parts[1];
    if (!rootFolder)
        return false;
    const folder = config.folders.find(f => f.name === rootFolder);
    return (folder === null || folder === void 0 ? void 0 : folder.readonly) || false;
};
exports.isPathReadonly = isPathReadonly;
//# sourceMappingURL=filestore.js.map