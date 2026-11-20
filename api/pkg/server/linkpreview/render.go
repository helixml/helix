package linkpreview

import (
	"bytes"
	_ "embed"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"strings"
	"sync"

	"golang.org/x/image/draw"
	"golang.org/x/image/font"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

// ImageWidth and ImageHeight are the recommended Open Graph card size.
const (
	ImageWidth  = 1200
	ImageHeight = 630
	margin      = 72
)

//go:embed logo.png
var logoPNG []byte

var (
	bg        = color.RGBA{0x0d, 0x0d, 0x0d, 0xff}
	fg        = color.RGBA{0xf5, 0xf5, 0xf5, 0xff}
	muted     = color.RGBA{0x9c, 0xa3, 0xaf, 0xff}
	accent    = color.RGBA{0x4a, 0xde, 0x80, 0xff}
	assetsErr error
	assets    struct {
		logo                          image.Image
		brand, project, number, title font.Face
		pill, stats                   font.Face
	}
	assetsOnce sync.Once
)

func loadAssets() error {
	assetsOnce.Do(func() {
		logo, err := png.Decode(bytes.NewReader(logoPNG))
		if err != nil {
			assetsErr = fmt.Errorf("decode logo: %w", err)
			return
		}
		assets.logo = logo
		bold, err := opentype.Parse(gobold.TTF)
		if err != nil {
			assetsErr = fmt.Errorf("parse bold font: %w", err)
			return
		}
		regular, err := opentype.Parse(goregular.TTF)
		if err != nil {
			assetsErr = fmt.Errorf("parse regular font: %w", err)
			return
		}
		faces := []struct {
			dst  *font.Face
			f    *opentype.Font
			size float64
		}{
			{&assets.brand, bold, 40},
			{&assets.project, regular, 30},
			{&assets.number, regular, 36},
			{&assets.title, bold, 60},
			{&assets.pill, bold, 28},
			{&assets.stats, regular, 32},
		}
		for _, fc := range faces {
			face, err := opentype.NewFace(fc.f, &opentype.FaceOptions{Size: fc.size, DPI: 72, Hinting: font.HintingFull})
			if err != nil {
				assetsErr = fmt.Errorf("font face: %w", err)
				return
			}
			*fc.dst = face
		}
	})
	return assetsErr
}

// RenderPNG draws the 1200x630 preview card.
func RenderPNG(p *Preview) ([]byte, error) {
	if err := loadAssets(); err != nil {
		return nil, err
	}
	img := image.NewRGBA(image.Rect(0, 0, ImageWidth, ImageHeight))
	draw.Draw(img, img.Bounds(), image.NewUniform(bg), image.Point{}, draw.Src)
	draw.Draw(img, image.Rect(0, 0, ImageWidth, 8), image.NewUniform(accent), image.Point{}, draw.Src)

	// Header: logo + wordmark on the left, project name on the right.
	const logoSize = 64
	draw.CatmullRom.Scale(img, image.Rect(margin, 56, margin+logoSize, 56+logoSize), assets.logo, assets.logo.Bounds(), draw.Over, nil)
	text(img, assets.brand, fg, margin+logoSize+20, 102, "Helix")
	if p.Project != "" {
		proj := fitText(assets.project, p.Project, 560)
		text(img, assets.project, muted, ImageWidth-margin-width(assets.project, proj), 100, proj)
	}

	// Body: task number and title (up to 3 lines).
	y := 220
	if p.Number > 0 {
		text(img, assets.number, muted, margin, y, fmt.Sprintf("#%d", p.Number))
		y += 76
	} else {
		y += 30
	}
	for _, line := range wrap(assets.title, p.Title, ImageWidth-2*margin, 3) {
		text(img, assets.title, fg, margin, y, line)
		y += 74
	}

	// Footer: status pill and PR stats.
	const footerY = ImageHeight - 72
	pillW := width(assets.pill, p.StatusLabel) + 64
	pillRect := image.Rect(margin, footerY-52, margin+pillW, footerY+8)
	roundRect(img, pillRect, 30, withAlpha(p.StatusColor, 0x33))
	dot := image.Rect(margin+22, footerY-28, margin+34, footerY-16)
	roundRect(img, dot, 6, p.StatusColor)
	text(img, assets.pill, p.StatusColor, margin+44, footerY-12, p.StatusLabel)
	if s := p.PRSummary(); s != "" {
		text(img, assets.stats, muted, ImageWidth-margin-width(assets.stats, s), footerY-10, s)
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		return nil, fmt.Errorf("encode png: %w", err)
	}
	return buf.Bytes(), nil
}

func text(dst *image.RGBA, face font.Face, c color.Color, x, y int, s string) {
	d := font.Drawer{Dst: dst, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

func width(face font.Face, s string) int {
	return font.MeasureString(face, s).Ceil()
}

// fitText truncates s with an ellipsis so it fits in maxW pixels.
func fitText(face font.Face, s string, maxW int) string {
	if width(face, s) <= maxW {
		return s
	}
	r := []rune(s)
	for len(r) > 0 && width(face, string(r)+"…") > maxW {
		r = r[:len(r)-1]
	}
	return strings.TrimSpace(string(r)) + "…"
}

// wrap breaks s into at most maxLines lines of maxW pixels, ellipsising the last.
func wrap(face font.Face, s string, maxW, maxLines int) []string {
	var lines []string
	cur := ""
	words := strings.Fields(s)
	for i, w := range words {
		next := w
		if cur != "" {
			next = cur + " " + w
		}
		if width(face, next) <= maxW || cur == "" {
			cur = next
			continue
		}
		lines = append(lines, cur)
		cur = w
		if len(lines) == maxLines-1 {
			cur = strings.Join(words[i:], " ")
			break
		}
	}
	if cur != "" {
		lines = append(lines, fitText(face, cur, maxW))
	}
	return lines
}

func withAlpha(c color.RGBA, a uint8) color.NRGBA {
	return color.NRGBA{c.R, c.G, c.B, a}
}

// roundRect fills r with rounded corners of radius rad.
func roundRect(dst *image.RGBA, r image.Rectangle, rad int, c color.Color) {
	src := image.NewUniform(c)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			cx, cy := x, y
			switch {
			case x < r.Min.X+rad:
				cx = r.Min.X + rad
			case x >= r.Max.X-rad:
				cx = r.Max.X - rad - 1
			}
			switch {
			case y < r.Min.Y+rad:
				cy = r.Min.Y + rad
			case y >= r.Max.Y-rad:
				cy = r.Max.Y - rad - 1
			}
			dx, dy := x-cx, y-cy
			if dx*dx+dy*dy <= rad*rad {
				draw.Draw(dst, image.Rect(x, y, x+1, y+1), src, image.Point{}, draw.Over)
			}
		}
	}
}
