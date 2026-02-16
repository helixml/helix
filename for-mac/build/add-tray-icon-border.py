#!/usr/bin/env python3
"""Add an anti-aliased white border around the tray icon.

Usage: python3 add-tray-icon-border.py [input.png] [output.png]

Defaults to reading/writing tray-icon.png in the same directory.
"""
import sys
from pathlib import Path
from PIL import Image, ImageFilter

script_dir = Path(__file__).parent
input_path = Path(sys.argv[1]) if len(sys.argv) > 1 else script_dir / "tray-icon.png"
output_path = Path(sys.argv[2]) if len(sys.argv) > 2 else input_path

img = Image.open(input_path).convert("RGBA")
_, _, _, a = img.split()

# Binary mask from alpha channel
mask = a.point(lambda p: 255 if p > 10 else 0)

# Gaussian blur creates naturally anti-aliased edges
blurred = mask.filter(ImageFilter.GaussianBlur(radius=2.5))

# Boost so border is opaque near icon but still has smooth falloff
boosted = blurred.point(lambda p: min(255, int(p * 2.0)))

# Only keep border pixels where original is transparent
outline = Image.new("L", img.size, 0)
for y in range(img.height):
    for x in range(img.width):
        if a.getpixel((x, y)) <= 10 and boosted.getpixel((x, y)) > 5:
            outline.putpixel((x, y), boosted.getpixel((x, y)))

# White border layer with smooth alpha
border_layer = Image.merge("RGBA", (
    Image.new("L", img.size, 255),
    Image.new("L", img.size, 255),
    Image.new("L", img.size, 255),
    outline,
))

# Composite: smooth white border behind original (original pixels untouched)
result = Image.alpha_composite(border_layer, img)
result.save(output_path)
print(f"Saved {output_path} with anti-aliased white border")
