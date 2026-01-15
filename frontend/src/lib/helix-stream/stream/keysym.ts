/**
 * X11 keysym conversion for keyboard input.
 *
 * X11 keysyms are a standardized way to represent keyboard symbols independent
 * of physical key layout. Despite the "X11" name, they're used by Wayland
 * compositors via xkbcommon - no X11/Xwayland required.
 *
 * This module converts browser KeyboardEvent.key values to X11 keysyms,
 * enabling proper international keyboard support on devices like iPad where
 * event.code is unavailable but event.key contains the correct character.
 *
 * Keysym encoding:
 * - Latin-1 characters (0x20-0xFF): keysym = Unicode code point
 * - Unicode beyond Latin-1: keysym = code point | 0x01000000
 * - Special keys (arrows, modifiers, etc.): predefined values in 0xFF00-0xFFFF range
 *
 * Reference: https://www.cl.cam.ac.uk/~mgk25/ucs/keysymdef.h
 */

// Special key keysyms (XK_* from keysymdef.h)
export const XKeySym = {
  // TTY function keys
  XK_BackSpace: 0xff08,
  XK_Tab: 0xff09,
  XK_Linefeed: 0xff0a,
  XK_Clear: 0xff0b,
  XK_Return: 0xff0d,
  XK_Pause: 0xff13,
  XK_Scroll_Lock: 0xff14,
  XK_Sys_Req: 0xff15,
  XK_Escape: 0xff1b,
  XK_Delete: 0xffff,

  // Cursor control & motion
  XK_Home: 0xff50,
  XK_Left: 0xff51,
  XK_Up: 0xff52,
  XK_Right: 0xff53,
  XK_Down: 0xff54,
  XK_Page_Up: 0xff55,
  XK_Page_Down: 0xff56,
  XK_End: 0xff57,
  XK_Begin: 0xff58,

  // Misc functions
  XK_Select: 0xff60,
  XK_Print: 0xff61,
  XK_Execute: 0xff62,
  XK_Insert: 0xff63,
  XK_Undo: 0xff65,
  XK_Redo: 0xff66,
  XK_Menu: 0xff67,
  XK_Find: 0xff68,
  XK_Cancel: 0xff69,
  XK_Help: 0xff6a,
  XK_Break: 0xff6b,
  XK_Num_Lock: 0xff7f,

  // Keypad
  XK_KP_Space: 0xff80,
  XK_KP_Tab: 0xff89,
  XK_KP_Enter: 0xff8d,
  XK_KP_F1: 0xff91,
  XK_KP_F2: 0xff92,
  XK_KP_F3: 0xff93,
  XK_KP_F4: 0xff94,
  XK_KP_Home: 0xff95,
  XK_KP_Left: 0xff96,
  XK_KP_Up: 0xff97,
  XK_KP_Right: 0xff98,
  XK_KP_Down: 0xff99,
  XK_KP_Page_Up: 0xff9a,
  XK_KP_Page_Down: 0xff9b,
  XK_KP_End: 0xff9c,
  XK_KP_Begin: 0xff9d,
  XK_KP_Insert: 0xff9e,
  XK_KP_Delete: 0xff9f,
  XK_KP_Equal: 0xffbd,
  XK_KP_Multiply: 0xffaa,
  XK_KP_Add: 0xffab,
  XK_KP_Separator: 0xffac,
  XK_KP_Subtract: 0xffad,
  XK_KP_Decimal: 0xffae,
  XK_KP_Divide: 0xffaf,
  XK_KP_0: 0xffb0,
  XK_KP_1: 0xffb1,
  XK_KP_2: 0xffb2,
  XK_KP_3: 0xffb3,
  XK_KP_4: 0xffb4,
  XK_KP_5: 0xffb5,
  XK_KP_6: 0xffb6,
  XK_KP_7: 0xffb7,
  XK_KP_8: 0xffb8,
  XK_KP_9: 0xffb9,

  // Function keys
  XK_F1: 0xffbe,
  XK_F2: 0xffbf,
  XK_F3: 0xffc0,
  XK_F4: 0xffc1,
  XK_F5: 0xffc2,
  XK_F6: 0xffc3,
  XK_F7: 0xffc4,
  XK_F8: 0xffc5,
  XK_F9: 0xffc6,
  XK_F10: 0xffc7,
  XK_F11: 0xffc8,
  XK_F12: 0xffc9,
  XK_F13: 0xffca,
  XK_F14: 0xffcb,
  XK_F15: 0xffcc,
  XK_F16: 0xffcd,
  XK_F17: 0xffce,
  XK_F18: 0xffcf,
  XK_F19: 0xffd0,
  XK_F20: 0xffd1,
  XK_F21: 0xffd2,
  XK_F22: 0xffd3,
  XK_F23: 0xffd4,
  XK_F24: 0xffd5,

  // Modifier keys
  XK_Shift_L: 0xffe1,
  XK_Shift_R: 0xffe2,
  XK_Control_L: 0xffe3,
  XK_Control_R: 0xffe4,
  XK_Caps_Lock: 0xffe5,
  XK_Shift_Lock: 0xffe6,
  XK_Meta_L: 0xffe7,
  XK_Meta_R: 0xffe8,
  XK_Alt_L: 0xffe9,
  XK_Alt_R: 0xffea,
  XK_Super_L: 0xffeb,
  XK_Super_R: 0xffec,
  XK_Hyper_L: 0xffed,
  XK_Hyper_R: 0xffee,
} as const

/**
 * Maps browser KeyboardEvent.key values to X11 keysyms.
 * Only contains special keys; regular characters use Unicode conversion.
 */
const KEY_TO_KEYSYM: Record<string, number> = {
  // Control keys
  Backspace: XKeySym.XK_BackSpace,
  Tab: XKeySym.XK_Tab,
  Enter: XKeySym.XK_Return,
  Escape: XKeySym.XK_Escape,
  Delete: XKeySym.XK_Delete,
  ' ': 0x0020, // Space is Latin-1

  // Navigation
  Home: XKeySym.XK_Home,
  End: XKeySym.XK_End,
  PageUp: XKeySym.XK_Page_Up,
  PageDown: XKeySym.XK_Page_Down,
  ArrowLeft: XKeySym.XK_Left,
  ArrowUp: XKeySym.XK_Up,
  ArrowRight: XKeySym.XK_Right,
  ArrowDown: XKeySym.XK_Down,
  Insert: XKeySym.XK_Insert,

  // Modifier keys
  Shift: XKeySym.XK_Shift_L,
  Control: XKeySym.XK_Control_L,
  Alt: XKeySym.XK_Alt_L,
  Meta: XKeySym.XK_Super_L, // Meta/Command → Super
  CapsLock: XKeySym.XK_Caps_Lock,
  NumLock: XKeySym.XK_Num_Lock,
  ScrollLock: XKeySym.XK_Scroll_Lock,

  // Function keys
  F1: XKeySym.XK_F1,
  F2: XKeySym.XK_F2,
  F3: XKeySym.XK_F3,
  F4: XKeySym.XK_F4,
  F5: XKeySym.XK_F5,
  F6: XKeySym.XK_F6,
  F7: XKeySym.XK_F7,
  F8: XKeySym.XK_F8,
  F9: XKeySym.XK_F9,
  F10: XKeySym.XK_F10,
  F11: XKeySym.XK_F11,
  F12: XKeySym.XK_F12,
  F13: XKeySym.XK_F13,
  F14: XKeySym.XK_F14,
  F15: XKeySym.XK_F15,
  F16: XKeySym.XK_F16,
  F17: XKeySym.XK_F17,
  F18: XKeySym.XK_F18,
  F19: XKeySym.XK_F19,
  F20: XKeySym.XK_F20,

  // Misc
  PrintScreen: XKeySym.XK_Print,
  Pause: XKeySym.XK_Pause,
  ContextMenu: XKeySym.XK_Menu,
  Help: XKeySym.XK_Help,
}

/**
 * Convert a browser KeyboardEvent.key value to an X11 keysym.
 *
 * For single characters (letters, numbers, punctuation, accented characters),
 * uses Unicode code point mapping:
 * - Latin-1 (U+0020 to U+00FF): keysym = code point
 * - Beyond Latin-1: keysym = code point | 0x01000000
 *
 * For special keys (arrows, modifiers, function keys), uses predefined keysyms.
 *
 * @param key - The KeyboardEvent.key value
 * @returns The X11 keysym, or null if no mapping exists
 */
export function keyToKeysym(key: string): number | null {
  // Check special key mapping first
  const specialKeysym = KEY_TO_KEYSYM[key]
  if (specialKeysym !== undefined) {
    return specialKeysym
  }

  // For single character keys, convert using Unicode
  if (key.length === 1) {
    const codePoint = key.codePointAt(0)
    if (codePoint === undefined) {
      return null
    }

    // Latin-1 range: keysym = Unicode code point
    if (codePoint >= 0x0020 && codePoint <= 0x00ff) {
      return codePoint
    }

    // Unicode beyond Latin-1: keysym = code point | 0x01000000
    if (codePoint >= 0x0100) {
      return codePoint | 0x01000000
    }
  }

  // Handle keys with longer values (e.g., "Dead" for dead keys)
  // Dead keys are composing keys that don't produce output on their own
  if (key.startsWith('Dead')) {
    // We can't handle dead keys via keysym - they require compositor support
    return null
  }

  // Unidentified or other special keys we don't handle
  return null
}

/**
 * Check if a keyboard event should use keysym mode.
 *
 * We use keysym mode when:
 * 1. event.code is empty (iOS virtual/physical keyboards)
 * 2. event.key contains a usable value (not "Unidentified")
 *
 * @param event - The browser keyboard event
 * @returns true if keysym mode should be used
 */
export function shouldUseKeysym(event: KeyboardEvent): boolean {
  // If we have a valid event.code, prefer evdev keycode mode
  if (event.code && event.code !== '' && event.code !== 'Unidentified') {
    return false
  }

  // If event.key is empty or unidentified, we can't use keysym either
  if (!event.key || event.key === '' || event.key === 'Unidentified') {
    return false
  }

  // Use keysym mode - we have event.key but no event.code
  return true
}

/**
 * Convert a KeyboardEvent to a keysym if applicable.
 *
 * Returns null if the event should use evdev keycode mode instead,
 * or if no keysym mapping exists.
 *
 * @param event - The browser keyboard event
 * @returns The X11 keysym, or null if keysym mode shouldn't be used
 */
export function convertToKeysym(event: KeyboardEvent): number | null {
  if (!shouldUseKeysym(event)) {
    return null
  }
  return keyToKeysym(event.key)
}
