"use strict";
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const react_dropzone_1 = require("react-dropzone");
const FileUpload = ({}) => {
    const onDrop = (0, react_1.useCallback)(acceptedFiles => {
    }, []);
    const { getRootProps, getInputProps, isDragActive } = (0, react_dropzone_1.useDropzone)({ onDrop });
    return ((0, jsx_runtime_1.jsxs)("div", Object.assign({}, getRootProps(), { children: [(0, jsx_runtime_1.jsx)("input", Object.assign({}, getInputProps())), isDragActive ?
                (0, jsx_runtime_1.jsx)("p", { children: "Drop the files here ..." }) :
                (0, jsx_runtime_1.jsx)("p", { children: "Drag 'n' drop some files here, or click to select files" })] })));
};
exports.default = FileUpload;
//# sourceMappingURL=FileUpload.js.map