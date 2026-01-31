# The Dumbest Smart Hack: Cursor Fingerprinting via Hotspot Positions

**tl;dr**: We couldn't read cursor pixels from GPU memory, so we made every cursor type have a unique hotspot position and identify them by where they click. This is cursed and we're not sorry.

## The Problem

We're building a remote desktop streaming thing. GNOME headless, PipeWire ScreenCast, H.264 to the browser, the whole deal. One small issue: we need to know what cursor the user is seeing so we can render it client-side with proper CSS cursors.

You'd think this would be easy. GNOME has a `Meta.CursorTracker` API. It has a `get_sprite()` method. The sprite has pixels. Job done, right?

Wrong.

In headless mode with GPU compositing, Mutter rents the cursor texture to the GPU. When you call `get_sprite()`, you get back a `CoglTexture` that lives entirely in VRAM. There's no `glReadPixels()` equivalent that works reliably. We tried. We really tried.

Our options were:
1. Write custom GStreamer plugin in Rust to intercept cursor data (we did this, it works, but now we have two problems)
2. Patch Mutter to export cursor data via D-Bus (lol no)
3. Use some undocumented Mutter API that probably doesn't exist
4. Something profoundly stupid

We chose option 4.

## The Hack

Here's the key insight: we control the cursor theme. And cursor themes aren't just images - they have **hotspots**. The hotspot is the pixel coordinate that represents "where the cursor is pointing."

For a normal cursor theme, the hotspot is wherever it makes sense:
- Arrow cursor: hotspot at the tip (0,0)
- I-beam cursor: hotspot in the middle (12,12)
- Crosshair: dead center

But what if... every cursor type had a *different* hotspot?

```
default       (0, 0)
pointer       (6, 0)
text          (0, 12)
ns-resize     (6, 12)
ew-resize     (12, 12)
nwse-resize   (18, 12)
nesw-resize   (24, 12)
```

Now we can call `CursorTracker.get_hot()` and immediately know what cursor type we're looking at. No pixel access needed. No GPU memory reads. Just two integers.

The cursor images themselves? Completely transparent 48x48 PNGs. The "cursor" the user sees is rendered client-side by the browser using the CSS `cursor` property.

## But Wait, There's More (Problems)

We shipped this and it worked great. For about a week.

Then someone tested with 200% display scaling.

Turns out Mutter has this delightful piece of code in `meta-cursor-sprite-xcursor.c`:

```c
hotspot_x = ((int) roundf ((float) xc_image->xhot /
                            sprite_xcursor->theme_scale) *
              sprite_xcursor->theme_scale);
```

At 2x scale, hotspots get **rounded to even numbers**. At 3x scale, **multiples of 3**.

Our carefully chosen hotspots (10, 11, 12, 13...) all collapsed into the same value. Every resize cursor became a text cursor. Every corner became... also a text cursor.

The fix was even dumber: use 48x48 cursors and space all hotspots at multiples of 6. This survives both 2x and 3x scaling.

```
# Available hotspots that survive scaling: 0, 6, 12, 18, 24, 30, 36, 42
# That's 8 values per axis = 64 combinations
# We only need ~21 cursor types
# Ship it
```

## Why Not Just Use pipewiresrc?

We do! When the GPU pipeline works, we have a custom GStreamer element that captures cursor data properly. But we want a fallback that works with stock `pipewiresrc` on systems where our custom stuff doesn't load.

This dumb fingerprinting hack is that fallback. A tiny GNOME Shell extension reads the hotspot, sends `{"cursor_name": "text"}` over a Unix socket, and the streaming server broadcasts it to all connected browsers.

## Lessons Learned

1. GPU memory is for GPUs, not for you
2. If you control both ends, you can encode information in weird places
3. 24x24 cursors seemed fine until display scaling happened
4. Always test at 200% DPI before claiming something works
5. Sometimes the stupidest solution is the one that actually ships

## The Code

- `desktop/ubuntu-config/helix-cursors/generate-cursors.sh` - Generates the transparent cursors with fingerprint hotspots
- `desktop/ubuntu-config/gnome-extension/helix-cursor@helix.ml/extension.js` - GNOME Shell extension that reads hotspots and sends cursor type
- `api/pkg/desktop/cursor_listener.go` - Go server that receives cursor updates

---

*We're [Helix](https://helix.ml). We make cloud desktops that AI agents can actually see and interact with. Yes, we did spend an entire day figuring out cursor hotspot math. No, we don't regret it.*
