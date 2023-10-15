"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const makeStyles_1 = __importDefault(require("@mui/styles/makeStyles"));
const useStyles = (0, makeStyles_1.default)(theme => ({
    root: ({ scrolling }) => ({
        width: '100%',
        height: '100%',
        overflow: scrolling ? 'auto' : 'visible',
    }),
}));
const JsonView = ({ data, scrolling = false, }) => {
    const classes = useStyles({ scrolling });
    return ((0, jsx_runtime_1.jsx)("div", Object.assign({ className: classes.root }, { children: (0, jsx_runtime_1.jsx)("pre", { children: (0, jsx_runtime_1.jsx)("code", { children: JSON.stringify(data, null, 4) }) }) })));
};
exports.default = JsonView;
//# sourceMappingURL=JsonView.js.map