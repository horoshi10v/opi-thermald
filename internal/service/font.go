package service

import (
	"image"
	"image/color"
	"os"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"
)

var systemFontCandidates = []string{
	"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
	"/usr/share/fonts/truetype/liberation2/LiberationSans-Regular.ttf",
	"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
	"/System/Library/Fonts/Supplemental/Arial.ttf",
}

type textRenderer struct {
	cache map[float64]font.Face
}

func newTextRenderer() *textRenderer {
	renderer := &textRenderer{cache: make(map[float64]font.Face)}

	fontData := loadSystemFont()
	if len(fontData) == 0 {
		return renderer
	}

	parsed, err := opentype.Parse(fontData)
	if err != nil {
		return renderer
	}

	for _, size := range []float64{14, 18, 22, 30} {
		face, err := opentype.NewFace(parsed, &opentype.FaceOptions{
			Size:    size,
			DPI:     72,
			Hinting: font.HintingFull,
		})
		if err != nil {
			continue
		}
		renderer.cache[size] = face
	}

	return renderer
}

func loadSystemFont() []byte {
	for _, path := range systemFontCandidates {
		data, err := os.ReadFile(path)
		if err == nil {
			return data
		}
	}
	return nil
}

func (r *textRenderer) draw(img *image.RGBA, x, y int, text string, size float64, col color.Color) {
	face, ok := r.cache[size]
	if !ok {
		return
	}

	drawer := font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(col),
		Face: face,
		Dot:  fixed.P(x, y),
	}
	drawer.DrawString(text)
}

func (r *textRenderer) width(text string, size float64) int {
	face, ok := r.cache[size]
	if !ok {
		return 0
	}
	drawer := font.Drawer{Face: face}
	return drawer.MeasureString(text).Ceil()
}
