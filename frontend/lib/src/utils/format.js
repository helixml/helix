"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.subtractFloat = exports.formatFloat = void 0;
const formatFloat = (num) => {
    if (num === undefined) {
        return 0;
    }
    return Math.round(num * 100) / 100;
};
exports.formatFloat = formatFloat;
const subtractFloat = (a, b) => {
    const useA = a || 0;
    const useB = b || 0;
    return (0, exports.formatFloat)(useA - useB);
};
exports.subtractFloat = subtractFloat;
//# sourceMappingURL=format.js.map