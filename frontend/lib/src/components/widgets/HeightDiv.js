"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const Box_1 = __importDefault(require("@mui/material/Box"));
const HeightDiv = ({ percent = 100, sx = {}, children, }) => {
    const mounted = (0, react_1.useRef)(true);
    const [width, setWidth] = (0, react_1.useState)(0);
    const ref = (0, react_1.useRef)();
    const calculateWidth = () => {
        if (!mounted.current || !ref.current)
            return;
        setWidth(ref.current.offsetWidth * percent);
    };
    (0, react_1.useEffect)(() => {
        const handleResize = () => calculateWidth();
        handleResize();
        window.addEventListener('resize', handleResize);
        return () => window.removeEventListener('resize', handleResize);
    }, []);
    (0, react_1.useEffect)(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);
    return ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ component: "div", sx: Object.assign({ width: '100%', height: `${width}px` }, sx), ref: ref }, { children: children })));
};
exports.default = HeightDiv;
//# sourceMappingURL=HeightDiv.js.map