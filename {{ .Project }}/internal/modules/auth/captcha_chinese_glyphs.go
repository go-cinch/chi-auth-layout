package auth

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"io"
	"math"
	"strings"
	"sync"
	"unicode/utf8"
)

var (
	pointCaptchaChineseGlyphOnce    sync.Once
	pointCaptchaChineseGlyphData    []byte
	pointCaptchaChineseGlyphIndexes map[rune]int
	pointCaptchaChineseGlyphErr     error
)

func loadChinesePointCaptchaGlyphs() error {
	pointCaptchaChineseGlyphOnce.Do(func() {
		decoder := hex.NewDecoder(strings.NewReader(strings.Join(strings.Fields(pointCaptchaChineseBitmapData), "")))
		compressed, err := io.ReadAll(decoder)
		if err != nil {
			pointCaptchaChineseGlyphErr = fmt.Errorf("decode Chinese captcha glyph data: %w", err)
			return
		}
		reader, err := zlib.NewReader(bytes.NewReader(compressed))
		if err != nil {
			pointCaptchaChineseGlyphErr = fmt.Errorf("open Chinese captcha glyph data: %w", err)
			return
		}
		defer reader.Close()
		pointCaptchaChineseGlyphData, err = io.ReadAll(reader)
		if err != nil {
			pointCaptchaChineseGlyphErr = fmt.Errorf("decompress Chinese captcha glyph data: %w", err)
			return
		}
		expected := utf8.RuneCountInString(pointCaptchaChineseGlyphSet) * pointCaptchaChineseBitmapBytes
		if len(pointCaptchaChineseGlyphData) != expected {
			pointCaptchaChineseGlyphErr = errors.New("Chinese captcha glyph data length is invalid")
			return
		}
		pointCaptchaChineseGlyphIndexes = make(map[rune]int, utf8.RuneCountInString(pointCaptchaChineseGlyphSet))
		for index, character := range []rune(pointCaptchaChineseGlyphSet) {
			pointCaptchaChineseGlyphIndexes[character] = index
		}
	})
	return pointCaptchaChineseGlyphErr
}

func hasChinesePointCaptchaGlyph(character rune) bool {
	if loadChinesePointCaptchaGlyphs() != nil {
		return false
	}
	_, ok := pointCaptchaChineseGlyphIndexes[character]
	return ok
}

func newChinesePointCaptchaGlyph(character rune, size int) (*image.Alpha, error) {
	if err := loadChinesePointCaptchaGlyphs(); err != nil {
		return nil, err
	}
	index, ok := pointCaptchaChineseGlyphIndexes[character]
	if !ok {
		return nil, fmt.Errorf("point captcha glyph %q is unavailable", character)
	}
	if size <= 0 {
		return nil, errors.New("point captcha glyph size must be positive")
	}
	source := pointCaptchaChineseGlyphData[index*pointCaptchaChineseBitmapBytes : (index+1)*pointCaptchaChineseBitmapBytes]
	glyph := image.NewAlpha(image.Rect(0, 0, size, size))
	// Area resampling preserves grayscale edge coverage when shrinking the
	// embedded 64-pixel glyphs to the captcha's display size.
	ratio := float64(pointCaptchaChineseBitmapSize) / float64(size)
	for y := 0; y < size; y++ {
		top, bottom := float64(y)*ratio, float64(y+1)*ratio
		for x := 0; x < size; x++ {
			left, right := float64(x)*ratio, float64(x+1)*ratio
			coverage := 0.0
			for sourceY := int(top); sourceY < min(int(math.Ceil(bottom)), pointCaptchaChineseBitmapSize); sourceY++ {
				weightY := min(bottom, float64(sourceY+1)) - max(top, float64(sourceY))
				for sourceX := int(left); sourceX < min(int(math.Ceil(right)), pointCaptchaChineseBitmapSize); sourceX++ {
					weightX := min(right, float64(sourceX+1)) - max(left, float64(sourceX))
					coverage += float64(source[sourceY*pointCaptchaChineseBitmapSize+sourceX]) * weightX * weightY
				}
			}
			glyph.Pix[y*glyph.Stride+x] = uint8(math.Round(coverage / (ratio * ratio)))
		}
	}
	return glyph, nil
}
