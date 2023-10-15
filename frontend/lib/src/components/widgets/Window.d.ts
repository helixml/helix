import { ReactNode, FC } from 'react';
import { DialogProps } from '@mui/material/Dialog';
export interface WindowProps {
    leftButtons?: ReactNode;
    rightButtons?: ReactNode;
    buttons?: ReactNode;
    withCancel?: boolean;
    loading?: boolean;
    submitTitle?: string;
    cancelTitle?: string;
    open: boolean;
    title?: string | ReactNode;
    size?: DialogProps["maxWidth"];
    compact?: boolean;
    noScroll?: boolean;
    fullHeight?: boolean;
    noActions?: boolean;
    background?: string;
    onCancel?: {
        (): void;
    };
    onSubmit?: {
        (): void;
    };
    theme?: Record<string, string>;
    disabled?: boolean;
}
declare const Window: FC<WindowProps>;
export default Window;
//# sourceMappingURL=Window.d.ts.map