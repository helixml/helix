import { FC } from 'react';
import { WindowProps } from './Window';
import { TerminalTextConfig } from './TerminalText';
type TerminalWindowProps = {
    data: any;
    title?: string;
    onClose: {
        (): void;
    };
} & WindowProps & TerminalTextConfig;
declare const TerminalWindow: FC<TerminalWindowProps>;
export default TerminalWindow;
//# sourceMappingURL=TerminalWindow.d.ts.map