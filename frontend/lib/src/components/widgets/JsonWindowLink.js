"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const JsonWindow_1 = __importDefault(require("./JsonWindow"));
const ClickLink_1 = __importDefault(require("./ClickLink"));
const JsonWindowLink = ({ data, className, children, }) => {
    const [open, setOpen] = (0, react_1.useState)(false);
    return ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)(ClickLink_1.default, Object.assign({ className: className, onClick: () => setOpen(true) }, { children: children })), open && ((0, jsx_runtime_1.jsx)(JsonWindow_1.default, { data: data, onClose: () => setOpen(false) }))] }));
};
exports.default = JsonWindowLink;
//# sourceMappingURL=JsonWindowLink.js.map