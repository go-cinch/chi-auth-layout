package auth

import (
	"image"
	"image/color"
	"math"
)

func drawRotatedGlyph(canvas *image.RGBA, glyph *image.Alpha, center CaptchaPoint, angleDegrees int, foreground color.RGBA) {
	angle := float64(angleDegrees) * math.Pi / 180
	cosine, sine := math.Cos(angle), math.Sin(angle)
	width, height := glyph.Bounds().Dx(), glyph.Bounds().Dy()
	halfWidth, halfHeight := float64(width-1)/2, float64(height-1)/2
	radius := int(math.Ceil(math.Hypot(float64(width), float64(height)) / 2))
	for destinationY := -radius; destinationY <= radius; destinationY++ {
		for destinationX := -radius; destinationX <= radius; destinationX++ {
			point := image.Pt(center.X+destinationX, center.Y+destinationY)
			if !point.In(canvas.Bounds()) {
				continue
			}
			sourceX := cosine*float64(destinationX) + sine*float64(destinationY) + halfWidth + float64(glyph.Rect.Min.X)
			sourceY := -sine*float64(destinationX) + cosine*float64(destinationY) + halfHeight + float64(glyph.Rect.Min.Y)
			coverage := uint32(math.Round(pointCaptchaAlphaAt(glyph, sourceX, sourceY)))
			if coverage == 0 {
				continue
			}
			// Blend the interpolated coverage over the background, including noise
			// and glyph shadows. RGBA channels are already premultiplied by alpha.
			background := canvas.RGBAAt(point.X, point.Y)
			inverse := 255 - (uint32(foreground.A)*coverage+127)/255
			blend := func(source, destination uint8) uint8 {
				return uint8((uint32(source)*coverage + uint32(destination)*inverse + 127) / 255)
			}
			canvas.SetRGBA(point.X, point.Y, color.RGBA{
				R: blend(foreground.R, background.R),
				G: blend(foreground.G, background.G),
				B: blend(foreground.B, background.B),
				A: blend(foreground.A, background.A),
			})
		}
	}
}

func pointCaptchaAlphaAt(glyph *image.Alpha, x, y float64) float64 {
	left, top := int(math.Floor(x)), int(math.Floor(y))
	fractionX, fractionY := x-float64(left), y-float64(top)
	// AlphaAt returns transparent outside the mask, smoothing its outer edge.
	upper := float64(glyph.AlphaAt(left, top).A)*(1-fractionX) + float64(glyph.AlphaAt(left+1, top).A)*fractionX
	lower := float64(glyph.AlphaAt(left, top+1).A)*(1-fractionX) + float64(glyph.AlphaAt(left+1, top+1).A)*fractionX
	return upper*(1-fractionY) + lower*fractionY
}
