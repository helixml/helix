"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.useLoading = void 0;
const react_1 = require("react");
const loading_1 = require("../contexts/loading");
const useLoading = () => {
    const loading = (0, react_1.useContext)(loading_1.LoadingContext);
    return loading;
};
exports.useLoading = useLoading;
exports.default = exports.useLoading;
//# sourceMappingURL=useLoading.js.map