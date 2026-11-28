package auth

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/draw"
	"strings"
	"testing"
	"time"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
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

func TestPointCaptchaUsesRequestLocale(t *testing.T) {
	for _, test := range []struct {
		name       string
		ctx        context.Context
		characters string
	}{
		{name: "english", ctx: t.Context(), characters: pointCaptchaTestEnglishCharacters},
		{name: "chinese", ctx: i18n.WithLocale(t.Context(), i18n.Chinese), characters: pointCaptchaTestChineseCharacters},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager := newPointCaptchaTestManager(t, NewMemoryPointCaptchaStore())
			challenge, err := manager.NewChallenge(test.ctx, "readonly")
			if err != nil {
				t.Fatal(err)
			}
			parts := strings.Split(challenge.HintText, " · ")
			if !validPointCaptchaTargetCount(len(parts)) || len(parts) != challenge.TargetCount {
				t.Fatalf("hint = %q", challenge.HintText)
			}
			for _, part := range parts {
				runes := []rune(part)
				if len(runes) != 1 || !strings.ContainsRune(test.characters, runes[0]) {
					t.Fatalf("hint %q does not use %s glyphs", challenge.HintText, test.name)
				}
			}
		})
	}
}

func TestPointCaptchaRandomCountsIncludeConfiguredBounds(t *testing.T) {
	manager := newPointCaptchaTestManager(t, NewMemoryPointCaptchaStore())
	for _, test := range []struct {
		name                    string
		random                  []byte
		wantGlyphs, wantTargets int
	}{
		{name: "minimum", random: []byte{0, 0}, wantGlyphs: 6, wantTargets: 2},
		{name: "maximum", random: []byte{2, 2}, wantGlyphs: 8, wantTargets: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			manager.random = bytes.NewReader(test.random)
			glyphs, targets, err := manager.pointCaptchaCounts()
			if err != nil || glyphs != test.wantGlyphs || targets != test.wantTargets {
				t.Fatalf("counts = %d, %d, %v", glyphs, targets, err)
			}
		})
	}
}

func TestPointCaptchaPositionsDoNotOverlap(t *testing.T) {
	manager := newPointCaptchaTestManager(t, NewMemoryPointCaptchaStore())
	dimensions := []image.Point{
		image.Pt(32, 32), image.Pt(31, 30), image.Pt(30, 31), image.Pt(29, 32),
		image.Pt(32, 29), image.Pt(30, 30), image.Pt(28, 31), image.Pt(31, 28),
	}
	for iteration := range 100 {
		centers, err := manager.randomPointCaptchaCenters(dimensions)
		if err != nil {
			t.Fatalf("iteration %d: %v", iteration, err)
		}
		for index, center := range centers {
			halfSize := dimensions[index]
			if center.X-halfSize.X < pointCaptchaCanvasPadding || center.X+halfSize.X > manager.width-pointCaptchaCanvasPadding ||
				center.Y-halfSize.Y < pointCaptchaCanvasPadding || center.Y+halfSize.Y > manager.height-pointCaptchaCanvasPadding {
				t.Fatalf("iteration %d: glyph %d is outside the canvas: center=%#v size=%v", iteration, index, center, halfSize)
			}
			placed := make([]int, index)
			for placedIndex := range index {
				placed[placedIndex] = placedIndex
			}
			if pointCaptchaCenterOverlaps(center, halfSize, centers, dimensions, placed) {
				t.Fatalf("iteration %d: glyph %d overlaps an earlier glyph", iteration, index)
			}
		}
	}
}

func TestPointCaptchaGlyphsSupportBidirectionalTilt(t *testing.T) {
	hasLeft, hasRight := false, false
	manager := newPointCaptchaTestManager(t, NewMemoryPointCaptchaStore())
	for range 200 {
		angle, err := manager.pointCaptchaTilt()
		if err != nil || angle < -30 || angle > 30 {
			t.Fatalf("captcha tilt = %d, %v", angle, err)
		}
		hasLeft = hasLeft || angle < 0
		hasRight = hasRight || angle > 0
	}
	if !hasLeft || !hasRight {
		t.Fatal("captcha tilt did not cover both directions")
	}
	characters := []rune(pointCaptchaChineseGlyphSet)
	if len(characters) != 3755 {
		t.Fatalf("Chinese glyph count = %d", len(characters))
	}
	for _, character := range characters[:128] {
		glyph, err := newChinesePointCaptchaGlyph(character, 42)
		if err != nil {
			t.Fatal(err)
		}
		nonempty := false
		for _, alpha := range glyph.Pix {
			nonempty = nonempty || alpha != 0
		}
		if !nonempty {
			t.Fatalf("Chinese glyph %q is empty", character)
		}
	}
}

func TestPointCaptchaChineseGlyphSizeFitsMinimumCanvas(t *testing.T) {
	manager, err := NewPointCaptcha(NewMemoryPointCaptchaStore(), newPointCaptchaTestDictionary(), 1, time.Minute, 240, 140, 20, false)
	if err != nil {
		t.Fatal(err)
	}
	maximumSize := manager.maxChinesePointCaptchaGlyphSize(pointCaptchaMaxGlyphCount)
	if maximumSize < 32 || maximumSize > 40 {
		t.Fatalf("maximum Chinese glyph size = %d", maximumSize)
	}
	glyph, err := newChinesePointCaptchaGlyph([]rune(pointCaptchaTestChineseCharacters)[0], maximumSize)
	if err != nil {
		t.Fatal(err)
	}
	halfWidth, halfHeight := rotatedGlyphHalfExtents(glyph, 30)
	centers, err := manager.randomPointCaptchaGridCenters(
		[]image.Point{
			image.Pt(halfWidth, halfHeight), image.Pt(halfWidth, halfHeight),
			image.Pt(halfWidth, halfHeight), image.Pt(halfWidth, halfHeight),
			image.Pt(halfWidth, halfHeight), image.Pt(halfWidth, halfHeight),
			image.Pt(halfWidth, halfHeight), image.Pt(halfWidth, halfHeight),
		},
		[]int{0, 1, 2, 3, 4, 5, 6, 7},
	)
	if err != nil || len(centers) != pointCaptchaMaxGlyphCount {
		t.Fatalf("minimum-canvas fallback centers = %#v, %v", centers, err)
	}
}
