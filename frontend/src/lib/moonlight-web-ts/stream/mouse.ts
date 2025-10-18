import { StreamMouseButton } from "../api_bindings"

const BUTTON_MAPPINGS = new Array(3)
BUTTON_MAPPINGS[0] = StreamMouseButton.LEFT
BUTTON_MAPPINGS[1] = StreamMouseButton.MIDDLE
BUTTON_MAPPINGS[2] = StreamMouseButton.RIGHT

export function convertToButton(event: MouseEvent): number | null {
    return BUTTON_MAPPINGS[event.button] ?? null
}