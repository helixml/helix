import { StreamCapabilities, StreamControllerCapabilities, StreamMouseButton } from "../api_bindings"
import { ByteBuffer, I16_MAX, U16_MAX, U8_MAX } from "./buffer"
import { ControllerConfig, extractGamepadState, GamepadState, SUPPORTED_BUTTONS } from "./gamepad"
import { convertToKey, convertToModifiers } from "./keyboard"
import { convertToEvdevKey, convertToEvdevModifiers } from "./evdev-keys"
import { convertToButton } from "./mouse"

// Smooth scrolling multiplier
const TOUCH_HIGH_RES_SCROLL_MULTIPLIER = 10
// Normal scrolling multiplier
const TOUCH_SCROLL_MULTIPLIER = 1
// Distance until a touch is 100% a click
const TOUCH_AS_CLICK_MAX_DISTANCE = 30
// Time till it's registered as a click, else it might be scrolling
const TOUCH_AS_CLICK_MIN_TIME_MS = 100
// Everything greater than this is a right click
const TOUCH_AS_CLICK_MAX_TIME_MS = 300
// How much to move to open up the screen keyboard when having three touches at the same time
const TOUCHES_AS_KEYBOARD_DISTANCE = 100

const CONTROLLER_RUMBLE_INTERVAL_MS = 60

function trySendChannel(channel: RTCDataChannel | null, buffer: ByteBuffer) {
    if (!channel || channel.readyState != "open") {
        return
    }

    buffer.flip()
    const readBuffer = buffer.getReadBuffer()
    if (readBuffer.length == 0) {
        throw "illegal buffer size"
    }
    channel.send(readBuffer.buffer)
}

export type MouseScrollMode = "highres" | "normal"
export type MouseMode = "relative" | "follow" | "pointAndDrag"

export type StreamInputConfig = {
    mouseMode: MouseMode
    mouseScrollMode: MouseScrollMode
    touchMode: "touch" | "mouseRelative" | "pointAndDrag"
    controllerConfig: ControllerConfig
    // Use Linux evdev keycodes instead of Windows VK codes for direct WebSocket mode
    // This eliminates backend VK→evdev conversion for lower latency
    useEvdevCodes?: boolean
}

export function defaultStreamInputConfig(): StreamInputConfig {
    return {
        mouseMode: "follow",
        mouseScrollMode: "highres",
        touchMode: "pointAndDrag",
        controllerConfig: {
            invertAB: false,
            invertXY: false
        }
    }
}

export type ScreenKeyboardSetVisibleEvent = CustomEvent<{ visible: boolean }>

export class StreamInput {

    private eventTarget = new EventTarget()

    private peer: RTCPeerConnection | null = null

    private buffer: ByteBuffer = new ByteBuffer(1024)

    private connected = false
    private config: StreamInputConfig
    private capabilities: StreamCapabilities = { touch: true }
    // Size of the streamer device
    private streamerSize: [number, number] = [0, 0]

    private keyboard: RTCDataChannel | null = null
    private mouseClicks: RTCDataChannel | null = null
    private mouseAbsolute: RTCDataChannel | null = null
    private mouseRelative: RTCDataChannel | null = null
    private touch: RTCDataChannel | null = null
    private controllers: RTCDataChannel | null = null
    private controllerInputs: Array<RTCDataChannel | null> = []

    private touchSupported: boolean | null = null

    constructor(config?: StreamInputConfig, peer?: RTCPeerConnection,) {
        if (peer) {
            this.setPeer(peer)
        }

        this.config = defaultStreamInputConfig()
        if (config) {
            this.setConfig(config)
        }
    }

    setPeer(peer: RTCPeerConnection) {
        if (this.peer) {
            this.keyboard?.close()
            this.mouseClicks?.close()
            this.mouseAbsolute?.close()
            this.mouseRelative?.close()
            this.touch?.close()
            this.controllers?.close()
            for (const controller of this.controllerInputs.splice(this.controllerInputs.length)) {
                controller?.close()
            }
        }
        this.peer = peer

        this.keyboard = peer.createDataChannel("keyboard")

        this.mouseClicks = peer.createDataChannel("mouseClicks", {
            ordered: false
        })
        this.mouseAbsolute = peer.createDataChannel("mouseAbsolute", {
            ordered: true,
            maxRetransmits: 0
        })
        this.mouseRelative = peer.createDataChannel("mouseRelative", {
            ordered: false
        })

        this.touch = peer.createDataChannel("touch")
        this.touch.onmessage = this.onTouchMessage.bind(this)

        this.controllers = peer.createDataChannel("controllers")
        this.controllers.addEventListener("message", this.onControllerMessage.bind(this))
    }

    setConfig(config: StreamInputConfig) {
        Object.assign(this.config, config)

        // Touch
        this.primaryTouch = null
        this.touchTracker.clear()
    }
    getConfig(): StreamInputConfig {
        return this.config
    }

    getCapabilities(): StreamCapabilities {
        return this.capabilities
    }

    // -- External Event Listeners
    addScreenKeyboardVisibleEvent(listener: (event: ScreenKeyboardSetVisibleEvent) => void) {
        this.eventTarget.addEventListener("ml-screenkeyboardvisible", listener as any)
    }

    // -- On Stream Start
    onStreamStart(capabilities: StreamCapabilities, streamerSize: [number, number]) {
        this.connected = true

        this.capabilities = capabilities
        this.streamerSize = streamerSize
        this.registerBufferedControllers()
    }

    // -- Keyboard
    onKeyDown(event: KeyboardEvent) {
        this.sendKeyEvent(true, event)
    }
    onKeyUp(event: KeyboardEvent) {
        this.sendKeyEvent(false, event)
    }
    private sendKeyEvent(isDown: boolean, event: KeyboardEvent) {
        this.buffer.reset()

        // Use evdev codes for direct WebSocket mode (Linux backend)
        // VK codes are only needed for Moonlight/Wolf compatibility
        let key: number | null
        let modifiers: number
        if (this.config.useEvdevCodes) {
            key = convertToEvdevKey(event)
            modifiers = convertToEvdevModifiers(event)
        } else {
            key = convertToKey(event)
            modifiers = convertToModifiers(event)
        }
        if (!key) {
            return
        }

        this.sendKey(isDown, key, modifiers)
    }

    // Note: key = StreamKeys.VK_, modifiers = StreamKeyModifiers.
    sendKey(isDown: boolean, key: number, modifiers: number) {
        this.buffer.putU8(0)

        this.buffer.putBool(isDown)
        this.buffer.putU8(modifiers)
        this.buffer.putU16(key)

        trySendChannel(this.keyboard, this.buffer)
    }
    sendText(text: string) {
        this.buffer.putU8(1)

        this.buffer.putU8(text.length)
        this.buffer.putUtf8(text)

        trySendChannel(this.keyboard, this.buffer)
    }

    // -- Mouse
    onMouseDown(event: MouseEvent, rect: DOMRect) {
        console.log(`[StreamInput] onMouseDown: event.button=${event.button}, mouseMode=${this.config.mouseMode}`)
        const button = convertToButton(event)
        if (button == null) {
            console.warn(`[StreamInput] onMouseDown: convertToButton returned null for event.button=${event.button}`)
            return
        }
        console.log(`[StreamInput] onMouseDown: converted button=${button}`)

        if (this.config.mouseMode == "relative" || this.config.mouseMode == "follow") {
            console.log(`[StreamInput] onMouseDown: calling sendMouseButton(true, ${button})`)
            this.sendMouseButton(true, button)
        } else if (this.config.mouseMode == "pointAndDrag") {
            this.sendMousePositionClientCoordinates(event.clientX, event.clientY, rect, button)
        }
    }
    onMouseUp(event: MouseEvent) {
        const button = convertToButton(event)
        if (button == null) {
            return
        }

        this.sendMouseButton(false, button)
    }
    onMouseMove(event: MouseEvent, rect: DOMRect) {
        if (this.config.mouseMode == "relative") {
            this.sendMouseMoveClientCoordinates(event.movementX, event.movementY, rect)
        } else if (this.config.mouseMode == "follow") {
            this.sendMousePositionClientCoordinates(event.clientX, event.clientY, rect)
        } else if (this.config.mouseMode == "pointAndDrag") {
            if (event.buttons) {
                // some button pressed
                this.sendMouseMoveClientCoordinates(event.movementX, event.movementY, rect)
            }
        }
    }
    onMouseWheel(event: WheelEvent) {
        // Normalize wheel deltas to pixels, then pass through to backend.
        // Backend handles compositor-specific conversion (Mutter, Sway, etc.)
        let deltaX = event.deltaX;
        let deltaY = event.deltaY;

        // Normalize deltaMode to pixels
        if (event.deltaMode === WheelEvent.DOM_DELTA_LINE) {
            // Firefox sends lines (~3 per notch). Convert to pixels (~40px per line).
            deltaX *= 40;
            deltaY *= 40;
        } else if (event.deltaMode === WheelEvent.DOM_DELTA_PAGE) {
            // Rare. Convert to approximate pixel equivalent.
            deltaX *= window.innerWidth;
            deltaY *= window.innerHeight;
        }
        // DOM_DELTA_PIXEL: already in pixels, use as-is

        // Clamp to i16 range for wire format
        const clamp = (v: number) => Math.max(-32768, Math.min(32767, Math.round(v)));

        if (this.config.mouseScrollMode == "highres") {
            // Negate Y: browser positive = content scrolls down, Moonlight expects opposite
            this.sendMouseWheelHighRes(clamp(deltaX), clamp(-deltaY))
        } else if (this.config.mouseScrollMode == "normal") {
            // Normal mode uses Int8 (-128 to 127)
            const clampI8 = (v: number) => Math.max(-128, Math.min(127, Math.round(v / 10)));
            this.sendMouseWheel(clampI8(deltaX), clampI8(-deltaY))
        }
    }

    sendMouseMove(movementX: number, movementY: number) {
        this.buffer.reset()

        this.buffer.putU8(0)
        this.buffer.putI16(movementX)
        this.buffer.putI16(movementY)

        trySendChannel(this.mouseRelative, this.buffer)
    }
    sendMouseMoveClientCoordinates(movementX: number, movementY: number, rect: DOMRect) {
        const scaledMovementX = movementX / rect.width * this.streamerSize[0];
        const scaledMovementY = movementY / rect.height * this.streamerSize[1];

        this.sendMouseMove(scaledMovementX, scaledMovementY)
    }
    sendMousePosition(x: number, y: number, referenceWidth: number, referenceHeight: number) {
        this.buffer.reset()

        this.buffer.putU8(1)
        this.buffer.putI16(x)
        this.buffer.putI16(y)
        this.buffer.putI16(referenceWidth)
        this.buffer.putI16(referenceHeight)

        trySendChannel(this.mouseAbsolute, this.buffer)
    }
    // Debug logging counter for mouse position
    private mousePositionLogCount = 0;

    sendMousePositionClientCoordinates(clientX: number, clientY: number, rect: DOMRect, mouseButton?: number) {
        const position = this.calcNormalizedPosition(clientX, clientY, rect)
        if (position) {
            const [x, y] = position

            // Debug logging: log first 5 and then every 100th
            this.mousePositionLogCount++;
            if (this.mousePositionLogCount <= 5 || this.mousePositionLogCount % 100 === 0) {
                console.log(`[INPUT_DEBUG] sendMousePosition #${this.mousePositionLogCount}: ` +
                    `client=(${clientX.toFixed(1)},${clientY.toFixed(1)}) ` +
                    `rect=(${rect.left.toFixed(0)},${rect.top.toFixed(0)} ${rect.width.toFixed(0)}x${rect.height.toFixed(0)}) ` +
                    `normalized=(${x.toFixed(4)},${y.toFixed(4)}) ` +
                    `moonlight=(${(x * 4096).toFixed(0)},${(y * 4096).toFixed(0)} ref=4096x4096) ` +
                    `streamerSize=(${this.streamerSize[0]}x${this.streamerSize[1]})`);
            }

            this.sendMousePosition(x * 4096.0, y * 4096.0, 4096.0, 4096.0)

            if (mouseButton != undefined) {
                this.sendMouseButton(true, mouseButton)
            }
        }
    }
    // Note: button = StreamMouseButton.
    sendMouseButton(isDown: boolean, button: number) {
        // If this log appears, the WebSocket patching is NOT working!
        // In WebSocket mode, this method should be replaced by WebSocketStream.sendMouseButton
        console.error(`[StreamInput] sendMouseButton CALLED DIRECTLY (patching failed?): isDown=${isDown} button=${button}`);

        this.buffer.reset()

        this.buffer.putU8(2)
        this.buffer.putBool(isDown)
        this.buffer.putU8(button)

        trySendChannel(this.mouseClicks, this.buffer)
    }
    sendMouseWheelHighRes(deltaX: number, deltaY: number) {
        this.buffer.reset()

        this.buffer.putU8(3)
        this.buffer.putI16(deltaX)
        this.buffer.putI16(deltaY)

        trySendChannel(this.mouseClicks, this.buffer)
    }
    sendMouseWheel(deltaX: number, deltaY: number) {
        this.buffer.reset()

        this.buffer.putU8(4)
        this.buffer.putI8(deltaX)
        this.buffer.putI8(deltaY)

        trySendChannel(this.mouseClicks, this.buffer)
    }

    // -- Touch
    private touchTracker: Map<number, {
        startTime: number
        originX: number
        originY: number
        x: number
        y: number
        mouseClicked: boolean
        mouseMoved: boolean
    }> = new Map()
    private touchMouseAction: "default" | "scroll" | "screenKeyboard" = "default"
    private primaryTouch: number | null = null

    private onTouchMessage(event: MessageEvent) {
        const data = event.data
        const buffer = new ByteBuffer(data)
        this.touchSupported = buffer.getBool()
    }

    private updateTouchTracker(touch: Touch) {
        const oldTouch = this.touchTracker.get(touch.identifier)
        if (!oldTouch) {
            this.touchTracker.set(touch.identifier, {
                startTime: Date.now(),
                originX: touch.clientX,
                originY: touch.clientY,
                x: touch.clientX,
                y: touch.clientY,
                mouseMoved: false,
                mouseClicked: false
            })
        } else {
            oldTouch.x = touch.clientX
            oldTouch.y = touch.clientY
        }
    }

    private calcTouchTime(touch: { startTime: number }): number {
        return Date.now() - touch.startTime
    }
    private calcTouchOriginDistance(
        touch: { x: number, y: number } | { clientX: number, clientY: number },
        oldTouch: { originX: number, originY: number }
    ): number {
        if ("clientX" in touch) {
            return Math.hypot(touch.clientX - oldTouch.originX, touch.clientY - oldTouch.originY)
        } else {
            return Math.hypot(touch.x - oldTouch.originX, touch.y - oldTouch.originY)
        }
    }

    onTouchStart(event: TouchEvent, rect: DOMRect) {
        for (const touch of event.changedTouches) {
            this.updateTouchTracker(touch)
        }

        if (this.config.touchMode == "touch") {
            for (const touch of event.changedTouches) {
                this.sendTouch(0, touch, rect)
            }
        } else if (this.config.touchMode == "mouseRelative" || this.config.touchMode == "pointAndDrag") {
            for (const touch of event.changedTouches) {
                if (this.primaryTouch == null) {
                    this.primaryTouch = touch.identifier
                    this.touchMouseAction = "default"
                }
            }

            if (this.primaryTouch != null && this.touchTracker.size == 2) {
                const primaryTouch = this.touchTracker.get(this.primaryTouch)
                if (primaryTouch && !primaryTouch.mouseMoved && !primaryTouch.mouseClicked) {
                    this.touchMouseAction = "scroll"

                    if (this.config.touchMode == "pointAndDrag") {
                        let middleX = 0;
                        let middleY = 0;
                        for (const touch of this.touchTracker.values()) {
                            middleX += touch.x;
                            middleY += touch.y;
                        }
                        // Tracker size = 2 so there will only be 2 elements
                        middleX /= 2;
                        middleY /= 2;

                        primaryTouch.mouseMoved = true
                        this.sendMousePositionClientCoordinates(middleX, middleY, rect)
                    }
                }
            } else if (this.touchTracker.size == 3) {
                this.touchMouseAction = "screenKeyboard"
            }
        }
    }

    onTouchUpdate(rect: DOMRect) {
        if (this.config.touchMode == "pointAndDrag") {
            if (this.primaryTouch == null) {
                return
            }
            const touch = this.touchTracker.get(this.primaryTouch)
            if (!touch) {
                return
            }

            const time = this.calcTouchTime(touch)
            if (this.touchMouseAction == "default" && !touch.mouseMoved && time >= TOUCH_AS_CLICK_MIN_TIME_MS) {
                this.sendMousePositionClientCoordinates(touch.originX, touch.originY, rect)

                touch.mouseMoved = true
            }
        }
    }

    onTouchMove(event: TouchEvent, rect: DOMRect) {
        if (this.config.touchMode == "touch") {
            for (const touch of event.changedTouches) {
                this.sendTouch(1, touch, rect)
            }
        } else if (this.config.touchMode == "mouseRelative" || this.config.touchMode == "pointAndDrag") {
            for (const touch of event.changedTouches) {
                if (this.primaryTouch != touch.identifier) {
                    continue
                }
                const oldTouch = this.touchTracker.get(this.primaryTouch)
                if (!oldTouch) {
                    continue
                }

                // mouse move
                const movementX = touch.clientX - oldTouch.x;
                const movementY = touch.clientY - oldTouch.y;

                if (this.touchMouseAction == "default") {
                    this.sendMouseMoveClientCoordinates(movementX, movementY, rect)

                    const distance = this.calcTouchOriginDistance(touch, oldTouch)
                    if (this.config.touchMode == "pointAndDrag" && distance > TOUCH_AS_CLICK_MAX_DISTANCE) {
                        if (!oldTouch.mouseMoved) {
                            this.sendMousePositionClientCoordinates(touch.clientX, touch.clientY, rect)
                            oldTouch.mouseMoved = true
                        }

                        if (!oldTouch.mouseClicked) {
                            this.sendMousePositionClientCoordinates(oldTouch.originX, oldTouch.originY, rect)
                            this.sendMouseButton(true, StreamMouseButton.LEFT)
                            oldTouch.mouseClicked = true
                        }
                    }
                } else if (this.touchMouseAction == "scroll") {
                    // inverting horizontal scroll
                    if (this.config.mouseScrollMode == "highres") {
                        this.sendMouseWheelHighRes(-movementX * TOUCH_HIGH_RES_SCROLL_MULTIPLIER, movementY * TOUCH_HIGH_RES_SCROLL_MULTIPLIER)
                    } else if (this.config.mouseScrollMode == "normal") {
                        this.sendMouseWheel(-movementX * TOUCH_SCROLL_MULTIPLIER, movementY * TOUCH_SCROLL_MULTIPLIER)
                    }
                } else if (this.touchMouseAction == "screenKeyboard") {
                    const distanceY = touch.clientY - oldTouch.originY

                    if (distanceY < -TOUCHES_AS_KEYBOARD_DISTANCE) {
                        const customEvent: ScreenKeyboardSetVisibleEvent = new CustomEvent("ml-screenkeyboardvisible", {
                            detail: { visible: true }
                        })
                        this.eventTarget.dispatchEvent(customEvent)
                    } else if (distanceY > TOUCHES_AS_KEYBOARD_DISTANCE) {
                        const customEvent: ScreenKeyboardSetVisibleEvent = new CustomEvent("ml-screenkeyboardvisible", {
                            detail: { visible: false }
                        })
                        this.eventTarget.dispatchEvent(customEvent)
                    }
                }
            }
        }

        for (const touch of event.changedTouches) {
            this.updateTouchTracker(touch)
        }
    }

    onTouchEnd(event: TouchEvent, rect: DOMRect) {
        if (this.config.touchMode == "touch") {
            for (const touch of event.changedTouches) {
                this.sendTouch(2, touch, rect)
            }
        } else if (this.config.touchMode == "mouseRelative" || this.config.touchMode == "pointAndDrag") {
            for (const touch of event.changedTouches) {
                if (this.primaryTouch != touch.identifier) {
                    continue
                }
                const oldTouch = this.touchTracker.get(this.primaryTouch)
                this.primaryTouch = null

                if (oldTouch) {
                    const time = this.calcTouchTime(oldTouch)
                    const distance = this.calcTouchOriginDistance(touch, oldTouch)

                    if (this.touchMouseAction == "default") {
                        if (distance <= TOUCH_AS_CLICK_MAX_DISTANCE) {
                            if (time <= TOUCH_AS_CLICK_MAX_TIME_MS || oldTouch.mouseClicked) {
                                if (this.config.touchMode == "pointAndDrag" && !oldTouch.mouseMoved) {
                                    this.sendMousePositionClientCoordinates(touch.clientX, touch.clientY, rect)
                                }
                                if (!oldTouch.mouseClicked) {
                                    this.sendMouseButton(true, StreamMouseButton.LEFT)
                                }
                                this.sendMouseButton(false, StreamMouseButton.LEFT)
                            } else {
                                this.sendMouseButton(true, StreamMouseButton.RIGHT)
                                this.sendMouseButton(false, StreamMouseButton.RIGHT)
                            }
                        } else if (this.config.touchMode == "pointAndDrag") {
                            this.sendMouseButton(true, StreamMouseButton.LEFT)
                            this.sendMouseButton(false, StreamMouseButton.LEFT)
                        }
                    }
                }
            }
        }

        for (const touch of event.changedTouches) {
            this.touchTracker.delete(touch.identifier)
        }
    }

    onTouchCancel(event: TouchEvent, rect: DOMRect) {
        this.onTouchEnd(event, rect)
    }

    private calcNormalizedPosition(clientX: number, clientY: number, rect: DOMRect): [number, number] | null {
        const x = (clientX - rect.left) / rect.width
        const y = (clientY - rect.top) / rect.height

        if (x < 0 || x > 1.0 || y < 0 || y > 1.0) {
            // invalid touch
            return null
        }
        return [x, y]
    }
    private sendTouch(type: number, touch: Touch, rect: DOMRect) {
        this.buffer.reset()

        this.buffer.putU8(type)

        this.buffer.putU32(touch.identifier)

        const position = this.calcNormalizedPosition(touch.clientX, touch.clientY, rect)
        if (!position) {
            return
        }
        const [x, y] = position
        this.buffer.putF32(x)
        this.buffer.putF32(y)

        this.buffer.putF32(touch.force)

        this.buffer.putF32(touch.radiusX)
        this.buffer.putF32(touch.radiusY)
        this.buffer.putU16(touch.rotationAngle)

        trySendChannel(this.touch, this.buffer)
    }

    isTouchSupported(): boolean | null {
        return this.touchSupported
    }

    // -- Controller
    // Wait for stream to connect and then send controllers
    private bufferedControllers: Array<number> = []
    private registerBufferedControllers() {
        const gamepads = navigator.getGamepads()

        for (const index of this.bufferedControllers.splice(0)) {
            const gamepad = gamepads[index]
            if (gamepad) {
                this.onGamepadConnect(gamepad)
            }
        }
    }

    private collectActuators(gamepad: Gamepad): Array<GamepadHapticActuator> {
        const actuators = []
        if ("vibrationActuator" in gamepad && gamepad.vibrationActuator) {
            actuators.push(gamepad.vibrationActuator)
        }
        if ("hapticActuators" in gamepad && gamepad.hapticActuators) {
            const hapticActuators = gamepad.hapticActuators as Array<GamepadHapticActuator>
            actuators.push(...hapticActuators)
        }
        return actuators
    }

    private gamepads: Array<number | null> = []
    private gamepadRumbleInterval: number | null = null

    onGamepadConnect(gamepad: Gamepad) {
        if (!this.connected) {
            this.bufferedControllers.push(gamepad.index)
            return
        }

        if (this.gamepads.indexOf(gamepad.index) != -1) {
            return
        }

        let id = -1
        for (let i = 0; i < this.gamepads.length; i++) {
            if (this.gamepads[i] == null) {
                this.gamepads[i] = gamepad.index
                id = i
                break
            }
        }
        if (id == -1) {
            id = this.gamepads.length
            this.gamepads.push(gamepad.index)
        }

        // Start Rumble interval
        if (this.gamepadRumbleInterval == null) {
            this.gamepadRumbleInterval = window.setInterval(this.onGamepadRumbleInterval.bind(this), CONTROLLER_RUMBLE_INTERVAL_MS - 10)
        }

        // Reset rumble
        this.gamepadRumbleCurrent[0] = { lowFrequencyMotor: 0, highFrequencyMotor: 0, leftTrigger: 0, rightTrigger: 0 }

        let capabilities = 0

        // Rumble capabilities
        for (const actuator of this.collectActuators(gamepad)) {
            if ("effects" in actuator) {
                const supportedEffects = actuator.effects as Array<string>

                for (const effect of supportedEffects) {
                    if (effect == "dual-rumble") {
                        capabilities = StreamControllerCapabilities.CAPABILITY_RUMBLE
                    } else if (effect == "trigger-rumble") {
                        capabilities = StreamControllerCapabilities.CAPABILITY_TRIGGER_RUMBLE
                    }
                }
            } else if ("type" in actuator && (actuator.type == "vibration" || actuator.type == "dual-rumble")) {
                capabilities = StreamControllerCapabilities.CAPABILITY_RUMBLE
            } else if ("playEffect" in actuator && typeof actuator.playEffect == "function") {
                // we're just hoping at this point
                capabilities = StreamControllerCapabilities.CAPABILITY_RUMBLE | StreamControllerCapabilities.CAPABILITY_TRIGGER_RUMBLE
            } else if ("pulse" in actuator && typeof actuator.pulse == "function") {
                capabilities = StreamControllerCapabilities.CAPABILITY_RUMBLE
            }
        }

        this.sendControllerAdd(this.gamepads.length - 1, SUPPORTED_BUTTONS, capabilities)

        if (gamepad.mapping != "standard") {
            console.warn(`[Gamepad]: Unable to read values of gamepad with mapping ${gamepad.mapping}`)
        }
    }
    onGamepadDisconnect(event: GamepadEvent) {
        const index = this.gamepads.indexOf(event.gamepad.index)
        if (index != -1) {
            const id = this.gamepads[index]
            if (id != null) {
                this.sendControllerRemove(id)
            }

            this.gamepads[index] = null
        }
    }
    onGamepadUpdate() {
        for (let gamepadId = 0; gamepadId < this.gamepads.length; gamepadId++) {
            const gamepadIndex = this.gamepads[gamepadId]
            if (gamepadIndex == null) {
                return
            }
            const gamepad = navigator.getGamepads()[gamepadIndex]
            if (!gamepad) {
                continue
            }

            if (gamepad.mapping != "standard") {
                continue
            }

            const state = extractGamepadState(gamepad, this.config.controllerConfig)

            this.sendController(gamepadId, state)
        }
    }

    private onControllerMessage(event: MessageEvent) {
        if (!(event.data instanceof ArrayBuffer)) {
            return
        }
        this.buffer.reset()

        this.buffer.putU8Array(new Uint8Array(event.data))
        this.buffer.flip()

        const ty = this.buffer.getU8()
        if (ty == 0) {
            // Rumble
            const id = this.buffer.getU8()
            const lowFrequencyMotor = this.buffer.getU16() / U16_MAX
            const highFrequencyMotor = this.buffer.getU16() / U16_MAX

            const gamepadIndex = this.gamepads[id]
            if (gamepadIndex == null) {
                return
            }

            this.setGamepadEffect(gamepadIndex, "dual-rumble", { lowFrequencyMotor, highFrequencyMotor })
        } else if (ty == 1) {
            // Trigger Rumble
            const id = this.buffer.getU8()
            const leftTrigger = this.buffer.getU16() / U16_MAX
            const rightTrigger = this.buffer.getU16() / U16_MAX

            const gamepadIndex = this.gamepads[id]
            if (gamepadIndex == null) {
                return
            }

            this.setGamepadEffect(gamepadIndex, "trigger-rumble", { leftTrigger, rightTrigger })
        }
    }

    // -- Controller rumble
    private gamepadRumbleCurrent: Array<{
        lowFrequencyMotor: number, highFrequencyMotor: number,
        leftTrigger: number, rightTrigger: number
    }> = []

    private setGamepadEffect(id: number, ty: "dual-rumble", params: { lowFrequencyMotor: number, highFrequencyMotor: number }): void
    private setGamepadEffect(id: number, ty: "trigger-rumble", params: { leftTrigger: number, rightTrigger: number }): void

    private setGamepadEffect(id: number, _ty: "dual-rumble" | "trigger-rumble", params: { lowFrequencyMotor: number, highFrequencyMotor: number } | { leftTrigger: number, rightTrigger: number }) {
        const rumble = this.gamepadRumbleCurrent[id]

        Object.assign(rumble, params)
    }

    private onGamepadRumbleInterval() {
        for (let id = 0; id < this.gamepads.length; id++) {
            const gamepadIndex = this.gamepads[id]
            if (gamepadIndex == null) {
                continue
            }

            const rumble = this.gamepadRumbleCurrent[gamepadIndex]
            const gamepad = navigator.getGamepads()[gamepadIndex]
            if (gamepad && rumble) {
                this.refreshGamepadRumble(rumble, gamepad)
            }
        }
    }
    private refreshGamepadRumble(
        rumble: {
            lowFrequencyMotor: number, highFrequencyMotor: number,
            leftTrigger: number, rightTrigger: number
        },
        gamepad: Gamepad
    ) {
        // Browsers are making this more complicated than it is

        const actuators = this.collectActuators(gamepad)

        for (const actuator of actuators) {
            if ("effects" in actuator) {
                const supportedEffects = actuator.effects as Array<string>

                for (const effect of supportedEffects) {
                    if (effect == "dual-rumble") {
                        actuator.playEffect("dual-rumble", {
                            duration: CONTROLLER_RUMBLE_INTERVAL_MS,
                            weakMagnitude: rumble.lowFrequencyMotor,
                            strongMagnitude: rumble.highFrequencyMotor
                        })
                    } else if (effect == "trigger-rumble") {
                        actuator.playEffect("trigger-rumble", {
                            duration: CONTROLLER_RUMBLE_INTERVAL_MS,
                            leftTrigger: rumble.leftTrigger,
                            rightTrigger: rumble.rightTrigger
                        })
                    }
                }
            } else if ("type" in actuator && (actuator.type == "vibration" || actuator.type == "dual-rumble")) {
                actuator.playEffect(actuator.type as any, {
                    duration: CONTROLLER_RUMBLE_INTERVAL_MS,
                    weakMagnitude: rumble.lowFrequencyMotor,
                    strongMagnitude: rumble.highFrequencyMotor
                })
            } else if ("playEffect" in actuator && typeof actuator.playEffect == "function") {
                actuator.playEffect("dual-rumble", {
                    duration: CONTROLLER_RUMBLE_INTERVAL_MS,
                    weakMagnitude: rumble.lowFrequencyMotor,
                    strongMagnitude: rumble.highFrequencyMotor
                })
                actuator.playEffect("trigger-rumble", {
                    duration: CONTROLLER_RUMBLE_INTERVAL_MS,
                    leftTrigger: rumble.leftTrigger,
                    rightTrigger: rumble.rightTrigger
                })
            } else if ("pulse" in actuator && typeof actuator.pulse == "function") {
                const weak = Math.min(Math.max(rumble.lowFrequencyMotor, 0), 1);
                const strong = Math.min(Math.max(rumble.highFrequencyMotor, 0), 1);

                const average = (weak + strong) / 2.0

                actuator.pulse(average, CONTROLLER_RUMBLE_INTERVAL_MS)
            }
        }
    }

    // -- Controller Sending
    sendControllerAdd(id: number, supportedButtons: number, capabilities: number) {
        this.buffer.reset()

        this.buffer.putU8(0)
        this.buffer.putU8(id)
        this.buffer.putU32(supportedButtons)
        this.buffer.putU16(capabilities)

        trySendChannel(this.controllers, this.buffer)
    }
    sendControllerRemove(id: number) {
        this.buffer.reset()

        this.buffer.putU8(1)
        this.buffer.putU8(id)

        trySendChannel(this.controllers, this.buffer)
    }
    // Values
    // - Trigger: range 0..1
    // - Stick: range -1..1
    sendController(id: number, state: GamepadState) {
        this.buffer.reset()

        this.buffer.putU8(0)
        this.buffer.putU32(state.buttonFlags)
        this.buffer.putU8(Math.max(0.0, Math.min(1.0, state.leftTrigger)) * U8_MAX)
        this.buffer.putU8(Math.max(0.0, Math.min(1.0, state.rightTrigger)) * U8_MAX)
        this.buffer.putI16(Math.max(-1.0, Math.min(1.0, state.leftStickX)) * I16_MAX)
        this.buffer.putI16(Math.max(-1.0, Math.min(1.0, -state.leftStickY)) * I16_MAX)
        this.buffer.putI16(Math.max(-1.0, Math.min(1.0, state.rightStickX)) * I16_MAX)
        this.buffer.putI16(Math.max(-1.0, Math.min(1.0, -state.rightStickY)) * I16_MAX)

        this.tryOpenControllerChannel(id)
        trySendChannel(this.controllerInputs[id], this.buffer)
    }
    private tryOpenControllerChannel(id: number) {
        if (!this.controllerInputs[id]) {
            this.controllerInputs[id] = this.peer?.createDataChannel(`controller${id}`, {
                maxRetransmits: 0,
                ordered: true,
            }) ?? null
        }
    }

}