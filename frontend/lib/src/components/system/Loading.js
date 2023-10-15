"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const createStyles_1 = __importDefault(require("@mui/styles/createStyles"));
const makeStyles_1 = __importDefault(require("@mui/styles/makeStyles"));
const CircularProgress_1 = __importDefault(require("@mui/material/CircularProgress"));
const Typography_1 = __importDefault(require("@mui/material/Typography"));
const useStyles = (0, makeStyles_1.default)(theme => (0, createStyles_1.default)({
    root: {
        display: 'flex',
        justifyContent: 'center',
        alignItems: 'center',
        height: '100%',
    },
    container: {
        maxWidth: '100%'
    },
    item: {
        textAlign: 'center',
        display: 'inline-block',
    },
}));
const Loading = ({ color = 'primary', message = 'loading', children, }) => {
    const classes = useStyles();
    return ((0, jsx_runtime_1.jsx)("div", Object.assign({ className: classes.root }, { children: (0, jsx_runtime_1.jsx)("div", Object.assign({ className: classes.container }, { children: (0, jsx_runtime_1.jsxs)("div", Object.assign({ className: classes.item }, { children: [(0, jsx_runtime_1.jsx)(CircularProgress_1.default, { color: color }), message && ((0, jsx_runtime_1.jsx)(Typography_1.default, Object.assign({ variant: 'subtitle1', color: color }, { children: message }))), children] })) })) })));
};
exports.default = Loading;
//# sourceMappingURL=Loading.js.map