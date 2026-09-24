package auth

import "testing"

func TestChinesePointCaptchaGlyphsRetainGrayEdges(t *testing.T) {
	for _, size := range []int{24, 32, 40} {
		for _, character := range "永国验证码警赢" {
			glyph, err := newChinesePointCaptchaGlyph(character, size)
			if err != nil {
				t.Fatal(err)
			}
			if glyph.Bounds().Dx() != size || glyph.Bounds().Dy() != size {
				t.Fatalf("%q: unexpected dimensions %v", character, glyph.Bounds())
			}
			transparent, solid, edge := false, false, false
			for _, alpha := range glyph.Pix {
				transparent = transparent || alpha == 0
				solid = solid || alpha >= 240
				edge = edge || (alpha > 0 && alpha < 240)
			}
			if !transparent || !solid || !edge {
				t.Fatalf("%q at size %d: missing background, stroke or antialiased edge", character, size)
			}
		}
	}
}

func TestChinesePointCaptchaGlyphsValidateInput(t *testing.T) {
	if _, err := newChinesePointCaptchaGlyph('🙂', 32); err == nil {
		t.Fatal("unsupported glyph accepted")
	}
	if _, err := newChinesePointCaptchaGlyph('中', 0); err == nil {
		t.Fatal("zero size accepted")
	}
}
