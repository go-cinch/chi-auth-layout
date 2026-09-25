package auth

import (
	"{{ .Computed.module_name_final }}/internal/common/apperror"
	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"math/big"
	"strings"
	"sync"
	"time"

{{- if .Computed.enable_redis_final }}
	"github.com/redis/go-redis/v9"
{{- end }}
)

const (
	pointCaptchaEnglishDictionaryKey  = "POINT_CAPTCHA_ENGLISH_CHARACTERS"
	pointCaptchaChineseDictionaryKey  = "POINT_CAPTCHA_CHINESE_CHARACTERS"
	pointCaptchaMinGlyphCount         = 6
	pointCaptchaMaxGlyphCount         = 8
	pointCaptchaMinTargetCount        = 2
	pointCaptchaMaxTargetCount        = 4
	pointCaptchaCanvasPadding         = 4
	pointCaptchaGlyphPadding          = 3
	pointCaptchaGlyphGap              = 0
	pointCaptchaLayoutAttempts        = 128
	pointCaptchaPlacementAttempts     = 256
	pointCaptchaFallbackMovesPerGlyph = 256
	pointCaptchaPurposeLogin          = "login"
	pointCaptchaPurposePasswordChange = "password_change"
)

var ErrPointCaptchaNotFound = apperror.New("AUTH_POINT_CAPTCHA_NOT_FOUND", "point captcha not found")

type CaptchaPoint struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type PointCaptchaChallenge struct {
	CaptchaID    string `json:"captcha_id"`
	CaptchaImage string `json:"captcha_image"`
	HintText     string `json:"hint_text"`
	TargetCount  int    `json:"target_count"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	ExpiredAt    int64  `json:"expired_at"`
}

type PointCaptchaStore interface {
	Put(context.Context, string, string, time.Duration) error
	Take(context.Context, string) (string, error)
}

type PointCaptchaDictionary interface {
	DictionaryValue(context.Context, string) (json.RawMessage, error)
}

type pointCaptchaAnswer struct {
	Purpose       string         `json:"purpose"`
	SubjectDigest string         `json:"subject_digest"`
	Points        []CaptchaPoint `json:"points"`
}

type PointCaptcha struct {
	store      PointCaptchaStore
	dictionary PointCaptchaDictionary
	threshold  int64
	ttl        time.Duration
	width      int
	height     int
	tolerance  int
	random     io.Reader
	enableE2ETest func() bool
}

// NewPointCaptcha evaluates enableE2ETest per validation; nil disables E2E.
func NewPointCaptcha(store PointCaptchaStore, dictionary PointCaptchaDictionary, threshold int64, ttl time.Duration, width, height, tolerance int, enableE2ETest func() bool) (*PointCaptcha, error) {
	if store == nil {
		return nil, errors.New("point captcha store is required")
	}
	if dictionary == nil {
		return nil, errors.New("point captcha dictionary is required")
	}
	if threshold < 1 {
		return nil, errors.New("point captcha threshold must be positive")
	}
	if ttl <= 0 || ttl > 10*time.Minute {
		return nil, errors.New("point captcha ttl must be between 1ns and 10m")
	}
	if width < 240 || width > 800 || height < 140 || height > 500 {
		return nil, errors.New("point captcha dimensions are out of range")
	}
	if tolerance < 8 || tolerance > 40 {
		return nil, errors.New("point captcha tolerance must be between 8 and 40 pixels")
	}
	if enableE2ETest == nil {
		enableE2ETest = func() bool { return false }
	}
	return &PointCaptcha{
		store: store, dictionary: dictionary, threshold: threshold, ttl: ttl, width: width, height: height,
		tolerance: tolerance, random: rand.Reader, enableE2ETest: enableE2ETest,
	}, nil
}

func (c *PointCaptcha) Required(wrong int64) bool {
	return wrong >= c.threshold
}

func (c *PointCaptcha) NewChallenge(ctx context.Context, username string) (*PointCaptchaChallenge, error) {
	return c.challenge(ctx, pointCaptchaPurposeLogin, usernameDigest(strings.TrimSpace(username)))
}

func (c *PointCaptcha) RefreshChallenge(ctx context.Context, captchaID string) (*PointCaptchaChallenge, error) {
	return c.refreshChallenge(ctx, captchaID, pointCaptchaPurposeLogin)
}

func (c *PointCaptcha) NewPasswordChangeChallenge(ctx context.Context, subject string) (*PointCaptchaChallenge, error) {
	return c.challenge(ctx, pointCaptchaPurposePasswordChange, usernameDigest(strings.TrimSpace(subject)))
}

func (c *PointCaptcha) RefreshPasswordChangeChallenge(ctx context.Context, captchaID string) (*PointCaptchaChallenge, error) {
	return c.refreshChallenge(ctx, captchaID, pointCaptchaPurposePasswordChange)
}

func (c *PointCaptcha) refreshChallenge(ctx context.Context, captchaID, purpose string) (*PointCaptchaChallenge, error) {
	captchaID = strings.TrimSpace(captchaID)
	if !validPointCaptchaID(captchaID, purpose) {
		return nil, ErrPointCaptchaNotFound
	}
	value, err := c.store.Take(ctx, captchaID)
	if err != nil {
		return nil, err
	}
	answer, err := decodePointCaptchaAnswer(value)
	if err != nil || answer.Purpose != purpose {
		return nil, ErrPointCaptchaNotFound
	}
	return c.challenge(ctx, purpose, answer.SubjectDigest)
}

func (c *PointCaptcha) Verify(ctx context.Context, username, captchaID string, points []CaptchaPoint) (bool, error) {
	return c.verify(ctx, pointCaptchaPurposeLogin, usernameDigest(strings.TrimSpace(username)), captchaID, points)
}

func (c *PointCaptcha) VerifyPasswordChange(ctx context.Context, subject, captchaID string, points []CaptchaPoint) (bool, error) {
	return c.verify(ctx, pointCaptchaPurposePasswordChange, usernameDigest(strings.TrimSpace(subject)), captchaID, points)
}

func (c *PointCaptcha) Check(ctx context.Context, username, captchaID string, points []CaptchaPoint) (bool, error) {
	return c.check(ctx, pointCaptchaPurposeLogin, usernameDigest(strings.TrimSpace(username)), captchaID, points)
}

func (c *PointCaptcha) CheckPasswordChange(ctx context.Context, subject, captchaID string, points []CaptchaPoint) (bool, error) {
	return c.check(ctx, pointCaptchaPurposePasswordChange, usernameDigest(strings.TrimSpace(subject)), captchaID, points)
}

func (c *PointCaptcha) verify(ctx context.Context, purpose, subjectDigest, captchaID string, points []CaptchaPoint) (bool, error) {
	captchaID = strings.TrimSpace(captchaID)
	e2eEnabled := c.enableE2ETest()
	if captchaID == "" || len(captchaID) > 128 || !validPointCaptchaID(captchaID, purpose) || (!e2eEnabled && !validPointCaptchaTargetCount(len(points))) {
		return false, nil
	}
	value, err := c.store.Take(ctx, captchaID)
	if errors.Is(err, ErrPointCaptchaNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consume point captcha: %w", err)
	}
	answer, err := decodePointCaptchaAnswer(value)
	if err != nil || !pointCaptchaSubjectMatches(answer, purpose, subjectDigest) || (!e2eEnabled && !pointCaptchaAnswerMatches(answer, purpose, subjectDigest, points, c.tolerance)) {
		return false, nil
	}
	return true, nil
}

func (c *PointCaptcha) check(ctx context.Context, purpose, subjectDigest, captchaID string, points []CaptchaPoint) (bool, error) {
	captchaID = strings.TrimSpace(captchaID)
	e2eEnabled := c.enableE2ETest()
	if captchaID == "" || len(captchaID) > 128 || !validPointCaptchaID(captchaID, purpose) || (!e2eEnabled && !validPointCaptchaTargetCount(len(points))) {
		return false, nil
	}
	value, err := c.store.Take(ctx, captchaID)
	if errors.Is(err, ErrPointCaptchaNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("consume point captcha for check: %w", err)
	}
	answer, err := decodePointCaptchaAnswer(value)
	if err != nil || !pointCaptchaSubjectMatches(answer, purpose, subjectDigest) || (!e2eEnabled && !pointCaptchaAnswerMatches(answer, purpose, subjectDigest, points, c.tolerance)) {
		return false, nil
	}
	if err := c.store.Put(ctx, captchaID, value, c.ttl); err != nil {
		return false, fmt.Errorf("restore checked point captcha: %w", err)
	}
	return true, nil
}

func pointCaptchaAnswerMatches(answer pointCaptchaAnswer, purpose, subjectDigest string, points []CaptchaPoint, tolerance int) bool {
	if !pointCaptchaSubjectMatches(answer, purpose, subjectDigest) || len(points) != len(answer.Points) {
		return false
	}
	toleranceSquared := tolerance * tolerance
	for index, expected := range answer.Points {
		dx := points[index].X - expected.X
		dy := points[index].Y - expected.Y
		if dx*dx+dy*dy > toleranceSquared {
			return false
		}
	}
	return true
}

func pointCaptchaSubjectMatches(answer pointCaptchaAnswer, purpose, subjectDigest string) bool {
	return answer.Purpose == purpose && subtle.ConstantTimeCompare([]byte(answer.SubjectDigest), []byte(subjectDigest)) == 1
}

func (c *PointCaptcha) challenge(ctx context.Context, purpose, subjectDigest string) (*PointCaptchaChallenge, error) {
	idBytes := make([]byte, 32)
	if _, err := io.ReadFull(c.random, idBytes); err != nil {
		return nil, fmt.Errorf("generate point captcha id: %w", err)
	}
	id := pointCaptchaIDPrefix(purpose) + base64.RawURLEncoding.EncodeToString(idBytes)
	imageData, hint, points, err := c.render(ctx)
	if err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(pointCaptchaAnswer{Purpose: purpose, SubjectDigest: subjectDigest, Points: points})
	if err != nil {
		return nil, fmt.Errorf("encode point captcha answer: %w", err)
	}
	if err := c.store.Put(ctx, id, string(encoded), c.ttl); err != nil {
		return nil, fmt.Errorf("store point captcha: %w", err)
	}
	return &PointCaptchaChallenge{
		CaptchaID: id, CaptchaImage: "data:image/png;base64," + base64.StdEncoding.EncodeToString(imageData),
		HintText: hint, TargetCount: len(points), Width: c.width, Height: c.height,
		ExpiredAt: time.Now().Add(c.ttl).UnixMilli(),
	}, nil
}

func pointCaptchaIDPrefix(purpose string) string {
	if purpose == pointCaptchaPurposePasswordChange {
		return "password-change:"
	}
	return purpose + ":"
}

func validPointCaptchaID(value, purpose string) bool {
	return strings.HasPrefix(value, pointCaptchaIDPrefix(purpose))
}

func decodePointCaptchaAnswer(value string) (pointCaptchaAnswer, error) {
	var answer pointCaptchaAnswer
	if err := json.Unmarshal([]byte(value), &answer); err != nil ||
		(answer.Purpose != pointCaptchaPurposeLogin && answer.Purpose != pointCaptchaPurposePasswordChange) ||
		answer.SubjectDigest == "" || !validPointCaptchaTargetCount(len(answer.Points)) {
		return pointCaptchaAnswer{}, ErrPointCaptchaNotFound
	}
	return answer, nil
}

func validPointCaptchaTargetCount(count int) bool {
	return count >= pointCaptchaMinTargetCount && count <= pointCaptchaMaxTargetCount
}

func usernameDigest(username string) string {
	digest := sha256.Sum256([]byte(username))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func (c *PointCaptcha) render(ctx context.Context) ([]byte, string, []CaptchaPoint, error) {
	chinese := i18n.Locale(ctx, "") == i18n.Chinese
	glyphCount, targetCount, err := c.pointCaptchaCounts()
	if err != nil {
		return nil, "", nil, fmt.Errorf("select point captcha counts: %w", err)
	}
	characters, err := c.pointCaptchaCharacters(ctx, chinese)
	if err != nil {
		return nil, "", nil, err
	}
	if err := c.shuffleRunes(characters); err != nil {
		return nil, "", nil, fmt.Errorf("select point captcha characters: %w", err)
	}
	characters = characters[:glyphCount]
	order := make([]int, glyphCount)
	for index := range order {
		order[index] = index
	}
	if err := c.shuffleInts(order); err != nil {
		return nil, "", nil, fmt.Errorf("select point captcha targets: %w", err)
	}

	canvas := image.NewRGBA(image.Rect(0, 0, c.width, c.height))
	draw.Draw(canvas, canvas.Bounds(), &image.Uniform{C: color.RGBA{R: 246, G: 248, B: 252, A: 255}}, image.Point{}, draw.Src)
	if err := c.drawNoise(canvas); err != nil {
		return nil, "", nil, err
	}

	type renderedGlyph struct {
		alpha      *image.Alpha
		angle      int
		halfWidth  int
		halfHeight int
	}
	glyphs := make([]renderedGlyph, glyphCount)
	for index, character := range characters {
		glyph, err := c.pointCaptchaGlyph(character, chinese, glyphCount)
		if err != nil {
			return nil, "", nil, err
		}
		angle, err := c.pointCaptchaTilt()
		if err != nil {
			return nil, "", nil, fmt.Errorf("tilt point captcha glyph: %w", err)
		}
		halfWidth, halfHeight := rotatedGlyphHalfExtents(glyph, angle)
		glyphs[index] = renderedGlyph{alpha: glyph, angle: angle, halfWidth: halfWidth, halfHeight: halfHeight}
	}
	dimensions := make([]image.Point, glyphCount)
	for index, glyph := range glyphs {
		dimensions[index] = image.Pt(glyph.halfWidth, glyph.halfHeight)
	}
	centers, err := c.randomPointCaptchaCenters(dimensions)
	if err != nil {
		return nil, "", nil, err
	}
	for index, glyph := range glyphs {
		shade, err := c.randomInt(80)
		if err != nil {
			return nil, "", nil, err
		}
		foreground := color.RGBA{R: uint8(20 + shade), G: uint8(35 + (shade*2)%70), B: uint8(80 + shade), A: 255}
		center := centers[index]
		drawRotatedGlyph(canvas, glyph.alpha, CaptchaPoint{X: center.X + 2, Y: center.Y + 2}, glyph.angle, color.RGBA{R: 210, G: 214, B: 225, A: 255})
		drawRotatedGlyph(canvas, glyph.alpha, center, glyph.angle, foreground)
	}

	targets := make([]CaptchaPoint, targetCount)
	hintParts := make([]string, targetCount)
	for index := range targetCount {
		characterIndex := order[index]
		targets[index] = centers[characterIndex]
		hintParts[index] = string(characters[characterIndex])
	}
	var output bytes.Buffer
	if err := png.Encode(&output, canvas); err != nil {
		return nil, "", nil, fmt.Errorf("encode point captcha image: %w", err)
	}
	return output.Bytes(), strings.Join(hintParts, " · "), targets, nil
}

func (c *PointCaptcha) pointCaptchaCounts() (int, int, error) {
	glyphOffset, err := c.randomInt(pointCaptchaMaxGlyphCount - pointCaptchaMinGlyphCount + 1)
	if err != nil {
		return 0, 0, err
	}
	targetOffset, err := c.randomInt(pointCaptchaMaxTargetCount - pointCaptchaMinTargetCount + 1)
	if err != nil {
		return 0, 0, err
	}
	return pointCaptchaMinGlyphCount + glyphOffset, pointCaptchaMinTargetCount + targetOffset, nil
}

func rotatedGlyphHalfExtents(glyph *image.Alpha, angleDegrees int) (int, int) {
	angle := float64(angleDegrees) * math.Pi / 180
	width, height := float64(glyph.Bounds().Dx()), float64(glyph.Bounds().Dy())
	rotatedWidth := math.Abs(math.Cos(angle))*width + math.Abs(math.Sin(angle))*height
	rotatedHeight := math.Abs(math.Sin(angle))*width + math.Abs(math.Cos(angle))*height
	return int(math.Ceil(rotatedWidth/2)) + pointCaptchaGlyphPadding,
		int(math.Ceil(rotatedHeight/2)) + pointCaptchaGlyphPadding
}

func (c *PointCaptcha) randomPointCaptchaCenters(dimensions []image.Point) ([]CaptchaPoint, error) {
	centers := make([]CaptchaPoint, len(dimensions))
	order := make([]int, len(dimensions))
	for index := range order {
		order[index] = index
	}
	// Place larger glyphs first so the random packing does not strand them in
	// narrow gaps. Their final positions are sampled across the available
	// canvas instead of being tied to rows or columns.
	for left := 0; left < len(order); left++ {
		largest := left
		for right := left + 1; right < len(order); right++ {
			leftSize := dimensions[order[largest]]
			rightSize := dimensions[order[right]]
			if rightSize.X*rightSize.Y > leftSize.X*leftSize.Y {
				largest = right
			}
		}
		order[left], order[largest] = order[largest], order[left]
	}

	for range pointCaptchaLayoutAttempts {
		placed := make([]int, 0, len(dimensions))
		for _, index := range order {
			halfSize := dimensions[index]
			availableWidth := c.width - 2*(halfSize.X+pointCaptchaCanvasPadding) + 1
			availableHeight := c.height - 2*(halfSize.Y+pointCaptchaCanvasPadding) + 1
			if availableWidth <= 0 || availableHeight <= 0 {
				return nil, errors.New("point captcha glyph does not fit inside canvas")
			}
			found := false
			for range pointCaptchaPlacementAttempts {
				x, err := c.randomInt(availableWidth)
				if err != nil {
					return nil, fmt.Errorf("position point captcha glyph: %w", err)
				}
				y, err := c.randomInt(availableHeight)
				if err != nil {
					return nil, fmt.Errorf("position point captcha glyph: %w", err)
				}
				candidate := CaptchaPoint{X: halfSize.X + pointCaptchaCanvasPadding + x, Y: halfSize.Y + pointCaptchaCanvasPadding + y}
				if pointCaptchaCenterOverlaps(candidate, halfSize, centers, dimensions, placed) {
					continue
				}
				centers[index] = candidate
				placed = append(placed, index)
				found = true
				break
			}
			if !found {
				break
			}
		}
		if len(placed) == len(dimensions) {
			return centers, nil
		}
	}
	return c.randomPointCaptchaGridCenters(dimensions, order)
}

func (c *PointCaptcha) randomPointCaptchaGridCenters(dimensions []image.Point, order []int) ([]CaptchaPoint, error) {
	if len(dimensions) == 0 {
		return nil, nil
	}
	columns, rows := (len(dimensions)+1)/2, 2
	cellOrder := make([]int, columns*rows)
	for index := range cellOrder {
		cellOrder[index] = index
	}
	if err := c.shuffleInts(cellOrder); err != nil {
		return nil, fmt.Errorf("shuffle point captcha fallback cells: %w", err)
	}
	innerWidth := c.width - 2*pointCaptchaCanvasPadding
	innerHeight := c.height - 2*pointCaptchaCanvasPadding
	centers := make([]CaptchaPoint, len(dimensions))
	for placementIndex, glyphIndex := range order {
		cell := cellOrder[placementIndex]
		column, row := cell%columns, cell/columns
		left := pointCaptchaCanvasPadding + column*innerWidth/columns
		right := pointCaptchaCanvasPadding + (column+1)*innerWidth/columns
		top := pointCaptchaCanvasPadding + row*innerHeight/rows
		bottom := pointCaptchaCanvasPadding + (row+1)*innerHeight/rows
		halfSize := dimensions[glyphIndex]
		availableWidth := right - left - 2*halfSize.X + 1
		availableHeight := bottom - top - 2*halfSize.Y + 1
		if availableWidth <= 0 || availableHeight <= 0 {
			return nil, errors.New("point captcha glyphs do not fit inside fallback cells")
		}
		x, err := c.randomInt(availableWidth)
		if err != nil {
			return nil, fmt.Errorf("position point captcha fallback glyph: %w", err)
		}
		y, err := c.randomInt(availableHeight)
		if err != nil {
			return nil, fmt.Errorf("position point captcha fallback glyph: %w", err)
		}
		centers[glyphIndex] = CaptchaPoint{X: left + halfSize.X + x, Y: top + halfSize.Y + y}
	}
	return c.randomizePointCaptchaCenters(centers, dimensions)
}

func (c *PointCaptcha) randomizePointCaptchaCenters(centers []CaptchaPoint, dimensions []image.Point) ([]CaptchaPoint, error) {
	for range len(dimensions) * pointCaptchaFallbackMovesPerGlyph {
		index, err := c.randomInt(len(dimensions))
		if err != nil {
			return nil, fmt.Errorf("select point captcha fallback glyph: %w", err)
		}
		halfSize := dimensions[index]
		availableWidth := c.width - 2*(halfSize.X+pointCaptchaCanvasPadding) + 1
		availableHeight := c.height - 2*(halfSize.Y+pointCaptchaCanvasPadding) + 1
		x, err := c.randomInt(availableWidth)
		if err != nil {
			return nil, fmt.Errorf("reposition point captcha fallback glyph: %w", err)
		}
		y, err := c.randomInt(availableHeight)
		if err != nil {
			return nil, fmt.Errorf("reposition point captcha fallback glyph: %w", err)
		}
		candidate := CaptchaPoint{X: halfSize.X + pointCaptchaCanvasPadding + x, Y: halfSize.Y + pointCaptchaCanvasPadding + y}
		overlaps := false
		for otherIndex, otherCenter := range centers {
			if otherIndex == index {
				continue
			}
			otherSize := dimensions[otherIndex]
			separatedX := abs(candidate.X-otherCenter.X) >= halfSize.X+otherSize.X+pointCaptchaGlyphGap
			separatedY := abs(candidate.Y-otherCenter.Y) >= halfSize.Y+otherSize.Y+pointCaptchaGlyphGap
			if !separatedX && !separatedY {
				overlaps = true
				break
			}
		}
		if !overlaps {
			centers[index] = candidate
		}
	}
	return centers, nil
}

func pointCaptchaCenterOverlaps(candidate CaptchaPoint, candidateSize image.Point, centers []CaptchaPoint, dimensions []image.Point, placed []int) bool {
	for _, index := range placed {
		existing := centers[index]
		existingSize := dimensions[index]
		separatedX := abs(candidate.X-existing.X) >= candidateSize.X+existingSize.X+pointCaptchaGlyphGap
		separatedY := abs(candidate.Y-existing.Y) >= candidateSize.Y+existingSize.Y+pointCaptchaGlyphGap
		if !separatedX && !separatedY {
			return true
		}
	}
	return false
}

func (c *PointCaptcha) drawNoise(canvas *image.RGBA) error {
	for range 24 {
		x1, err := c.randomInt(c.width)
		if err != nil {
			return fmt.Errorf("draw point captcha noise: %w", err)
		}
		y1, err := c.randomInt(c.height)
		if err != nil {
			return fmt.Errorf("draw point captcha noise: %w", err)
		}
		x2, err := c.randomInt(c.width)
		if err != nil {
			return fmt.Errorf("draw point captcha noise: %w", err)
		}
		y2, err := c.randomInt(c.height)
		if err != nil {
			return fmt.Errorf("draw point captcha noise: %w", err)
		}
		shade, err := c.randomInt(45)
		if err != nil {
			return fmt.Errorf("draw point captcha noise: %w", err)
		}
		drawLine(canvas, x1, y1, x2, y2, color.RGBA{R: uint8(190 + shade), G: uint8(195 + shade), B: uint8(205 + shade), A: 255})
	}
	for range c.width * c.height / 90 {
		x, err := c.randomInt(c.width)
		if err != nil {
			return fmt.Errorf("draw point captcha speckle: %w", err)
		}
		y, err := c.randomInt(c.height)
		if err != nil {
			return fmt.Errorf("draw point captcha speckle: %w", err)
		}
		canvas.Set(x, y, color.RGBA{R: 150, G: 165, B: 190, A: 255})
	}
	return nil
}

func (c *PointCaptcha) shuffleRunes(values []rune) error {
	for index := len(values) - 1; index > 0; index-- {
		other, err := c.randomInt(index + 1)
		if err != nil {
			return err
		}
		values[index], values[other] = values[other], values[index]
	}
	return nil
}

func (c *PointCaptcha) pointCaptchaTilt() (int, error) {
	angle, err := c.randomInt(61)
	if err != nil {
		return 0, err
	}
	return angle - 30, nil
}

func (c *PointCaptcha) pointCaptchaCharacters(ctx context.Context, chinese bool) ([]rune, error) {
	key := pointCaptchaEnglishDictionaryKey
	if chinese {
		key = pointCaptchaChineseDictionaryKey
	}
	raw, err := c.dictionary.DictionaryValue(ctx, key)
	if err != nil {
		return nil, fmt.Errorf("load point captcha dictionary %s: %w", key, err)
	}
	var values []string
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode point captcha dictionary %s: %w", key, err)
	}
	characters := make([]rune, 0, len(values))
	seen := make(map[rune]struct{}, len(values))
	for _, value := range values {
		runes := []rune(value)
		if len(runes) != 1 {
			return nil, fmt.Errorf("point captcha dictionary %s contains a non-character value", key)
		}
		character := runes[0]
		if chinese {
			if !hasChinesePointCaptchaGlyph(character) {
				return nil, fmt.Errorf("point captcha dictionary %s contains unsupported character %q", key, character)
			}
		} else if _, ok := pointCaptchaGlyphs[byte(character)]; character > 127 || !ok {
			return nil, fmt.Errorf("point captcha dictionary %s contains unsupported character %q", key, character)
		}
		if _, ok := seen[character]; ok {
			continue
		}
		seen[character] = struct{}{}
		characters = append(characters, character)
	}
	if len(characters) < pointCaptchaMaxGlyphCount {
		return nil, fmt.Errorf("point captcha dictionary %s must contain at least %d unique characters", key, pointCaptchaMaxGlyphCount)
	}
	return characters, nil
}

func (c *PointCaptcha) pointCaptchaGlyph(character rune, chinese bool, glyphCount int) (*image.Alpha, error) {
	if chinese {
		maximumSize := c.maxChinesePointCaptchaGlyphSize(glyphCount)
		minimumSize := max(24, maximumSize-8)
		sizeOffset, err := c.randomInt(maximumSize - minimumSize + 1)
		if err != nil {
			return nil, fmt.Errorf("size point captcha glyph: %w", err)
		}
		return newChinesePointCaptchaGlyph(character, minimumSize+sizeOffset)
	}
	scaleOffset, err := c.randomInt(3)
	if err != nil {
		return nil, fmt.Errorf("size point captcha glyph: %w", err)
	}
	return newLatinPointCaptchaGlyph(character, 4+scaleOffset)
}

func (c *PointCaptcha) maxChinesePointCaptchaGlyphSize(glyphCount int) int {
	columns := (glyphCount + 1) / 2
	cellWidth := (c.width - 2*pointCaptchaCanvasPadding) / columns
	cellHeight := (c.height - 2*pointCaptchaCanvasPadding) / 2
	cellSize := min(cellWidth, cellHeight)
	const maximumSize = 40
	const sinePlusCosine30Degrees = 1.3660254037844386
	for size := maximumSize; size > 0; size-- {
		halfExtent := int(math.Ceil(sinePlusCosine30Degrees*float64(size)/2)) + pointCaptchaGlyphPadding
		if 2*halfExtent <= cellSize {
			return size
		}
	}
	return 1
}

func (c *PointCaptcha) shuffleInts(values []int) error {
	for index := len(values) - 1; index > 0; index-- {
		other, err := c.randomInt(index + 1)
		if err != nil {
			return err
		}
		values[index], values[other] = values[other], values[index]
	}
	return nil
}

func (c *PointCaptcha) randomInt(maximum int) (int, error) {
	if maximum <= 0 {
		return 0, errors.New("random maximum must be positive")
	}
	value, err := rand.Int(c.random, big.NewInt(int64(maximum)))
	if err != nil {
		return 0, err
	}
	return int(value.Int64()), nil
}

func newLatinPointCaptchaGlyph(character rune, scale int) (*image.Alpha, error) {
	rows, ok := pointCaptchaGlyphs[byte(character)]
	if !ok {
		return nil, fmt.Errorf("point captcha glyph %q is unavailable", character)
	}
	glyph := image.NewAlpha(image.Rect(0, 0, len(rows[0])*scale, len(rows)*scale))
	for row, pattern := range rows {
		for column, pixel := range pattern {
			if pixel != '1' {
				continue
			}
			draw.Draw(glyph, image.Rect(column*scale, row*scale, (column+1)*scale, (row+1)*scale), image.White, image.Point{}, draw.Src)
		}
	}
	return glyph, nil
}

func drawLine(canvas *image.RGBA, x0, y0, x1, y1 int, lineColor color.Color) {
	dx, dy := abs(x1-x0), -abs(y1-y0)
	stepX, stepY := -1, -1
	if x0 < x1 {
		stepX = 1
	}
	if y0 < y1 {
		stepY = 1
	}
	err := dx + dy
	for {
		if image.Pt(x0, y0).In(canvas.Bounds()) {
			canvas.Set(x0, y0, lineColor)
		}
		if x0 == x1 && y0 == y1 {
			return
		}
		doubled := 2 * err
		if doubled >= dy {
			err += dy
			x0 += stepX
		}
		if doubled <= dx {
			err += dx
			y0 += stepY
		}
	}
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

var pointCaptchaGlyphs = map[byte][7]string{
	'0': {"01110", "10001", "10011", "10101", "11001", "10001", "01110"},
	'1': {"00100", "01100", "00100", "00100", "00100", "00100", "01110"},
	'2': {"11110", "00001", "00001", "11110", "10000", "10000", "11111"},
	'3': {"11110", "00001", "00001", "01110", "00001", "00001", "11110"},
	'4': {"10010", "10010", "10010", "11111", "00010", "00010", "00010"},
	'5': {"11111", "10000", "10000", "11110", "00001", "00001", "11110"},
	'6': {"01111", "10000", "10000", "11110", "10001", "10001", "01110"},
	'7': {"11111", "00001", "00010", "00100", "01000", "01000", "01000"},
	'8': {"01110", "10001", "10001", "01110", "10001", "10001", "01110"},
	'9': {"01110", "10001", "10001", "01111", "00001", "00001", "11110"},
	'A': {"01110", "10001", "10001", "11111", "10001", "10001", "10001"},
	'B': {"11110", "10001", "10001", "11110", "10001", "10001", "11110"},
	'C': {"01111", "10000", "10000", "10000", "10000", "10000", "01111"},
	'D': {"11110", "10001", "10001", "10001", "10001", "10001", "11110"},
	'E': {"11111", "10000", "10000", "11110", "10000", "10000", "11111"},
	'F': {"11111", "10000", "10000", "11110", "10000", "10000", "10000"},
	'G': {"01111", "10000", "10000", "10111", "10001", "10001", "01111"},
	'H': {"10001", "10001", "10001", "11111", "10001", "10001", "10001"},
	'I': {"11111", "00100", "00100", "00100", "00100", "00100", "11111"},
	'J': {"00111", "00010", "00010", "00010", "10010", "10010", "01100"},
	'K': {"10001", "10010", "10100", "11000", "10100", "10010", "10001"},
	'L': {"10000", "10000", "10000", "10000", "10000", "10000", "11111"},
	'M': {"10001", "11011", "10101", "10101", "10001", "10001", "10001"},
	'N': {"10001", "11001", "10101", "10011", "10001", "10001", "10001"},
	'O': {"01110", "10001", "10001", "10001", "10001", "10001", "01110"},
	'P': {"11110", "10001", "10001", "11110", "10000", "10000", "10000"},
	'Q': {"01110", "10001", "10001", "10001", "10101", "10010", "01101"},
	'R': {"11110", "10001", "10001", "11110", "10100", "10010", "10001"},
	'S': {"01111", "10000", "10000", "01110", "00001", "00001", "11110"},
	'T': {"11111", "00100", "00100", "00100", "00100", "00100", "00100"},
	'U': {"10001", "10001", "10001", "10001", "10001", "10001", "01110"},
	'V': {"10001", "10001", "10001", "10001", "10001", "01010", "00100"},
	'W': {"10001", "10001", "10001", "10101", "10101", "10101", "01010"},
	'X': {"10001", "10001", "01010", "00100", "01010", "10001", "10001"},
	'Y': {"10001", "10001", "01010", "00100", "00100", "00100", "00100"},
	'Z': {"11111", "00001", "00010", "00100", "01000", "10000", "11111"},
}

type memoryPointCaptcha struct {
	value     string
	expiredAt time.Time
}

type memoryPointCaptchaStore struct {
	mu     sync.Mutex
	values map[string]memoryPointCaptcha
}

func NewMemoryPointCaptchaStore() PointCaptchaStore {
	return &memoryPointCaptchaStore{values: make(map[string]memoryPointCaptcha)}
}

func (s *memoryPointCaptchaStore) Put(_ context.Context, id, value string, ttl time.Duration) error {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()
	for currentID, captcha := range s.values {
		if !captcha.expiredAt.After(now) {
			delete(s.values, currentID)
		}
	}
	s.values[id] = memoryPointCaptcha{value: value, expiredAt: now.Add(ttl)}
	return nil
}

func (s *memoryPointCaptchaStore) Take(_ context.Context, id string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	captcha, ok := s.values[id]
	delete(s.values, id)
	if !ok || !captcha.expiredAt.After(time.Now()) {
		return "", ErrPointCaptchaNotFound
	}
	return captcha.value, nil
}

{{ if .Computed.enable_redis_final -}}
type redisPointCaptchaStore struct {
	client redisChallengeClient
}

func NewRedisPointCaptchaStore(client redisChallengeClient) PointCaptchaStore {
	return &redisPointCaptchaStore{client: client}
}

func (s *redisPointCaptchaStore) Put(ctx context.Context, id, value string, ttl time.Duration) error {
	return s.client.Set(ctx, "point-captcha:"+id, value, ttl).Err()
}

func (s *redisPointCaptchaStore) Take(ctx context.Context, id string) (string, error) {
	value, err := s.client.GetDel(ctx, "point-captcha:"+id).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrPointCaptchaNotFound
	}
	return value, err
}
{{ end -}}
