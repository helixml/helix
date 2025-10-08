import { StreamMouseButton } from "../api_bindings.js";
const BUTTON_MAPPINGS = new Array(3);
BUTTON_MAPPINGS[0] = StreamMouseButton.LEFT;
BUTTON_MAPPINGS[1] = StreamMouseButton.MIDDLE;
BUTTON_MAPPINGS[2] = StreamMouseButton.RIGHT;
export function convertToButton(event) {
    var _a;
    return (_a = BUTTON_MAPPINGS[event.button]) !== null && _a !== void 0 ? _a : null;
}
