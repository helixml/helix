"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
exports.BoldSectionTitle = exports.RequesterNode = exports.SmallLink = exports.TinyText = exports.SmallText = void 0;
const jsx_runtime_1 = require("react/jsx-runtime");
const system_1 = require("@mui/system");
const Typography_1 = __importDefault(require("@mui/material/Typography"));
exports.SmallText = (0, system_1.styled)('span')({
    fontSize: '0.8em',
});
exports.TinyText = (0, system_1.styled)('span')({
    fontSize: '0.65em',
});
exports.SmallLink = (0, system_1.styled)('div')({
    fontSize: '0.8em',
    color: 'blue',
    cursor: 'pointer',
    textDecoration: 'underline',
});
exports.RequesterNode = (0, system_1.styled)('span')({
    fontWeight: 'bold',
    color: '#009900',
});
const BoldSectionTitle = ({ children, }) => {
    return ((0, jsx_runtime_1.jsx)(Typography_1.default, Object.assign({ variant: "subtitle1", sx: {
            fontWeight: 'bold',
        } }, { children: children })));
};
exports.BoldSectionTitle = BoldSectionTitle;
//# sourceMappingURL=GeneralText.js.map