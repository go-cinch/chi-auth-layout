package auth

import (
	"image"
	"image/color"
	"image/draw"
	"testing"
)

func TestDrawRotatedGlyphBlendsCoverage(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(0, 0, 7, 7))
	draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
	glyph := image.NewAlpha(image.Rect(0, 0, 3, 3))
	glyph.SetAlpha(1, 1, color.Alpha{A: 128})
	drawRotatedGlyph(canvas, glyph, CaptchaPoint{X: 3, Y: 3}, 0, color.RGBA{A: 255})
	if got := canvas.RGBAAt(3, 3); got != (color.RGBA{R: 127, G: 127, B: 127, A: 255}) {
		t.Fatalf("half-covered pixel = %v; expected gray over white", got)
	}
	if got := canvas.RGBAAt(2, 3); got != (color.RGBA{R: 255, G: 255, B: 255, A: 255}) {
		t.Fatalf("transparent glyph pixel changed background: %v", got)
	}
}

func TestDrawRotatedGlyphSmoothsBothTiltDirections(t *testing.T) {
	glyph := image.NewAlpha(image.Rect(0, 0, 9, 9))
	draw.Draw(glyph, image.Rect(3, 1, 6, 8), image.Opaque, image.Point{}, draw.Src)
	for _, angle := range []int{-30, 30} {
		canvas := image.NewRGBA(image.Rect(0, 0, 21, 21))
		draw.Draw(canvas, canvas.Bounds(), image.White, image.Point{}, draw.Src)
		drawRotatedGlyph(canvas, glyph, CaptchaPoint{X: 10, Y: 10}, angle, color.RGBA{A: 255})
		solid, edges := 0, 0
		for y := 0; y < 21; y++ {
			for x := 0; x < 21; x++ {
				pixel := canvas.RGBAAt(x, y)
				if pixel.A != 255 || pixel.R != pixel.G || pixel.G != pixel.B {
					t.Fatalf("angle %d: invalid blended pixel %v", angle, pixel)
				}
				if pixel.R == 0 {
					solid++
				} else if pixel.R < 255 {
					edges++
				}
			}
		}
		if solid == 0 || edges == 0 {
			t.Fatalf("angle %d: solid=%d, antialiased edge=%d", angle, solid, edges)
		}
		if canvas.RGBAAt(0, 0).R != 255 {
			t.Fatal("rotation painted outside the glyph area")
		}
	}
}

func TestDrawRotatedGlyphClipsAndCompositesAlpha(t *testing.T) {
	canvas := image.NewRGBA(image.Rect(5, 5, 8, 8))
	glyph := image.NewAlpha(image.Rect(10, 10, 13, 13))
	draw.Draw(glyph, glyph.Bounds(), image.Opaque, image.Point{}, draw.Src)
	drawRotatedGlyph(canvas, glyph, CaptchaPoint{X: 5, Y: 5}, 0, color.RGBA{R: 128, A: 128})
	if got := canvas.RGBAAt(5, 5); got != (color.RGBA{R: 128, A: 128}) {
		t.Fatalf("translucent foreground on transparent canvas = %v", got)
	}
}
