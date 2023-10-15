"use strict";
var __createBinding = (this && this.__createBinding) || (Object.create ? (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    var desc = Object.getOwnPropertyDescriptor(m, k);
    if (!desc || ("get" in desc ? !m.__esModule : desc.writable || desc.configurable)) {
      desc = { enumerable: true, get: function() { return m[k]; } };
    }
    Object.defineProperty(o, k2, desc);
}) : (function(o, m, k, k2) {
    if (k2 === undefined) k2 = k;
    o[k2] = m[k];
}));
var __setModuleDefault = (this && this.__setModuleDefault) || (Object.create ? (function(o, v) {
    Object.defineProperty(o, "default", { enumerable: true, value: v });
}) : function(o, v) {
    o["default"] = v;
});
var __importStar = (this && this.__importStar) || function (mod) {
    if (mod && mod.__esModule) return mod;
    var result = {};
    if (mod != null) for (var k in mod) if (k !== "default" && Object.prototype.hasOwnProperty.call(mod, k)) __createBinding(result, mod, k);
    __setModuleDefault(result, mod);
    return result;
};
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = __importStar(require("react"));
const Snackbar_1 = __importDefault(require("@mui/material/Snackbar"));
const Alert_1 = __importDefault(require("@mui/material/Alert"));
const snackbar_1 = require("../../contexts/snackbar");
const Snackbar = () => {
    const snackbarContext = react_1.default.useContext(snackbar_1.SnackbarContext);
    const handleClose = (0, react_1.useCallback)(() => {
        snackbarContext.setSnackbar('');
    }, []);
    if (!snackbarContext.snackbar)
        return null;
    return ((0, jsx_runtime_1.jsx)(Snackbar_1.default, Object.assign({ open: true, autoHideDuration: 5000, anchorOrigin: { vertical: 'top', horizontal: 'center' }, onClose: handleClose }, { children: (0, jsx_runtime_1.jsx)(Alert_1.default, Object.assign({ severity: snackbarContext.snackbar.severity, elevation: 6, variant: "filled", onClose: handleClose }, { children: snackbarContext.snackbar.message })) })));
};
exports.default = Snackbar;
//# sourceMappingURL=Snackbar.js.map