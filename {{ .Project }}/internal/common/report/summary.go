// Package report defines the stable JSON shape for aggregate management views.
package report

import "encoding/json"

// ManagementSummary returns initialized requested collections, even for an empty database.
type ManagementSummary struct {
	Fields Fields            `json:"-"`
	Total  int64             `json:"total,omitempty"`
	Counts map[string]int64  `json:"counts,omitempty"`
	Groups []ManagementGroup `json:"groups,omitempty"`
	Trend  []ManagementPoint `json:"trend,omitempty"`
}

type ManagementGroup struct {
	ID    int64  `json:"id,omitempty"`
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

// Point represents the half-open interval [From, To) of a calendar bucket.
type ManagementPoint struct {
	Date                string `json:"date"`
	From                int64  `json:"from"`
	To                  int64  `json:"to"`
	Count               int64  `json:"count"`
	RecentLoggedInUsers int64  `json:"recent_logged_in_users"`
}

func New(keys ...string) *ManagementSummary {
	value := &ManagementSummary{Counts: make(map[string]int64, len(keys)), Groups: []ManagementGroup{}, Trend: []ManagementPoint{}}
	for _, key := range keys {
		value.Counts[key] = 0
	}
	return value
}

func (s ManagementSummary) MarshalJSON() ([]byte, error) {
	value := map[string]any{}
	if s.Fields.Has("total") {
		value["total"] = s.Total
	}
	if s.Fields.Has("counts") {
		value["counts"] = s.Counts
	}
	if s.Fields.Has("groups") {
		value["groups"] = s.Groups
	}
	if s.Fields.Has("trend") {
		value["trend"] = s.Trend
	}
	return json.Marshal(value)
}
