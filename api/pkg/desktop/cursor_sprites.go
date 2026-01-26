// Package desktop provides cursor sprite compositing for screenshots.
// When using transparent cursor themes (like Helix-Invisible), we need to
// composite cursors onto screenshots so AI agents can see cursor position.
package desktop

import (
	"bytes"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"image/png"
	"sync"
)

// CursorSprite represents a cursor image for compositing.
type CursorSprite struct {
	Image    image.Image
	HotspotX int
	HotspotY int
}

var (
	cursorSprites     map[string]*CursorSprite
	cursorSpritesOnce sync.Once
)

// GetCursorSprite returns the sprite for a given cursor name.
// If the cursor name is not found, returns the default arrow cursor.
func GetCursorSprite(cursorName string) *CursorSprite {
	cursorSpritesOnce.Do(initCursorSprites)

	if sprite, ok := cursorSprites[cursorName]; ok {
		return sprite
	}
	return cursorSprites["default"]
}

// initCursorSprites initializes all cursor sprites.
// These are simple drawn cursor shapes that match the frontend SVG cursors.
func initCursorSprites() {
	cursorSprites = make(map[string]*CursorSprite)

	// Default arrow cursor (dark body, white outline)
	cursorSprites["default"] = createArrowCursor()
	cursorSprites["arrow"] = cursorSprites["default"]

	// Pointer cursor (same as default for now)
	cursorSprites["pointer"] = cursorSprites["default"]
	cursorSprites["hand"] = cursorSprites["pointer"]

	// Text cursor (I-beam)
	cursorSprites["text"] = createTextCursor()
	cursorSprites["ibeam"] = cursorSprites["text"]

	// Crosshair
	cursorSprites["crosshair"] = createCrosshairCursor()
	cursorSprites["cross"] = cursorSprites["crosshair"]

	// Move cursor (4-way arrows)
	cursorSprites["move"] = createMoveCursor()
	cursorSprites["all-scroll"] = cursorSprites["move"]

	// Wait cursor (hourglass/spinner)
	cursorSprites["wait"] = createWaitCursor()
	cursorSprites["busy"] = cursorSprites["wait"]
	cursorSprites["progress"] = cursorSprites["wait"]

	// Not-allowed cursor
	cursorSprites["not-allowed"] = createNotAllowedCursor()
	cursorSprites["no-drop"] = cursorSprites["not-allowed"]

	// Resize cursors
	cursorSprites["ns-resize"] = createNSResizeCursor()
	cursorSprites["row-resize"] = cursorSprites["ns-resize"]
	cursorSprites["ew-resize"] = createEWResizeCursor()
	cursorSprites["col-resize"] = cursorSprites["ew-resize"]
	cursorSprites["nwse-resize"] = createNWSEResizeCursor()
	cursorSprites["nesw-resize"] = createNESWResizeCursor()

	// Zoom cursors
	cursorSprites["zoom-in"] = createZoomInCursor()
	cursorSprites["zoom-out"] = createZoomOutCursor()

	// Grab cursors
	cursorSprites["grab"] = createGrabCursor()
	cursorSprites["openhand"] = cursorSprites["grab"]
	cursorSprites["grabbing"] = createGrabbingCursor()
	cursorSprites["closedhand"] = cursorSprites["grabbing"]

	// Help cursor
	cursorSprites["help"] = createHelpCursor()

	// Context menu
	cursorSprites["context-menu"] = createContextMenuCursor()

	// Copy/alias cursors
	cursorSprites["copy"] = createCopyCursor()
	cursorSprites["alias"] = createAliasCursor()

	// Cell cursor
	cursorSprites["cell"] = createCellCursor()
}

// Helper colors - Standard Adwaita cursor style (white with black outline)
// This matches the default GNOME cursor theme which is most common in training data.
var (
	cursorBody    = color.RGBA{255, 255, 255, 255} // white body
	cursorOutline = color.RGBA{0, 0, 0, 255}       // black outline
	transparent   = color.RGBA{0, 0, 0, 0}
)

// createArrowCursor creates the default arrow cursor.
func createArrowCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Draw arrow shape - matches the frontend SVG path
	// Path: M1 1L1 18L5 14L8 20L11 18L8 12L14 12L1 1Z
	// Simple filled polygon for the arrow

	// Outline first (white)
	drawLine(img, 1, 1, 1, 18, cursorOutline)
	drawLine(img, 1, 18, 5, 14, cursorOutline)
	drawLine(img, 5, 14, 8, 20, cursorOutline)
	drawLine(img, 8, 20, 11, 18, cursorOutline)
	drawLine(img, 11, 18, 8, 12, cursorOutline)
	drawLine(img, 8, 12, 14, 12, cursorOutline)
	drawLine(img, 14, 12, 1, 1, cursorOutline)

	// Fill interior with body color
	fillArrowPolygon(img, cursorBody)

	return &CursorSprite{
		Image:    img,
		HotspotX: 0,
		HotspotY: 0,
	}
}

// fillArrowPolygon fills the arrow shape with the given color.
func fillArrowPolygon(img *image.RGBA, c color.Color) {
	// Simple scanline fill for the arrow shape
	// The arrow extends from (1,1) to about (14,20)
	// We'll use a simple approach: for each row, find the left and right bounds

	// Arrow polygon points (from SVG path):
	// (1,1), (1,18), (5,14), (8,20), (11,18), (8,12), (14,12)
	for y := 2; y < 19; y++ {
		var minX, maxX int
		if y <= 12 {
			// Upper part of arrow
			minX = 2
			maxX = min(y, 13)
		} else if y <= 14 {
			// Middle part
			minX = 2
			maxX = 7
		} else {
			// Lower part (the tail)
			minX = y - 14 + 5
			maxX = 10
		}
		for x := minX; x < maxX; x++ {
			img.Set(x, y, c)
		}
	}
}

// createTextCursor creates an I-beam text cursor.
func createTextCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Vertical line with serifs at top and bottom
	cx := size / 2

	// Draw with body color first (thicker)
	for y := 3; y < 21; y++ {
		img.Set(cx-1, y, cursorBody)
		img.Set(cx, y, cursorBody)
		img.Set(cx+1, y, cursorBody)
	}
	// Top serif
	for x := cx - 3; x <= cx+3; x++ {
		img.Set(x, 3, cursorBody)
		img.Set(x, 4, cursorBody)
	}
	// Bottom serif
	for x := cx - 3; x <= cx+3; x++ {
		img.Set(x, 20, cursorBody)
		img.Set(x, 21, cursorBody)
	}

	// White outline
	for y := 3; y < 21; y++ {
		img.Set(cx, y, cursorOutline)
	}
	for x := cx - 3; x <= cx+3; x++ {
		img.Set(x, 3, cursorOutline)
		img.Set(x, 21, cursorOutline)
	}

	return &CursorSprite{
		Image:    img,
		HotspotX: size / 2,
		HotspotY: size / 2,
	}
}

// createCrosshairCursor creates a crosshair cursor.
func createCrosshairCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := size/2, size/2

	// Draw crosshair lines
	// Vertical line
	for y := 2; y < size-2; y++ {
		if y < cy-2 || y > cy+2 { // Gap in middle
			img.Set(cx, y, cursorOutline)
		}
	}
	// Horizontal line
	for x := 2; x < size-2; x++ {
		if x < cx-2 || x > cx+2 { // Gap in middle
			img.Set(x, cy, cursorOutline)
		}
	}
	// Center dot
	img.Set(cx, cy, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: cx,
		HotspotY: cy,
	}
}

// createMoveCursor creates a 4-way move cursor.
func createMoveCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := size/2, size/2

	// Draw 4 arrows pointing outward
	// Up arrow
	drawTriangle(img, cx, 3, cx-3, 8, cx+3, 8, cursorOutline)
	// Down arrow
	drawTriangle(img, cx, 21, cx-3, 16, cx+3, 16, cursorOutline)
	// Left arrow
	drawTriangle(img, 3, cy, 8, cy-3, 8, cy+3, cursorOutline)
	// Right arrow
	drawTriangle(img, 21, cy, 16, cy-3, 16, cy+3, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: cx,
		HotspotY: cy,
	}
}

// createWaitCursor creates a wait/busy cursor (hourglass).
func createWaitCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Simple hourglass shape
	// Top triangle
	for y := 4; y < 12; y++ {
		halfWidth := 10 - (y - 4)
		for x := 12 - halfWidth; x <= 12+halfWidth; x++ {
			img.Set(x, y, cursorBody)
		}
	}
	// Bottom triangle
	for y := 12; y < 20; y++ {
		halfWidth := y - 12
		for x := 12 - halfWidth; x <= 12+halfWidth; x++ {
			img.Set(x, y, cursorBody)
		}
	}
	// Top and bottom lines
	for x := 7; x < 17; x++ {
		img.Set(x, 4, cursorOutline)
		img.Set(x, 20, cursorOutline)
	}

	return &CursorSprite{
		Image:    img,
		HotspotX: 12,
		HotspotY: 12,
	}
}

// createNotAllowedCursor creates a not-allowed/prohibited cursor.
func createNotAllowedCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := size/2, size/2

	// Draw circle
	for angle := 0; angle < 360; angle++ {
		rad := float64(angle) * 3.14159 / 180
		x := cx + int(8*cos(rad))
		y := cy + int(8*sin(rad))
		img.Set(x, y, cursorOutline)
	}
	// Draw diagonal line
	drawLine(img, 6, 18, 18, 6, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: cx,
		HotspotY: cy,
	}
}

// createNSResizeCursor creates a north-south resize cursor.
func createNSResizeCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx := size / 2

	// Up arrow
	drawTriangle(img, cx, 3, cx-4, 9, cx+4, 9, cursorOutline)
	// Down arrow
	drawTriangle(img, cx, 21, cx-4, 15, cx+4, 15, cursorOutline)
	// Connecting line
	drawLine(img, cx, 9, cx, 15, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: cx,
		HotspotY: 12,
	}
}

// createEWResizeCursor creates an east-west resize cursor.
func createEWResizeCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cy := size / 2

	// Left arrow
	drawTriangle(img, 3, cy, 9, cy-4, 9, cy+4, cursorOutline)
	// Right arrow
	drawTriangle(img, 21, cy, 15, cy-4, 15, cy+4, cursorOutline)
	// Connecting line
	drawLine(img, 9, cy, 15, cy, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: 12,
		HotspotY: cy,
	}
}

// createNWSEResizeCursor creates a northwest-southeast resize cursor.
func createNWSEResizeCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// NW arrow head
	for i := 0; i < 6; i++ {
		img.Set(5+i, 5, cursorOutline)
		img.Set(5, 5+i, cursorOutline)
	}
	// SE arrow head
	for i := 0; i < 6; i++ {
		img.Set(19-i, 19, cursorOutline)
		img.Set(19, 19-i, cursorOutline)
	}
	// Connecting line
	drawLine(img, 7, 7, 17, 17, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: 12,
		HotspotY: 12,
	}
}

// createNESWResizeCursor creates a northeast-southwest resize cursor.
func createNESWResizeCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// NE arrow head
	for i := 0; i < 6; i++ {
		img.Set(19-i, 5, cursorOutline)
		img.Set(19, 5+i, cursorOutline)
	}
	// SW arrow head
	for i := 0; i < 6; i++ {
		img.Set(5+i, 19, cursorOutline)
		img.Set(5, 19-i, cursorOutline)
	}
	// Connecting line
	drawLine(img, 17, 7, 7, 17, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: 12,
		HotspotY: 12,
	}
}

// createZoomInCursor creates a zoom-in cursor (magnifier with plus).
func createZoomInCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Draw magnifier circle
	cx, cy := 10, 10
	for angle := 0; angle < 360; angle++ {
		rad := float64(angle) * 3.14159 / 180
		x := cx + int(6*cos(rad))
		y := cy + int(6*sin(rad))
		img.Set(x, y, cursorOutline)
	}
	// Handle
	drawLine(img, 15, 15, 21, 21, cursorOutline)
	// Plus sign
	drawLine(img, cx-3, cy, cx+3, cy, cursorOutline)
	drawLine(img, cx, cy-3, cx, cy+3, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: cx,
		HotspotY: cy,
	}
}

// createZoomOutCursor creates a zoom-out cursor (magnifier with minus).
func createZoomOutCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Draw magnifier circle
	cx, cy := 10, 10
	for angle := 0; angle < 360; angle++ {
		rad := float64(angle) * 3.14159 / 180
		x := cx + int(6*cos(rad))
		y := cy + int(6*sin(rad))
		img.Set(x, y, cursorOutline)
	}
	// Handle
	drawLine(img, 15, 15, 21, 21, cursorOutline)
	// Minus sign
	drawLine(img, cx-3, cy, cx+3, cy, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: cx,
		HotspotY: cy,
	}
}

// createGrabCursor creates an open hand cursor.
func createGrabCursor() *CursorSprite {
	return createArrowCursor() // Fallback to arrow for now
}

// createGrabbingCursor creates a closed hand cursor.
func createGrabbingCursor() *CursorSprite {
	return createArrowCursor() // Fallback to arrow for now
}

// createHelpCursor creates a help cursor (arrow with question mark).
func createHelpCursor() *CursorSprite {
	img := createArrowCursor().Image.(*image.RGBA)

	// Add question mark badge at bottom right
	for y := 14; y < 22; y++ {
		for x := 14; x < 22; x++ {
			img.Set(x, y, cursorBody)
		}
	}
	// Question mark
	img.Set(17, 16, cursorOutline)
	img.Set(18, 16, cursorOutline)
	img.Set(19, 17, cursorOutline)
	img.Set(18, 18, cursorOutline)
	img.Set(18, 20, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: 0,
		HotspotY: 0,
	}
}

// createContextMenuCursor creates a context menu cursor.
func createContextMenuCursor() *CursorSprite {
	return createArrowCursor() // Fallback to arrow
}

// createCopyCursor creates a copy cursor (arrow with plus).
func createCopyCursor() *CursorSprite {
	img := createArrowCursor().Image.(*image.RGBA)

	// Add plus badge
	for x := 15; x < 21; x++ {
		img.Set(x, 17, cursorOutline)
	}
	for y := 14; y < 21; y++ {
		img.Set(18, y, cursorOutline)
	}

	return &CursorSprite{
		Image:    img,
		HotspotX: 0,
		HotspotY: 0,
	}
}

// createAliasCursor creates an alias/shortcut cursor.
func createAliasCursor() *CursorSprite {
	return createArrowCursor() // Fallback to arrow
}

// createCellCursor creates a cell/table cursor.
func createCellCursor() *CursorSprite {
	const size = 24
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := size/2, size/2

	// Draw plus sign
	drawLine(img, cx, 4, cx, 20, cursorOutline)
	drawLine(img, 4, cy, 20, cy, cursorOutline)

	return &CursorSprite{
		Image:    img,
		HotspotX: cx,
		HotspotY: cy,
	}
}

// Drawing helpers

func drawLine(img *image.RGBA, x1, y1, x2, y2 int, c color.Color) {
	// Bresenham's line algorithm
	dx := abs(x2 - x1)
	dy := abs(y2 - y1)
	sx := 1
	if x1 > x2 {
		sx = -1
	}
	sy := 1
	if y1 > y2 {
		sy = -1
	}
	err := dx - dy

	for {
		img.Set(x1, y1, c)
		if x1 == x2 && y1 == y2 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x1 += sx
		}
		if e2 < dx {
			err += dx
			y1 += sy
		}
	}
}

func drawTriangle(img *image.RGBA, x1, y1, x2, y2, x3, y3 int, c color.Color) {
	drawLine(img, x1, y1, x2, y2, c)
	drawLine(img, x2, y2, x3, y3, c)
	drawLine(img, x3, y3, x1, y1, c)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func cos(x float64) float64 {
	// Simple approximation
	return 1 - x*x/2 + x*x*x*x/24
}

func sin(x float64) float64 {
	// Simple approximation
	return x - x*x*x/6 + x*x*x*x*x/120
}

// CompositeCursorOnImage draws the cursor on the given image.
// Returns a new image with the cursor composited.
func CompositeCursorOnImage(img image.Image, cursorX, cursorY int, cursorName string) image.Image {
	sprite := GetCursorSprite(cursorName)

	// Calculate cursor position (adjust for hotspot)
	x := cursorX - sprite.HotspotX
	y := cursorY - sprite.HotspotY

	// Create a copy of the image to draw on
	bounds := img.Bounds()
	result := image.NewRGBA(bounds)
	draw.Draw(result, bounds, img, bounds.Min, draw.Src)

	// Draw the cursor sprite
	cursorBounds := sprite.Image.Bounds()
	cursorRect := image.Rect(x, y, x+cursorBounds.Dx(), y+cursorBounds.Dy())

	// Only draw if cursor is within image bounds
	if cursorRect.Overlaps(bounds) {
		draw.Draw(result, cursorRect, sprite.Image, cursorBounds.Min, draw.Over)
	}

	return result
}

// CompositeCursorOnPNG takes PNG data and returns new PNG data with cursor composited.
func CompositeCursorOnPNG(pngData []byte, cursorX, cursorY int, cursorName string) ([]byte, error) {
	// Decode PNG
	img, err := png.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, err
	}

	// Composite cursor
	result := CompositeCursorOnImage(img, cursorX, cursorY, cursorName)

	// Encode back to PNG
	var buf bytes.Buffer
	if err := png.Encode(&buf, result); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// CompositeCursorOnJPEG takes JPEG data and returns new JPEG data with cursor composited.
func CompositeCursorOnJPEG(jpegData []byte, cursorX, cursorY int, cursorName string, quality int) ([]byte, error) {
	// Decode JPEG
	img, err := jpeg.Decode(bytes.NewReader(jpegData))
	if err != nil {
		return nil, err
	}

	// Composite cursor
	result := CompositeCursorOnImage(img, cursorX, cursorY, cursorName)

	// Encode back to JPEG
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, result, &jpeg.Options{Quality: quality}); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// CompositeCursorOnImageData composites cursor onto image data in any supported format.
// Returns the data in the same format as input.
func CompositeCursorOnImageData(data []byte, format string, cursorX, cursorY int, cursorName string, quality int) ([]byte, error) {
	switch format {
	case "jpeg", "jpg":
		return CompositeCursorOnJPEG(data, cursorX, cursorY, cursorName, quality)
	case "png":
		return CompositeCursorOnPNG(data, cursorX, cursorY, cursorName)
	default:
		// Try PNG first, then JPEG
		if result, err := CompositeCursorOnPNG(data, cursorX, cursorY, cursorName); err == nil {
			return result, nil
		}
		return CompositeCursorOnJPEG(data, cursorX, cursorY, cursorName, quality)
	}
}
