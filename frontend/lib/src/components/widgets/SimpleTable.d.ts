import React, { FC } from 'react';
export interface ITableField {
    name: string;
    title: string;
    numeric?: boolean;
    style?: React.CSSProperties;
    className?: string;
}
declare const SimpleTable: FC<{
    fields: ITableField[];
    data: Record<string, any>[];
    compact?: boolean;
    withContainer?: boolean;
    hideHeader?: boolean;
    hideHeaderIfEmpty?: boolean;
    actionsTitle?: string;
    actionsFieldClassname?: string;
    onRowClick?: {
        (row: Record<string, any>): void;
    };
    getActions?: {
        (row: Record<string, any>): JSX.Element;
    };
}>;
export default SimpleTable;
//# sourceMappingURL=SimpleTable.d.ts.map