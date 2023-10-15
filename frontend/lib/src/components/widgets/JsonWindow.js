"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const Window_1 = __importDefault(require("./Window"));
const JsonView_1 = __importDefault(require("./JsonView"));
const JsonWindow = ({ data, size = 'md', onClose, }) => {
    return ((0, jsx_runtime_1.jsx)(Window_1.default, Object.assign({ open: true, withCancel: true, size: size, cancelTitle: "Close", onCancel: onClose }, { children: (0, jsx_runtime_1.jsx)(JsonView_1.default, { data: data, scrolling: false }) })));
};
exports.default = JsonWindow;
//# sourceMappingURL=JsonWindow.js.map