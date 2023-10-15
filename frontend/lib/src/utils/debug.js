"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.logger = void 0;
const logger = (...args) => {
    if (process.env.NODE_ENV === 'development') {
        console.log(...args);
    }
};
exports.logger = logger;
//# sourceMappingURL=debug.js.map