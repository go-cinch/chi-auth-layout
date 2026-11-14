package dictionary

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"
)

const TimezoneDictionaryKey = "TIMEZONE_OPTIONS"

type TimezoneOption struct {
	Value   string            `json:"value"`
	Name    map[string]string `json:"name"`
	Region  string            `json:"region"`
	Sort    int               `json:"sort"`
	Enabled bool              `json:"enabled"`
}

type TimezoneCatalog struct {
	Items []TimezoneOption `json:"items"`
}

// ParseTimezoneCatalog validates stored identifiers, never accepting a display offset as a zone.
func ParseTimezoneCatalog(raw []byte) (TimezoneCatalog, error) {
	var value TimezoneCatalog
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&value); err != nil {
		return value, err
	}
	if decoder.Decode(new(any)) != io.EOF || value.Items == nil {
		return value, errors.New("invalid timezone catalog")
	}
	seen := map[string]bool{}
	for _, item := range value.Items {
		if item.Value == "" || item.Value == "Local" || seen[item.Value] {
			return value, errors.New("invalid or duplicate timezone")
		}
		if _, err := time.LoadLocation(item.Value); err != nil {
			return value, err
		}
		if len(item.Name) != 2 || strings.TrimSpace(item.Name["zh-CN"]) == "" || strings.TrimSpace(item.Name["en-US"]) == "" {
			return value, errors.New("timezone names are required")
		}
		switch item.Region {
		case "Asia", "Europe", "Americas", "Oceania", "Africa", "Other":
		default:
			return value, errors.New("invalid timezone region")
		}
		seen[item.Value] = true
	}
	return value, nil
}

// availableTimezones filters disabled options and uses dictionary-defined ordering.
func availableTimezones(value TimezoneCatalog) TimezoneCatalog {
	result := TimezoneCatalog{Items: make([]TimezoneOption, 0, len(value.Items))}
	for _, item := range value.Items {
		if item.Enabled {
			result.Items = append(result.Items, item)
		}
	}
	sort.SliceStable(result.Items, func(i, j int) bool {
		a, b := result.Items[i], result.Items[j]
		return a.Sort < b.Sort
	})
	return result
}

// TimezoneOptions reads the current dictionary on each picker open.
func (m *Module) TimezoneOptions(ctx context.Context) (TimezoneCatalog, error) {
	raw, err := m.requiredValue(ctx, TimezoneDictionaryKey)
	if err != nil {
		return TimezoneCatalog{}, err
	}
	value, err := ParseTimezoneCatalog(raw)
	if err != nil {
		return TimezoneCatalog{}, fmt.Errorf("dictionary %s: %w", TimezoneDictionaryKey, err)
	}
	return availableTimezones(value), nil
}
