//go:build ignore

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// trayIconData is a 22x22 PNG icon for the macOS menu bar.
// White hexagon on transparent background — macOS treats this as a template image.
var trayIconData []byte

func init() {
	trayIconData = generateHexagonPNG()
}

func generateHexagonPNG() []byte {
	const size = 22
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Hexagon parameters: center and radius
	cx, cy := float64(size)/2, float64(size)/2
	r := float64(size)/2 - 2 // 9px radius with 2px padding

	white := color.RGBA{R: 255, G: 255, B: 255, A: 255}

	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			if isInsideHexagon(float64(x)+0.5, float64(y)+0.5, cx, cy, r) {
				img.Set(x, y, white)
			}
		}
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// isInsideHexagon checks if point (px, py) is inside a regular hexagon
// centered at (cx, cy) with circumradius r (pointy-top orientation).
func isInsideHexagon(px, py, cx, cy, r float64) bool {
	dx := px - cx
	dy := py - cy

	// Rotate by 30 degrees for pointy-top → flat-top check
	// For a pointy-top hexagon, the distance function is:
	// A point is inside if for all 3 axis pairs, the projection is within r
	adx := math.Abs(dx)
	ady := math.Abs(dy)

	// Pointy-top hexagon boundary:
	// |y| <= r * sqrt(3)/2
	// |x| + |y|/sqrt(3) <= r
	sqrt3over2 := math.Sqrt(3) / 2
	return ady <= r*sqrt3over2 && adx+ady/math.Sqrt(3) <= r
}
