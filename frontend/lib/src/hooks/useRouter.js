"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
exports.useRouter = void 0;
const react_1 = require("react");
const router_1 = require("../contexts/router");
const useRouter = () => {
    const router = (0, react_1.useContext)(router_1.RouterContext);
    return router;
};
exports.useRouter = useRouter;
exports.default = exports.useRouter;
//# sourceMappingURL=useRouter.js.map