package desktop

// VK → evdev keycode mapping
// VK codes are Windows Virtual Key codes used by the streaming frontend
// evdev codes are Linux input event codes

// vkToEvdev maps Windows VK codes to Linux evdev key codes
var vkToEvdev = map[uint16]int{
	// Control keys
	0x08: 14,  // VK_BACK -> KEY_BACKSPACE
	0x09: 15,  // VK_TAB -> KEY_TAB
	0x0C: 102, // VK_CLEAR -> KEY_CLEAR (not standard, using KEY_HOME)
	0x0D: 28,  // VK_RETURN -> KEY_ENTER
	0x10: 42,  // VK_SHIFT -> KEY_LEFTSHIFT
	0x11: 29,  // VK_CONTROL -> KEY_LEFTCTRL
	0x12: 56,  // VK_MENU (Alt) -> KEY_LEFTALT
	0x13: 119, // VK_PAUSE -> KEY_PAUSE
	0x14: 58,  // VK_CAPITAL -> KEY_CAPSLOCK
	0x1B: 1,   // VK_ESCAPE -> KEY_ESC

	// Navigation
	0x20: 57,  // VK_SPACE -> KEY_SPACE
	0x21: 104, // VK_PRIOR -> KEY_PAGEUP
	0x22: 109, // VK_NEXT -> KEY_PAGEDOWN
	0x23: 107, // VK_END -> KEY_END
	0x24: 102, // VK_HOME -> KEY_HOME
	0x25: 105, // VK_LEFT -> KEY_LEFT
	0x26: 103, // VK_UP -> KEY_UP
	0x27: 106, // VK_RIGHT -> KEY_RIGHT
	0x28: 108, // VK_DOWN -> KEY_DOWN
	0x29: 0,   // VK_SELECT (no direct mapping)
	0x2A: 0,   // VK_PRINT (no direct mapping)
	0x2C: 99,  // VK_SNAPSHOT -> KEY_SYSRQ
	0x2D: 110, // VK_INSERT -> KEY_INSERT
	0x2E: 111, // VK_DELETE -> KEY_DELETE
	0x2F: 138, // VK_HELP -> KEY_HELP

	// Numbers (top row)
	0x30: 11, // VK_KEY_0 -> KEY_0
	0x31: 2,  // VK_KEY_1 -> KEY_1
	0x32: 3,  // VK_KEY_2 -> KEY_2
	0x33: 4,  // VK_KEY_3 -> KEY_3
	0x34: 5,  // VK_KEY_4 -> KEY_4
	0x35: 6,  // VK_KEY_5 -> KEY_5
	0x36: 7,  // VK_KEY_6 -> KEY_6
	0x37: 8,  // VK_KEY_7 -> KEY_7
	0x38: 9,  // VK_KEY_8 -> KEY_8
	0x39: 10, // VK_KEY_9 -> KEY_9

	// Letters
	0x41: 30, // VK_KEY_A -> KEY_A
	0x42: 48, // VK_KEY_B -> KEY_B
	0x43: 46, // VK_KEY_C -> KEY_C
	0x44: 32, // VK_KEY_D -> KEY_D
	0x45: 18, // VK_KEY_E -> KEY_E
	0x46: 33, // VK_KEY_F -> KEY_F
	0x47: 34, // VK_KEY_G -> KEY_G
	0x48: 35, // VK_KEY_H -> KEY_H
	0x49: 23, // VK_KEY_I -> KEY_I
	0x4A: 36, // VK_KEY_J -> KEY_J
	0x4B: 37, // VK_KEY_K -> KEY_K
	0x4C: 38, // VK_KEY_L -> KEY_L
	0x4D: 50, // VK_KEY_M -> KEY_M
	0x4E: 49, // VK_KEY_N -> KEY_N
	0x4F: 24, // VK_KEY_O -> KEY_O
	0x50: 25, // VK_KEY_P -> KEY_P
	0x51: 16, // VK_KEY_Q -> KEY_Q
	0x52: 19, // VK_KEY_R -> KEY_R
	0x53: 31, // VK_KEY_S -> KEY_S
	0x54: 20, // VK_KEY_T -> KEY_T
	0x55: 22, // VK_KEY_U -> KEY_U
	0x56: 47, // VK_KEY_V -> KEY_V
	0x57: 17, // VK_KEY_W -> KEY_W
	0x58: 45, // VK_KEY_X -> KEY_X
	0x59: 21, // VK_KEY_Y -> KEY_Y
	0x5A: 44, // VK_KEY_Z -> KEY_Z

	// Windows/Meta keys
	0x5B: 125, // VK_LWIN -> KEY_LEFTMETA
	0x5C: 126, // VK_RWIN -> KEY_RIGHTMETA
	0x5D: 127, // VK_APPS -> KEY_COMPOSE
	0x5F: 142, // VK_SLEEP -> KEY_SLEEP

	// Numpad
	0x60: 82,  // VK_NUMPAD0 -> KEY_KP0
	0x61: 79,  // VK_NUMPAD1 -> KEY_KP1
	0x62: 80,  // VK_NUMPAD2 -> KEY_KP2
	0x63: 81,  // VK_NUMPAD3 -> KEY_KP3
	0x64: 75,  // VK_NUMPAD4 -> KEY_KP4
	0x65: 76,  // VK_NUMPAD5 -> KEY_KP5
	0x66: 77,  // VK_NUMPAD6 -> KEY_KP6
	0x67: 71,  // VK_NUMPAD7 -> KEY_KP7
	0x68: 72,  // VK_NUMPAD8 -> KEY_KP8
	0x69: 73,  // VK_NUMPAD9 -> KEY_KP9
	0x6A: 55,  // VK_MULTIPLY -> KEY_KPASTERISK
	0x6B: 78,  // VK_ADD -> KEY_KPPLUS
	0x6C: 121, // VK_SEPARATOR -> KEY_KPCOMMA
	0x6D: 74,  // VK_SUBTRACT -> KEY_KPMINUS
	0x6E: 83,  // VK_DECIMAL -> KEY_KPDOT
	0x6F: 98,  // VK_DIVIDE -> KEY_KPSLASH

	// Function keys
	0x70: 59,  // VK_F1 -> KEY_F1
	0x71: 60,  // VK_F2 -> KEY_F2
	0x72: 61,  // VK_F3 -> KEY_F3
	0x73: 62,  // VK_F4 -> KEY_F4
	0x74: 63,  // VK_F5 -> KEY_F5
	0x75: 64,  // VK_F6 -> KEY_F6
	0x76: 65,  // VK_F7 -> KEY_F7
	0x77: 66,  // VK_F8 -> KEY_F8
	0x78: 67,  // VK_F9 -> KEY_F9
	0x79: 68,  // VK_F10 -> KEY_F10
	0x7A: 87,  // VK_F11 -> KEY_F11
	0x7B: 88,  // VK_F12 -> KEY_F12
	0x7C: 183, // VK_F13 -> KEY_F13
	0x7D: 184, // VK_F14 -> KEY_F14
	0x7E: 185, // VK_F15 -> KEY_F15
	0x7F: 186, // VK_F16 -> KEY_F16
	0x80: 187, // VK_F17 -> KEY_F17
	0x81: 188, // VK_F18 -> KEY_F18
	0x82: 189, // VK_F19 -> KEY_F19
	0x83: 190, // VK_F20 -> KEY_F20
	0x84: 191, // VK_F21 -> KEY_F21
	0x85: 192, // VK_F22 -> KEY_F22
	0x86: 193, // VK_F23 -> KEY_F23
	0x87: 194, // VK_F24 -> KEY_F24

	// Lock keys
	0x90: 69, // VK_NUMLOCK -> KEY_NUMLOCK
	0x91: 70, // VK_SCROLL -> KEY_SCROLLLOCK

	// Left/Right modifiers
	0xA0: 42,  // VK_LSHIFT -> KEY_LEFTSHIFT
	0xA1: 54,  // VK_RSHIFT -> KEY_RIGHTSHIFT
	0xA2: 29,  // VK_LCONTROL -> KEY_LEFTCTRL
	0xA3: 97,  // VK_RCONTROL -> KEY_RIGHTCTRL
	0xA4: 56,  // VK_LMENU -> KEY_LEFTALT
	0xA5: 100, // VK_RMENU -> KEY_RIGHTALT

	// Browser keys
	0xA6: 158, // VK_BROWSER_BACK -> KEY_BACK
	0xA7: 159, // VK_BROWSER_FORWARD -> KEY_FORWARD
	0xA8: 173, // VK_BROWSER_REFRESH -> KEY_REFRESH
	0xA9: 128, // VK_BROWSER_STOP -> KEY_STOP
	0xAA: 217, // VK_BROWSER_SEARCH -> KEY_SEARCH
	0xAB: 156, // VK_BROWSER_FAVORITES -> KEY_BOOKMARKS
	0xAC: 172, // VK_BROWSER_HOME -> KEY_HOMEPAGE

	// Volume/Media
	0xAD: 113, // VK_VOLUME_MUTE -> KEY_MUTE
	0xAE: 114, // VK_VOLUME_DOWN -> KEY_VOLUMEDOWN
	0xAF: 115, // VK_VOLUME_UP -> KEY_VOLUMEUP
	0xB0: 163, // VK_MEDIA_NEXT_TRACK -> KEY_NEXTSONG
	0xB1: 165, // VK_MEDIA_PREV_TRACK -> KEY_PREVIOUSSONG
	0xB2: 166, // VK_MEDIA_STOP -> KEY_STOPCD
	0xB3: 164, // VK_MEDIA_PLAY_PAUSE -> KEY_PLAYPAUSE
	0xB4: 155, // VK_LAUNCH_MAIL -> KEY_MAIL
	0xB5: 226, // VK_LAUNCH_MEDIA_SELECT -> KEY_MEDIA
	0xB6: 148, // VK_LAUNCH_APP1 -> KEY_PROG1
	0xB7: 149, // VK_LAUNCH_APP2 -> KEY_PROG2

	// OEM keys (punctuation)
	0xBA: 39,  // VK_OEM_1 (;:) -> KEY_SEMICOLON
	0xBB: 13,  // VK_OEM_PLUS (=+) -> KEY_EQUAL
	0xBC: 51,  // VK_OEM_COMMA (,<) -> KEY_COMMA
	0xBD: 12,  // VK_OEM_MINUS (-_) -> KEY_MINUS
	0xBE: 52,  // VK_OEM_PERIOD (.>) -> KEY_DOT
	0xBF: 53,  // VK_OEM_2 (/?) -> KEY_SLASH
	0xC0: 41,  // VK_OEM_3 (`~) -> KEY_GRAVE
	0xDB: 26,  // VK_OEM_4 ([{) -> KEY_LEFTBRACE
	0xDC: 43,  // VK_OEM_5 (\|) -> KEY_BACKSLASH
	0xDD: 27,  // VK_OEM_6 (]}) -> KEY_RIGHTBRACE
	0xDE: 40,  // VK_OEM_7 ('") -> KEY_APOSTROPHE
	0xE2: 86,  // VK_OEM_102 (<>) -> KEY_102ND
}

// VKToEvdev converts a Windows VK code to a Linux evdev keycode.
// Returns 0 if no mapping exists.
func VKToEvdev(vk uint16) int {
	if evdev, ok := vkToEvdev[vk]; ok {
		return evdev
	}
	return 0
}
