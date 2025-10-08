import { StreamControllerButton } from "../api_bindings.js";
// https://w3c.github.io/gamepad/#remapping
const STANDARD_BUTTONS = [
    StreamControllerButton.BUTTON_B,
    StreamControllerButton.BUTTON_A,
    StreamControllerButton.BUTTON_Y,
    StreamControllerButton.BUTTON_X,
    StreamControllerButton.BUTTON_LB,
    StreamControllerButton.BUTTON_RB,
    // These are triggers
    null,
    null,
    StreamControllerButton.BUTTON_BACK,
    StreamControllerButton.BUTTON_PLAY,
    StreamControllerButton.BUTTON_LS_CLK,
    StreamControllerButton.BUTTON_RS_CLK,
    StreamControllerButton.BUTTON_UP,
    StreamControllerButton.BUTTON_DOWN,
    StreamControllerButton.BUTTON_LEFT,
    StreamControllerButton.BUTTON_RIGHT,
    StreamControllerButton.BUTTON_SPECIAL,
];
export const SUPPORTED_BUTTONS = StreamControllerButton.BUTTON_A | StreamControllerButton.BUTTON_B | StreamControllerButton.BUTTON_X | StreamControllerButton.BUTTON_Y | StreamControllerButton.BUTTON_UP | StreamControllerButton.BUTTON_DOWN | StreamControllerButton.BUTTON_LEFT | StreamControllerButton.BUTTON_RIGHT | StreamControllerButton.BUTTON_LB | StreamControllerButton.BUTTON_RB | StreamControllerButton.BUTTON_PLAY | StreamControllerButton.BUTTON_BACK | StreamControllerButton.BUTTON_LS_CLK | StreamControllerButton.BUTTON_RS_CLK | StreamControllerButton.BUTTON_SPECIAL;
function convertStandardButton(buttonIndex, config) {
    var _a;
    let button = (_a = STANDARD_BUTTONS[buttonIndex]) !== null && _a !== void 0 ? _a : null;
    if (config === null || config === void 0 ? void 0 : config.invertAB) {
        if (button == StreamControllerButton.BUTTON_A) {
            button = StreamControllerButton.BUTTON_B;
        }
        else if (button == StreamControllerButton.BUTTON_B) {
            button = StreamControllerButton.BUTTON_A;
        }
    }
    if (config === null || config === void 0 ? void 0 : config.invertXY) {
        if (button == StreamControllerButton.BUTTON_X) {
            button = StreamControllerButton.BUTTON_Y;
        }
        else if (button == StreamControllerButton.BUTTON_Y) {
            button = StreamControllerButton.BUTTON_X;
        }
    }
    return button;
}
export function extractGamepadState(gamepad, config) {
    let buttonFlags = 0;
    for (let buttonId = 0; buttonId < gamepad.buttons.length; buttonId++) {
        const button = gamepad.buttons[buttonId];
        const buttonFlag = convertStandardButton(buttonId, config);
        if (button.pressed && buttonFlag !== null) {
            buttonFlags |= buttonFlag;
        }
    }
    const leftTrigger = gamepad.buttons[6].value;
    const rightTrigger = gamepad.buttons[7].value;
    const leftStickX = gamepad.axes[0];
    const leftStickY = gamepad.axes[1];
    const rightStickX = gamepad.axes[2];
    const rightStickY = gamepad.axes[3];
    return {
        buttonFlags,
        leftTrigger,
        rightTrigger,
        leftStickX,
        leftStickY,
        rightStickX,
        rightStickY
    };
}
