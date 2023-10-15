"use strict";
var __importDefault = (this && this.__importDefault) || function (mod) {
    return (mod && mod.__esModule) ? mod : { "default": mod };
};
Object.defineProperty(exports, "__esModule", { value: true });
const jsx_runtime_1 = require("react/jsx-runtime");
const react_1 = require("react");
const Box_1 = __importDefault(require("@mui/material/Box"));
const Button_1 = __importDefault(require("@mui/material/Button"));
const TextField_1 = __importDefault(require("@mui/material/TextField"));
const Add_1 = __importDefault(require("@mui/icons-material/Add"));
const DataGridWithFilters_1 = __importDefault(require("../components/datagrid/DataGridWithFilters"));
const FileStore_1 = __importDefault(require("../components/datagrid/FileStore"));
const Window_1 = __importDefault(require("../components/widgets/Window"));
const useFilestore_1 = __importDefault(require("../hooks/useFilestore"));
const useAccount_1 = __importDefault(require("../hooks/useAccount"));
const useRouter_1 = __importDefault(require("../hooks/useRouter"));
const filestore_1 = require("../utils/filestore");
const Files = () => {
    const account = (0, useAccount_1.default)();
    const filestore = (0, useFilestore_1.default)();
    const { name, params, setParams, } = (0, useRouter_1.default)();
    const [editName, setEditName] = (0, react_1.useState)('');
    const sortedFiles = (0, react_1.useMemo)(() => {
        const folders = filestore.files.filter((file) => file.directory);
        const files = filestore.files.filter((file) => !file.directory);
        return folders.concat(files);
    }, [
        filestore.files,
    ]);
    const openFolderEditor = (0, react_1.useCallback)((id) => {
        setParams({
            edit_folder_id: id,
        });
    }, [
        setParams,
    ]);
    const onViewFile = (0, react_1.useCallback)((file) => {
        if (file.directory) {
            filestore.onSetPath((0, filestore_1.getRelativePath)(filestore.config, file));
        }
        else {
            window.open(file.url);
        }
    }, [
        filestore.config,
    ]);
    const onEditFile = (0, react_1.useCallback)((file) => {
    }, []);
    const onDeleteFile = (0, react_1.useCallback)((file) => {
    }, []);
    (0, react_1.useEffect)(() => {
        //account.onSetFilestorePath(queryParams.path || '/')
    }, []);
    if (!account.user)
        return null;
    return ((0, jsx_runtime_1.jsxs)(jsx_runtime_1.Fragment, { children: [(0, jsx_runtime_1.jsx)(DataGridWithFilters_1.default, { filters: (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ sx: {
                        width: '100%',
                    } }, { children: (0, jsx_runtime_1.jsx)(Button_1.default, Object.assign({ sx: {
                            width: '100%',
                        }, variant: "contained", color: "secondary", endIcon: (0, jsx_runtime_1.jsx)(Add_1.default, {}), onClick: () => {
                            setParams({
                                edit_folder_id: 'new',
                            });
                        } }, { children: "Create Folder" })) })), datagrid: (0, jsx_runtime_1.jsx)(FileStore_1.default, { files: sortedFiles, config: filestore.config, loading: filestore.loading, onView: onViewFile, onEdit: onEditFile, onDelete: onDeleteFile }) }), params.edit_folder_id && ((0, jsx_runtime_1.jsx)(Window_1.default, Object.assign({ open: true, title: "Edit Folder", withCancel: true, onCancel: () => {
                    setParams({
                        edit_folder_id: ''
                    });
                }, onSubmit: () => {
                    console.log('--------------------------------------------');
                } }, { children: (0, jsx_runtime_1.jsx)(Box_1.default, Object.assign({ sx: {
                        p: 2,
                    } }, { children: (0, jsx_runtime_1.jsx)(TextField_1.default, { fullWidth: true, label: "Folder Name", value: editName, onChange: (e) => setEditName(e.target.value) }) })) })))] }));
};
exports.default = Files;
//# sourceMappingURL=Files.js.map