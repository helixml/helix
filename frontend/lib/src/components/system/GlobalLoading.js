"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = __importDefault(require("react"));
const Box_1 = __importDefault(require("@mui/material/Box"));
const CircularProgress_1 = __importDefault(require("@mui/material/CircularProgress"));
const Typography_1 = __importDefault(require("@mui/material/Typography"));
const loading_1 = require("../../contexts/loading");
const GlobalLoading = () => {
    const loadingContext = react_1.default.useContext(loading_1.LoadingContext);
    if (!loadingContext.loading)
        return null;
    return ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: {
            position: 'fixed',
            left: '0px',
            top: '0px',
            zIndex: 10000,
            width: '100%',
            height: '100%',
            display: 'flex',
            justifyContent: 'center',
            alignItems: 'center',
            backgroundColor: 'rgba(255, 255, 255, 0.7)'
        } }, { children: (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: {
                padding: 6,
                backgroundColor: '#ffffff',
                border: '1px solid #e5e5e5',
            } }, { children: (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: {
                    display: 'flex',
                    justifyContent: 'center',
                    alignItems: 'center',
                    height: '100%',
                } }, { children: (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: {
                        maxWidth: '100%'
                    } }, { children: (0, jsx_runtime_1.jsxs)(Box_1.default, Object.assign({ component: "div", sx: {
                            textAlign: 'center',
                            display: 'inline-block',
                        } }, { children: [(0, jsx_runtime_1.jsx)(CircularProgress_1.default, {}), (0, jsx_runtime_1.jsx)(Typography_1.default, Object.assign({ variant: 'subtitle1' }, { children: "loading..." }))] })) })) })) })) })));
};
exports.default = GlobalLoading;
//# sourceMappingURL=GlobalLoading.js.map