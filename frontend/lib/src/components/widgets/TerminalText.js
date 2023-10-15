"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const Box_1 = __importDefault(require("@mui/material/Box"));
const TerminalText = ({ data, backgroundColor = '#000', color = '#fff', }) => {
    return ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: {
            width: '100%',
            padding: 2,
            margin: 0,
            backgroundColor,
            overflow: 'auto',
        } }, { children: (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "pre", sx: {
                padding: 1,
                margin: 0,
                color,
                font: 'Courier',
                fontSize: '12px',
            } }, { children: typeof (data) === 'string' ? data : JSON.stringify(data, null, 4) })) })));
};
exports.default = TerminalText;
//# sourceMappingURL=TerminalText.js.map