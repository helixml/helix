-- Helix window positioning rules for Ubuntu GNOME
-- Automatically positions windows in 3 columns like Sway's tiling
--
-- Screen layout at 1920x1080:
--   Column 1 (Left):   Terminal    - x=0,    width=640
--   Column 2 (Middle): Zed Editor  - x=640,  width=640
--   Column 3 (Right):  Firefox     - x=1280, width=640
--
-- Using vanilla Ubuntu with no custom scaling.

-- Screen dimensions (matches GAMESCOPE_WIDTH/HEIGHT defaults)
local SCREEN_WIDTH = 1920
local SCREEN_HEIGHT = 1080
local COLUMN_WIDTH = SCREEN_WIDTH / 3  -- 640 pixels

-- Panel height (GNOME top bar)
local PANEL_HEIGHT = 30
local WINDOW_HEIGHT = SCREEN_HEIGHT - PANEL_HEIGHT

-- Get window class for matching
local win_class = get_window_class()
local win_name = get_window_name()

-- Debug logging (check ~/.local/share/devilspie2/debug.log or /tmp/devilspie2.log)
debug_print("Window opened: class=" .. tostring(win_class) .. ", name=" .. tostring(win_name))

-- Firefox browser -> Right column (column 3)
if win_class == "firefox" or win_class == "Firefox" or win_class == "Navigator" then
    debug_print("Positioning Firefox in right column")
    set_window_position(COLUMN_WIDTH * 2, PANEL_HEIGHT)  -- x=1280
    set_window_size(COLUMN_WIDTH, WINDOW_HEIGHT)

-- Zed editor -> Middle column (column 2)
-- Zed reports class as "dev.zed.Zed-Dev" for dev builds, "dev.zed.Zed" for release
elseif win_class == "dev.zed.Zed-Dev" or win_class == "dev.zed.Zed"
       or win_class == "Zed" or win_class == "zed"
       or string.find(string.lower(tostring(win_class) or ""), "zed") then
    debug_print("Positioning Zed in middle column: " .. tostring(win_class))
    set_window_position(COLUMN_WIDTH, PANEL_HEIGHT)  -- x=640
    set_window_size(COLUMN_WIDTH, WINDOW_HEIGHT)

-- Terminal windows -> Left column (column 1)
-- Match various terminal emulators: ghostty, gnome-terminal, kitty, etc.
elseif win_class == "com.mitchellh.ghostty" or win_class == "ghostty"
       or win_class == "Gnome-terminal" or win_class == "gnome-terminal"
       or win_class == "gnome-terminal-server"
       or win_class == "Xfce4-terminal" or win_class == "xfce4-terminal"
       or win_class == "kitty" or win_class == "Kitty"
       or win_class == "XTerm" or win_class == "xterm"
       or win_class == "Terminator" or win_class == "terminator" then
    debug_print("Positioning terminal in left column")
    set_window_position(0, PANEL_HEIGHT)
    set_window_size(COLUMN_WIDTH, WINDOW_HEIGHT)
end
