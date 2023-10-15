import React, { FC } from 'react';
import { IFileStoreItem, IFileStoreConfig } from '../../types';
interface FileStoreDataGridProps {
    files: IFileStoreItem[];
    config: IFileStoreConfig;
    loading: boolean;
    onView: (file: IFileStoreItem) => void;
    onEdit: (file: IFileStoreItem) => void;
    onDelete: (file: IFileStoreItem) => void;
}
declare const FileStoreDataGrid: FC<React.PropsWithChildren<FileStoreDataGridProps>>;
export default FileStoreDataGrid;
//# sourceMappingURL=FileStore.d.ts.map