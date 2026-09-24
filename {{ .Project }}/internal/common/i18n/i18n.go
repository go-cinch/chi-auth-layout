// Package i18n localizes public responses. Catalogs are immutable after startup.
package i18n

import (
	"context"
	"embed"
	"encoding/json"

	"golang.org/x/text/language"
)

const English = "en-US"
const Chinese = "zh-CN"

//go:embed locales/*.json
var files embed.FS

var catalogs = loadCatalogs()
var matcher = language.NewMatcher([]language.Tag{language.AmericanEnglish, language.SimplifiedChinese})

type localeKey struct{}

func loadCatalogs() map[string]map[string]string {
	result := make(map[string]map[string]string)
	for _, locale := range []string{English, Chinese} {
		data, err := files.ReadFile("locales/" + locale + ".json")
		if err != nil {
			panic(err)
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			panic(err)
		}
		result[locale] = messages
	}
	return result
}

func Match(accept string) string {
	tags, _, err := language.ParseAcceptLanguage(accept)
	if err != nil {
		return English
	}
	_, index, _ := matcher.Match(tags...)
	if index == 1 {
		return Chinese
	}
	return English
}

func WithLocale(ctx context.Context, locale string) context.Context {
	return context.WithValue(ctx, localeKey{}, locale)
}

func Locale(ctx context.Context, accept string) string {
	if locale, ok := ctx.Value(localeKey{}).(string); ok {
		return locale
	}
	return Match(accept)
}

func Translate(locale, code, fallback string) string {
	if message := catalogs[locale][code]; message != "" {
		return message
	}
	if message := catalogs[English][code]; message != "" {
		return message
	}
	return fallback
}
