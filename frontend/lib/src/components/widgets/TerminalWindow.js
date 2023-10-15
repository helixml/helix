"use strict";
var __rest = (this && this.__rest) || function (s, e) {
    var t = {};
    for (var p in s) if (Object.prototype.hasOwnProperty.call(s, p) && e.indexOf(p) < 0)
        t[p] = s[p];
    if (s != null && typeof Object.getOwnPropertySymbols === "function")
        for (var i = 0, p = Object.getOwnPropertySymbols(s); i < p.length; i++) {
            if (e.indexOf(p[i]) < 0 && Object.prototype.propertyIsEnumerable.call(s, p[i]))
                t[p[i]] = s[p[i]];
        }
    return t;
};
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const Window_1 = __importDefault(require("./Window"));
const TerminalText_1 = __importDefault(require("./TerminalText"));
const TerminalWindow = (_a) => {
    var { data, title = 'Data', color, backgroundColor, onClose } = _a, windowProps = __rest(_a, ["data", "title", "color", "backgroundColor", "onClose"]);
    return ((0, jsx_runtime_1.jsx)(Window_1.default, Object.assign({ withCancel: true, compact: true, title: title, onCancel: onClose, cancelTitle: "Close" }, windowProps, { children: (0, jsx_runtime_1.jsx)(TerminalText_1.default, { data: data, color: color, backgroundColor: backgroundColor }) })));
};
exports.default = TerminalWindow;
//# sourceMappingURL=TerminalWindow.js.map