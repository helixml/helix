# Requirements: Make Ghostty Respect System Light/Dark Mode

## Background

Ghostty is the terminal emulator used by the Helix Desktop setup
(`~/.config/ghostty/config`). The current configuration does not set a `theme`
at all, so Ghostty falls back to its built-in default (a dark theme) regardless
of the system color scheme. When the user toggles their desktop into light
mode, the terminal stays dark and stands out as the only un-themed surface.

The user wants Ghostty to follow the system color scheme automatically and to
ship with a tasteful light theme out of the box (the dark theme should remain
pleasant too).

## User Story

> As a Helix Desktop user, when I switch my system between light and dark mode,
> I want Ghostty to switch its colors to match — without having to edit any
> config or restart the terminal — so that my whole desktop has a consistent
> look.

## Acceptance Criteria

1. Opening Ghostty while the system is in **light mode** shows a clearly
   light-themed terminal (light background, dark text) using an attractive,
   readable light theme.
2. Opening Ghostty while the system is in **dark mode** shows a clearly
   dark-themed terminal using a matching dark theme from the same theme
   family.
3. Changing the system color scheme **while Ghostty is running** updates the
   colors of existing windows live, without restarting.
4. The window chrome (titlebar / decoration) follows the same setting and does
   not look out of place against the terminal contents.
5. All existing config behavior is preserved: translucent background, font,
   padding, scrollback, keybindings, `gtk-single-instance = false`, etc.
6. The chosen light/dark theme pair has been visually reviewed and approved by
   the user during interactive testing.

## Out of Scope

- Adding a hot-key to toggle themes manually inside Ghostty.
- Shipping a custom theme file. We will use Ghostty's built-in themes.
- Changing the theming of any other application in Helix Desktop.
