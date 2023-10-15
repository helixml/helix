"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const Box_1 = __importDefault(require("@mui/material/Box"));
const ClickLink = ({ className, textDecoration = false, onClick, children, }) => {
    const onOpen = (0, react_1.useCallback)((e) => {
        e.preventDefault();
        e.stopPropagation();
        onClick();
        return false;
    }, [
        onClick,
    ]);
    return ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: 'a', href: '#', onClick: onOpen, className: className, sx: {
            color: 'primary.main',
            textDecoration: textDecoration ? 'underline' : 'none',
        } }, { children: children })));
};
exports.default = ClickLink;
//# sourceMappingURL=ClickLink.js.map