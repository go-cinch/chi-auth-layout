package auth

import (
	"{{ .Computed.module_name_final }}/internal/common/i18n"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image"
	"image/png"
	"strings"
	"testing"
	"time"
)

const (
	pointCaptchaTestEnglishCharacters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	pointCaptchaTestChineseCharacters = "啊阿埃挨哎唉哀皑癌蔼矮艾碍爱隘鞍氨安俺按暗岸胺案肮昂盎凹敖熬翱袄傲奥懊澳芭捌扒叭吧笆八疤巴拔"
)

type pointCaptchaTestDictionary struct {
	values map[string]json.RawMessage
	err    error
}

func (d pointCaptchaTestDictionary) DictionaryValue(_ context.Context, key string) (json.RawMessage, error) {
	if d.err != nil {
		return nil, d.err
	}
	return d.values[key], nil
}

func newPointCaptchaTestDictionary() PointCaptchaDictionary {
	encode := func(characters string) json.RawMessage {
		values := make([]string, 0, len([]rune(characters)))
		for _, character := range characters {
			values = append(values, string(character))
		}
		encoded, _ := json.Marshal(values)
		return encoded
	}
	return pointCaptchaTestDictionary{values: map[string]json.RawMessage{
		pointCaptchaEnglishDictionaryKey: encode(pointCaptchaTestEnglishCharacters),
		pointCaptchaChineseDictionaryKey: encode(pointCaptchaTestChineseCharacters),
	}}
}

func newPointCaptchaTestManager(t *testing.T, store PointCaptchaStore) *PointCaptcha {
	t.Helper()
	manager, err := NewPointCaptcha(store, newPointCaptchaTestDictionary(), 5, time.Minute, 300, 180, 22)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func storedPointCaptchaAnswer(t *testing.T, store *memoryPointCaptchaStore, id string) pointCaptchaAnswer {
	t.Helper()
	store.mu.Lock()
	value := store.values[id].value
	store.mu.Unlock()
	answer, err := decodePointCaptchaAnswer(value)
	if err != nil {
		t.Fatal(err)
	}
	return answer
}

func TestPointCaptchaChallengeAndVerify(t *testing.T) {
	store := NewMemoryPointCaptchaStore().(*memoryPointCaptchaStore)
	manager := newPointCaptchaTestManager(t, store)
	if manager.Required(4) || !manager.Required(5) {
		t.Fatal("wrong-password threshold is incorrect")
	}

	challenge, err := manager.NewChallenge(t.Context(), " readonly ")
	if err != nil {
		t.Fatal(err)
	}
	if challenge.CaptchaID == "" || !validPointCaptchaTargetCount(challenge.TargetCount) || challenge.Width != 300 || challenge.Height != 180 || challenge.ExpiredAt <= time.Now().UnixMilli() {
		t.Fatalf("challenge = %#v", challenge)
	}
	const prefix = "data:image/png;base64,"
	if !strings.HasPrefix(challenge.CaptchaImage, prefix) || strings.Count(challenge.HintText, "·") != challenge.TargetCount-1 {
		t.Fatalf("challenge content = %#v", challenge)
	}
	imageData, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(challenge.CaptchaImage, prefix))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := png.Decode(bytes.NewReader(imageData))
	if err != nil {
		t.Fatal(err)
	}
	if decoded.Bounds().Dx() != 300 || decoded.Bounds().Dy() != 180 {
		t.Fatalf("image bounds = %v", decoded.Bounds())
	}

	answer := storedPointCaptchaAnswer(t, store, challenge.CaptchaID)
	if len(answer.Points) != challenge.TargetCount {
		t.Fatalf("stored target count = %d, challenge target count = %d", len(answer.Points), challenge.TargetCount)
	}
	if ok, err := manager.Verify(t.Context(), "different", challenge.CaptchaID, answer.Points); err != nil || ok {
		t.Fatalf("different user verify = %v, %v", ok, err)
	}
	if ok, err := manager.Verify(t.Context(), "readonly", challenge.CaptchaID, answer.Points); err != nil || ok {
		t.Fatalf("consumed captcha verify = %v, %v", ok, err)
	}

	challenge, err = manager.NewChallenge(t.Context(), "readonly")
	if err != nil {
		t.Fatal(err)
	}
	answer = storedPointCaptchaAnswer(t, store, challenge.CaptchaID)
	wrong := append([]CaptchaPoint(nil), answer.Points...)
	wrong[0].X += manager.tolerance + 1
	if ok, err := manager.Verify(t.Context(), "readonly", challenge.CaptchaID, wrong); err != nil || ok {
		t.Fatalf("wrong point verify = %v, %v", ok, err)
	}

	challenge, err = manager.NewChallenge(t.Context(), "readonly")
	if err != nil {
		t.Fatal(err)
	}
	answer = storedPointCaptchaAnswer(t, store, challenge.CaptchaID)
	if ok, err := manager.Verify(t.Context(), "readonly", challenge.CaptchaID, answer.Points); err != nil || !ok {
		t.Fatalf("correct verify = %v, %v", ok, err)
	}
	if ok, err := manager.Verify(t.Context(), "readonly", "", nil); err != nil || ok {
		t.Fatalf("empty verify = %v, %v", ok, err)
	}
}

func TestPointCaptchaCheckRejectsWrongOrderAndPreservesCorrectAnswer(t *testing.T) {
	store := NewMemoryPointCaptchaStore().(*memoryPointCaptchaStore)
	manager := newPointCaptchaTestManager(t, store)
	answer := pointCaptchaAnswer{
		Purpose:       pointCaptchaPurposeLogin,
		SubjectDigest: usernameDigest("readonly"),
		Points:        []CaptchaPoint{CaptchaPoint{X: 40, Y: 50}, CaptchaPoint{X: 240, Y: 130}},
	}
	encoded, err := json.Marshal(answer)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Put(t.Context(), "login.wrong-order", string(encoded), time.Minute); err != nil {
		t.Fatal(err)
	}
	wrongOrder := []CaptchaPoint{answer.Points[1], answer.Points[0]}
	if verified, err := manager.Check(t.Context(), "readonly", "login.wrong-order", wrongOrder); err != nil || verified {
		t.Fatalf("wrong-order check = %v, %v", verified, err)
	}
	if verified, err := manager.Verify(t.Context(), "readonly", "login.wrong-order", answer.Points); err != nil || verified {
		t.Fatalf("consumed wrong-order captcha = %v, %v", verified, err)
	}

	if err := store.Put(t.Context(), "login.correct-order", string(encoded), time.Minute); err != nil {
		t.Fatal(err)
	}
	if verified, err := manager.Check(t.Context(), "readonly", "login.correct-order", answer.Points); err != nil || !verified {
		t.Fatalf("correct-order check = %v, %v", verified, err)
	}
	if verified, err := manager.Verify(t.Context(), "readonly", "login.correct-order", answer.Points); err != nil || !verified {
		t.Fatalf("preserved correct-order captcha = %v, %v", verified, err)
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
	manager, err := NewPointCaptcha(NewMemoryPointCaptchaStore(), newPointCaptchaTestDictionary(), 1, time.Minute, 240, 140, 20)
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

func TestPointCaptchaRefreshAndStores(t *testing.T) {
	store := NewMemoryPointCaptchaStore().(*memoryPointCaptchaStore)
	manager := newPointCaptchaTestManager(t, store)
	challenge, err := manager.NewChallenge(t.Context(), "readonly")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RefreshPasswordChangeChallenge(t.Context(), challenge.CaptchaID); !errors.Is(err, ErrPointCaptchaNotFound) {
		t.Fatalf("login captcha refreshed as password captcha: %v", err)
	}
	refreshed, err := manager.RefreshChallenge(t.Context(), challenge.CaptchaID)
	if err != nil || refreshed.CaptchaID == challenge.CaptchaID {
		t.Fatalf("refresh = %#v, %v", refreshed, err)
	}
	if _, err := manager.RefreshChallenge(t.Context(), challenge.CaptchaID); !errors.Is(err, ErrPointCaptchaNotFound) {
		t.Fatalf("second refresh error = %v", err)
	}
	if err := store.Put(t.Context(), "expired", `{}`, -time.Second); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Take(t.Context(), "expired"); !errors.Is(err, ErrPointCaptchaNotFound) {
		t.Fatalf("expired error = %v", err)
	}
	if err := store.Put(t.Context(), "login.invalid", `{}`, time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RefreshChallenge(t.Context(), "login.invalid"); !errors.Is(err, ErrPointCaptchaNotFound) {
		t.Fatalf("invalid answer error = %v", err)
	}
}

func TestPointCaptchaPurposeBinding(t *testing.T) {
	manager, err := NewPointCaptcha(NewMemoryPointCaptchaStore(), newPointCaptchaTestDictionary(), 1, time.Minute, 300, 180, 20)
	if err != nil {
		t.Fatal(err)
	}
	challenge, err := manager.NewPasswordChangeChallenge(t.Context(), "password-change:7")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RefreshChallenge(t.Context(), challenge.CaptchaID); !errors.Is(err, ErrPointCaptchaNotFound) {
		t.Fatalf("password captcha refreshed as login captcha: %v", err)
	}
	refreshed, err := manager.RefreshPasswordChangeChallenge(t.Context(), challenge.CaptchaID)
	if err != nil || refreshed.CaptchaID == challenge.CaptchaID {
		t.Fatalf("refresh password captcha: %#v %v", refreshed, err)
	}
}

func TestNewPointCaptchaValidation(t *testing.T) {
	validStore := NewMemoryPointCaptchaStore()
	validDictionary := newPointCaptchaTestDictionary()
	for name, item := range map[string]struct {
		store                    PointCaptchaStore
		dictionary               PointCaptchaDictionary
		threshold                int64
		ttl                      time.Duration
		width, height, tolerance int
	}{
		"store":      {nil, validDictionary, 5, time.Minute, 300, 180, 22},
		"dictionary": {validStore, nil, 5, time.Minute, 300, 180, 22},
		"threshold":  {validStore, validDictionary, 0, time.Minute, 300, 180, 22},
		"ttl":        {validStore, validDictionary, 5, 0, 300, 180, 22},
		"long ttl":   {validStore, validDictionary, 5, 11 * time.Minute, 300, 180, 22},
		"width":      {validStore, validDictionary, 5, time.Minute, 100, 180, 22},
		"height":     {validStore, validDictionary, 5, time.Minute, 300, 100, 22},
		"tolerance":  {validStore, validDictionary, 5, time.Minute, 300, 180, 2},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := NewPointCaptcha(item.store, item.dictionary, item.threshold, item.ttl, item.width, item.height, item.tolerance); err == nil {
				t.Fatal("invalid point captcha configuration was accepted")
			}
		})
	}
}

type failingCaptchaStore struct{ err error }

func (s failingCaptchaStore) Put(_ context.Context, _, _ string, _ time.Duration) error { return s.err }

func (s failingCaptchaStore) Take(_ context.Context, _ string) (string, error) { return "", s.err }

func TestPointCaptchaStoreFailures(t *testing.T) {
	expected := errors.New("store unavailable")
	manager := newPointCaptchaTestManager(t, failingCaptchaStore{err: expected})
	if _, err := manager.NewChallenge(t.Context(), "readonly"); !errors.Is(err, expected) {
		t.Fatalf("challenge error = %v", err)
	}
	if _, err := manager.RefreshChallenge(t.Context(), "login.id"); !errors.Is(err, expected) {
		t.Fatalf("refresh error = %v", err)
	}
	if ok, err := manager.Verify(t.Context(), "readonly", "login.id", make([]CaptchaPoint, pointCaptchaMinTargetCount)); ok || !errors.Is(err, expected) {
		t.Fatalf("verify = %v, %v", ok, err)
	}
}

{{ if .Computed.enable_redis_final -}}
func TestRedisPointCaptchaStore(t *testing.T) {
	client := &fakeRedisChallengeClient{values: make(map[string]string)}
	store := NewRedisPointCaptchaStore(client)
	if err := store.Put(t.Context(), "abc", "answer", time.Minute); err != nil {
		t.Fatal(err)
	}
	if value, err := store.Take(t.Context(), "abc"); err != nil || value != "answer" {
		t.Fatalf("take = %q, %v", value, err)
	}
	if _, err := store.Take(t.Context(), "abc"); !errors.Is(err, ErrPointCaptchaNotFound) {
		t.Fatalf("missing error = %v", err)
	}
	client.err = errors.New("redis unavailable")
	if err := store.Put(t.Context(), "def", "answer", time.Minute); err == nil {
		t.Fatal("put error was ignored")
	}
	if _, err := store.Take(t.Context(), "def"); err == nil {
		t.Fatal("take error was ignored")
	}
}
{{ end -}}
