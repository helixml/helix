import CONFIG from "./config.js";
export function buildUrl(path) {
    var _a;
    return `${window.location.origin}${(_a = CONFIG === null || CONFIG === void 0 ? void 0 : CONFIG.path_prefix) !== null && _a !== void 0 ? _a : ""}${path}`;
}
