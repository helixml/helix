/**
 * Browser keyboard event.code to Linux evdev keycode mapping.
 *
 * This module provides direct mapping from browser KeyboardEvent.code values
 * to Linux evdev keycodes, eliminating the need for VK code translation.
 *
 * evdev keycodes are defined in:
 * - Linux kernel: include/uapi/linux/input-event-codes.h
 * - Reference: https://github.com/torvalds/linux/blob/master/include/uapi/linux/input-event-codes.h
 */

// Linux evdev key codes (from input-event-codes.h)
export const EvdevKey = {
  KEY_ESC: 1,
  KEY_1: 2,
  KEY_2: 3,
  KEY_3: 4,
  KEY_4: 5,
  KEY_5: 6,
  KEY_6: 7,
  KEY_7: 8,
  KEY_8: 9,
  KEY_9: 10,
  KEY_0: 11,
  KEY_MINUS: 12,
  KEY_EQUAL: 13,
  KEY_BACKSPACE: 14,
  KEY_TAB: 15,
  KEY_Q: 16,
  KEY_W: 17,
  KEY_E: 18,
  KEY_R: 19,
  KEY_T: 20,
  KEY_Y: 21,
  KEY_U: 22,
  KEY_I: 23,
  KEY_O: 24,
  KEY_P: 25,
  KEY_LEFTBRACE: 26,
  KEY_RIGHTBRACE: 27,
  KEY_ENTER: 28,
  KEY_LEFTCTRL: 29,
  KEY_A: 30,
  KEY_S: 31,
  KEY_D: 32,
  KEY_F: 33,
  KEY_G: 34,
  KEY_H: 35,
  KEY_J: 36,
  KEY_K: 37,
  KEY_L: 38,
  KEY_SEMICOLON: 39,
  KEY_APOSTROPHE: 40,
  KEY_GRAVE: 41,
  KEY_LEFTSHIFT: 42,
  KEY_BACKSLASH: 43,
  KEY_Z: 44,
  KEY_X: 45,
  KEY_C: 46,
  KEY_V: 47,
  KEY_B: 48,
  KEY_N: 49,
  KEY_M: 50,
  KEY_COMMA: 51,
  KEY_DOT: 52,
  KEY_SLASH: 53,
  KEY_RIGHTSHIFT: 54,
  KEY_KPASTERISK: 55,
  KEY_LEFTALT: 56,
  KEY_SPACE: 57,
  KEY_CAPSLOCK: 58,
  KEY_F1: 59,
  KEY_F2: 60,
  KEY_F3: 61,
  KEY_F4: 62,
  KEY_F5: 63,
  KEY_F6: 64,
  KEY_F7: 65,
  KEY_F8: 66,
  KEY_F9: 67,
  KEY_F10: 68,
  KEY_NUMLOCK: 69,
  KEY_SCROLLLOCK: 70,
  KEY_KP7: 71,
  KEY_KP8: 72,
  KEY_KP9: 73,
  KEY_KPMINUS: 74,
  KEY_KP4: 75,
  KEY_KP5: 76,
  KEY_KP6: 77,
  KEY_KPPLUS: 78,
  KEY_KP1: 79,
  KEY_KP2: 80,
  KEY_KP3: 81,
  KEY_KP0: 82,
  KEY_KPDOT: 83,
  KEY_102ND: 86,
  KEY_F11: 87,
  KEY_F12: 88,
  KEY_KPENTER: 96,
  KEY_RIGHTCTRL: 97,
  KEY_KPSLASH: 98,
  KEY_SYSRQ: 99,
  KEY_RIGHTALT: 100,
  KEY_HOME: 102,
  KEY_UP: 103,
  KEY_PAGEUP: 104,
  KEY_LEFT: 105,
  KEY_RIGHT: 106,
  KEY_END: 107,
  KEY_DOWN: 108,
  KEY_PAGEDOWN: 109,
  KEY_INSERT: 110,
  KEY_DELETE: 111,
  KEY_MUTE: 113,
  KEY_VOLUMEDOWN: 114,
  KEY_VOLUMEUP: 115,
  KEY_PAUSE: 119,
  KEY_KPCOMMA: 121,
  KEY_LEFTMETA: 125,
  KEY_RIGHTMETA: 126,
  KEY_COMPOSE: 127,
  KEY_STOP: 128,
  KEY_HELP: 138,
  KEY_SLEEP: 142,
  KEY_PROG1: 148,
  KEY_PROG2: 149,
  KEY_MAIL: 155,
  KEY_BOOKMARKS: 156,
  KEY_BACK: 158,
  KEY_FORWARD: 159,
  KEY_NEXTSONG: 163,
  KEY_PLAYPAUSE: 164,
  KEY_PREVIOUSSONG: 165,
  KEY_STOPCD: 166,
  KEY_HOMEPAGE: 172,
  KEY_REFRESH: 173,
  KEY_F13: 183,
  KEY_F14: 184,
  KEY_F15: 185,
  KEY_F16: 186,
  KEY_F17: 187,
  KEY_F18: 188,
  KEY_F19: 189,
  KEY_F20: 190,
  KEY_F21: 191,
  KEY_F22: 192,
  KEY_F23: 193,
  KEY_F24: 194,
  KEY_SEARCH: 217,
  KEY_MEDIA: 226,
} as const

/**
 * Maps browser KeyboardEvent.code to Linux evdev keycode.
 * Returns null if the key has no evdev mapping.
 *
 * Reference: https://developer.mozilla.org/en-US/docs/Web/API/UI_Events/Keyboard_event_code_values
 */
const EVDEV_MAPPINGS: Record<string, number | null> = {
  // Control keys
  Escape: EvdevKey.KEY_ESC,
  Backspace: EvdevKey.KEY_BACKSPACE,
  Tab: EvdevKey.KEY_TAB,
  Enter: EvdevKey.KEY_ENTER,
  Space: EvdevKey.KEY_SPACE,
  CapsLock: EvdevKey.KEY_CAPSLOCK,
  Pause: EvdevKey.KEY_PAUSE,

  // Modifier keys
  ShiftLeft: EvdevKey.KEY_LEFTSHIFT,
  ShiftRight: EvdevKey.KEY_RIGHTSHIFT,
  ControlLeft: EvdevKey.KEY_LEFTCTRL,
  ControlRight: EvdevKey.KEY_RIGHTCTRL,
  AltLeft: EvdevKey.KEY_LEFTALT,
  AltRight: EvdevKey.KEY_RIGHTALT,
  MetaLeft: EvdevKey.KEY_LEFTMETA,
  MetaRight: EvdevKey.KEY_RIGHTMETA,
  ContextMenu: EvdevKey.KEY_COMPOSE,

  // Number row
  Digit1: EvdevKey.KEY_1,
  Digit2: EvdevKey.KEY_2,
  Digit3: EvdevKey.KEY_3,
  Digit4: EvdevKey.KEY_4,
  Digit5: EvdevKey.KEY_5,
  Digit6: EvdevKey.KEY_6,
  Digit7: EvdevKey.KEY_7,
  Digit8: EvdevKey.KEY_8,
  Digit9: EvdevKey.KEY_9,
  Digit0: EvdevKey.KEY_0,

  // Letters
  KeyA: EvdevKey.KEY_A,
  KeyB: EvdevKey.KEY_B,
  KeyC: EvdevKey.KEY_C,
  KeyD: EvdevKey.KEY_D,
  KeyE: EvdevKey.KEY_E,
  KeyF: EvdevKey.KEY_F,
  KeyG: EvdevKey.KEY_G,
  KeyH: EvdevKey.KEY_H,
  KeyI: EvdevKey.KEY_I,
  KeyJ: EvdevKey.KEY_J,
  KeyK: EvdevKey.KEY_K,
  KeyL: EvdevKey.KEY_L,
  KeyM: EvdevKey.KEY_M,
  KeyN: EvdevKey.KEY_N,
  KeyO: EvdevKey.KEY_O,
  KeyP: EvdevKey.KEY_P,
  KeyQ: EvdevKey.KEY_Q,
  KeyR: EvdevKey.KEY_R,
  KeyS: EvdevKey.KEY_S,
  KeyT: EvdevKey.KEY_T,
  KeyU: EvdevKey.KEY_U,
  KeyV: EvdevKey.KEY_V,
  KeyW: EvdevKey.KEY_W,
  KeyX: EvdevKey.KEY_X,
  KeyY: EvdevKey.KEY_Y,
  KeyZ: EvdevKey.KEY_Z,

  // Punctuation
  Minus: EvdevKey.KEY_MINUS,
  Equal: EvdevKey.KEY_EQUAL,
  BracketLeft: EvdevKey.KEY_LEFTBRACE,
  BracketRight: EvdevKey.KEY_RIGHTBRACE,
  Semicolon: EvdevKey.KEY_SEMICOLON,
  Quote: EvdevKey.KEY_APOSTROPHE,
  Backquote: EvdevKey.KEY_GRAVE,
  Backslash: EvdevKey.KEY_BACKSLASH,
  Comma: EvdevKey.KEY_COMMA,
  Period: EvdevKey.KEY_DOT,
  Slash: EvdevKey.KEY_SLASH,
  IntlBackslash: EvdevKey.KEY_102ND,

  // Function keys
  F1: EvdevKey.KEY_F1,
  F2: EvdevKey.KEY_F2,
  F3: EvdevKey.KEY_F3,
  F4: EvdevKey.KEY_F4,
  F5: EvdevKey.KEY_F5,
  F6: EvdevKey.KEY_F6,
  F7: EvdevKey.KEY_F7,
  F8: EvdevKey.KEY_F8,
  F9: EvdevKey.KEY_F9,
  F10: EvdevKey.KEY_F10,
  F11: EvdevKey.KEY_F11,
  F12: EvdevKey.KEY_F12,
  F13: EvdevKey.KEY_F13,
  F14: EvdevKey.KEY_F14,
  F15: EvdevKey.KEY_F15,
  F16: EvdevKey.KEY_F16,
  F17: EvdevKey.KEY_F17,
  F18: EvdevKey.KEY_F18,
  F19: EvdevKey.KEY_F19,
  F20: EvdevKey.KEY_F20,
  F21: EvdevKey.KEY_F21,
  F22: EvdevKey.KEY_F22,
  F23: EvdevKey.KEY_F23,
  F24: EvdevKey.KEY_F24,

  // Navigation
  Insert: EvdevKey.KEY_INSERT,
  Delete: EvdevKey.KEY_DELETE,
  Home: EvdevKey.KEY_HOME,
  End: EvdevKey.KEY_END,
  PageUp: EvdevKey.KEY_PAGEUP,
  PageDown: EvdevKey.KEY_PAGEDOWN,
  ArrowUp: EvdevKey.KEY_UP,
  ArrowDown: EvdevKey.KEY_DOWN,
  ArrowLeft: EvdevKey.KEY_LEFT,
  ArrowRight: EvdevKey.KEY_RIGHT,

  // Lock keys
  NumLock: EvdevKey.KEY_NUMLOCK,
  ScrollLock: EvdevKey.KEY_SCROLLLOCK,

  // Numpad
  Numpad0: EvdevKey.KEY_KP0,
  Numpad1: EvdevKey.KEY_KP1,
  Numpad2: EvdevKey.KEY_KP2,
  Numpad3: EvdevKey.KEY_KP3,
  Numpad4: EvdevKey.KEY_KP4,
  Numpad5: EvdevKey.KEY_KP5,
  Numpad6: EvdevKey.KEY_KP6,
  Numpad7: EvdevKey.KEY_KP7,
  Numpad8: EvdevKey.KEY_KP8,
  Numpad9: EvdevKey.KEY_KP9,
  NumpadDecimal: EvdevKey.KEY_KPDOT,
  NumpadAdd: EvdevKey.KEY_KPPLUS,
  NumpadSubtract: EvdevKey.KEY_KPMINUS,
  NumpadMultiply: EvdevKey.KEY_KPASTERISK,
  NumpadDivide: EvdevKey.KEY_KPSLASH,
  NumpadEnter: EvdevKey.KEY_KPENTER,
  NumpadComma: EvdevKey.KEY_KPCOMMA,

  // Media keys
  AudioVolumeMute: EvdevKey.KEY_MUTE,
  AudioVolumeDown: EvdevKey.KEY_VOLUMEDOWN,
  AudioVolumeUp: EvdevKey.KEY_VOLUMEUP,
  VolumeMute: EvdevKey.KEY_MUTE,
  MediaTrackPrevious: EvdevKey.KEY_PREVIOUSSONG,
  MediaTrackNext: EvdevKey.KEY_NEXTSONG,
  MediaPlayPause: EvdevKey.KEY_PLAYPAUSE,
  MediaStop: EvdevKey.KEY_STOPCD,

  // Browser keys
  BrowserHome: EvdevKey.KEY_HOMEPAGE,
  BrowserSearch: EvdevKey.KEY_SEARCH,
  BrowserFavorites: EvdevKey.KEY_BOOKMARKS,
  BrowserRefresh: EvdevKey.KEY_REFRESH,
  BrowserStop: EvdevKey.KEY_STOP,
  BrowserForward: EvdevKey.KEY_FORWARD,
  BrowserBack: EvdevKey.KEY_BACK,

  // App launch keys
  LaunchApp1: EvdevKey.KEY_PROG1,
  LaunchApp2: EvdevKey.KEY_PROG2,
  LaunchMail: EvdevKey.KEY_MAIL,
  MediaSelect: EvdevKey.KEY_MEDIA,

  // System keys
  PrintScreen: EvdevKey.KEY_SYSRQ,
  Sleep: EvdevKey.KEY_SLEEP,
  Help: EvdevKey.KEY_HELP,

  // Keys without evdev mapping
  Unidentified: null,
  NumpadEqual: null,
  IntlYen: null,
  IntlRo: null,
  Convert: null,
  NonConvert: null,
  KanaMode: null,
  Lang1: null,
  Lang2: null,
  Power: null,
  WakeUp: null,
  Undo: null,
  Paste: null,
  Cut: null,
  Copy: null,
  Eject: null,
  Fn: null,
  Again: null,
  Props: null,
  Select: null,
  Open: null,
  Find: null,
}

/**
 * Convert a browser KeyboardEvent to a Linux evdev keycode.
 *
 * @param event - The browser keyboard event
 * @returns The evdev keycode, or null if no mapping exists
 */
export function convertToEvdevKey(event: KeyboardEvent): number | null {
  const key = EVDEV_MAPPINGS[event.code] ?? null
  if (key === null) {
    // Fallback to event.key for some special cases
    return EVDEV_MAPPINGS[event.key] ?? null
  }
  return key
}

/**
 * Modifier key flags for evdev (matching D-Bus RemoteDesktop expectations).
 * These are bit flags that can be ORed together.
 */
export const EvdevModifiers = {
  SHIFT: 1 << 0,
  CTRL: 1 << 1,
  ALT: 1 << 2,
  META: 1 << 3,
} as const

/**
 * Convert browser keyboard modifiers to evdev modifier flags.
 *
 * @param event - The browser keyboard event
 * @returns Bitmask of active modifiers
 */
export function convertToEvdevModifiers(event: KeyboardEvent): number {
  let modifiers = 0
  if (event.shiftKey) modifiers |= EvdevModifiers.SHIFT
  if (event.ctrlKey) modifiers |= EvdevModifiers.CTRL
  if (event.altKey) modifiers |= EvdevModifiers.ALT
  if (event.metaKey) modifiers |= EvdevModifiers.META
  return modifiers
}
