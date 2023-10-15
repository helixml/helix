"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const Visibility_1 = __importDefault(require("@mui/icons-material/Visibility"));
const Edit_1 = __importDefault(require("@mui/icons-material/Edit"));
const Delete_1 = __importDefault(require("@mui/icons-material/Delete"));
const Folder_1 = __importDefault(require("@mui/icons-material/Folder"));
const Box_1 = __importDefault(require("@mui/material/Box"));
const Avatar_1 = __importDefault(require("@mui/material/Avatar"));
const pretty_bytes_1 = __importDefault(require("pretty-bytes"));
const DataGrid_1 = __importDefault(require("./DataGrid"));
const ClickLink_1 = __importDefault(require("../widgets/ClickLink"));
const getFileExtension = (filename) => {
    const parts = filename.split('.');
    return parts[parts.length - 1];
};
const isImage = (filename) => {
    if (!filename)
        return false;
    if (filename.match(/\.(jpg)|(png)|(jpeg)|(gif)$/i))
        return true;
    return false;
};
const FileStoreDataGrid = ({ files, config, loading, onView, onEdit, onDelete, }) => {
    const columns = (0, react_1.useMemo)(() => {
        return [{
                name: 'icon',
                header: '',
                defaultWidth: 100,
                render: ({ data }) => {
                    let icon = null;
                    if (isImage(data.name)) {
                        icon = ((0, jsx_runtime_1.jsx)(Box_1.default, { component: 'img', sx: {
                                maxWidth: '50px',
                                maxHeight: '50px',
                                border: '1px solid',
                                borderColor: 'secondary.main',
                            }, src: data.url }));
                    }
                    else if (data.directory) {
                        icon = ((0, jsx_runtime_1.jsx)(Avatar_1.default, { children: (0, jsx_runtime_1.jsx)(Folder_1.default, {}) }));
                    }
                    else {
                        icon = ((0, jsx_runtime_1.jsx)("span", { className: `fiv-viv fiv-size-md fiv-icon-${getFileExtension(data.name)}` }));
                    }
                    return ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ sx: {
                            width: '100%',
                            height: '100%',
                            display: 'flex',
                            flexDirection: 'row',
                            alignItems: 'center',
                            justifyContent: 'center',
                        } }, { children: (0, jsx_runtime_1.jsx)(ClickLink_1.default, Object.assign({ onClick: () => {
                                onView(data);
                            } }, { children: icon })) })));
                }
            },
            {
                name: 'name',
                header: 'Name',
                defaultFlex: 1,
                render: ({ data }) => {
                    return ((0, jsx_runtime_1.jsx)("a", Object.assign({ href: "#", onClick: (e) => {
                            e.preventDefault();
                            e.stopPropagation();
                            onView(data);
                        } }, { children: data.name })));
                }
            },
            {
                name: 'updated',
                header: 'Updated',
                defaultFlex: 1,
                render: ({ data }) => {
                    return ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ sx: {
                            fontSize: '0.9em',
                        } }, { children: new Date(data.created * 1000).toLocaleString() })));
                }
            },
            {
                name: 'size',
                header: 'Size',
                defaultFlex: 1,
                render: ({ data }) => {
                    return data.directory ? null : ((0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ sx: {
                            fontSize: '0.9em',
                        } }, { children: (0, pretty_bytes_1.default)(data.size) })));
                }
            },
            {
                name: 'actions',
                header: 'Actions',
                minWidth: 160,
                defaultWidth: 160,
                render: ({ data }) => {
                    return ((0, jsx_runtime_1.jsxs)(Box_1.default, Object.assign({ sx: {
                            width: '100%',
                            display: 'flex',
                            flexDirection: 'row',
                            alignItems: 'flex-end',
                            justifyContent: 'space-between',
                            pl: 2,
                            pr: 2,
                        } }, { children: [(0, jsx_runtime_1.jsx)(ClickLink_1.default, Object.assign({ onClick: () => {
                                    console.log('--------------------------------------------');
                                } }, { children: (0, jsx_runtime_1.jsx)(Delete_1.default, {}) })), (0, jsx_runtime_1.jsx)(ClickLink_1.default, Object.assign({ onClick: () => {
                                    console.log('--------------------------------------------');
                                } }, { children: (0, jsx_runtime_1.jsx)(Edit_1.default, {}) })), (0, jsx_runtime_1.jsx)(ClickLink_1.default, Object.assign({ onClick: () => {
                                    onView(data);
                                } }, { children: (0, jsx_runtime_1.jsx)(Visibility_1.default, {}) }))] })));
                }
            }];
    }, [
        onView,
        onEdit,
        onDelete,
    ]);
    return ((0, jsx_runtime_1.jsx)(DataGrid_1.default, { autoSort: true, userSelect: true, rows: files, columns: columns, loading: loading }));
};
exports.default = FileStoreDataGrid;
//# sourceMappingURL=FileStore.js.map