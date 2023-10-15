import React, { FC } from 'react';
import { DialogProps } from '@mui/material/Dialog';
interface JsonWindowProps {
    data: any;
    size?: DialogProps["maxWidth"];
    onClose: {
        (): void;
    };
}
declare const JsonWindow: FC<React.PropsWithChildren<JsonWindowProps>>;
export default JsonWindow;
//# sourceMappingURL=JsonWindow.d.ts.map