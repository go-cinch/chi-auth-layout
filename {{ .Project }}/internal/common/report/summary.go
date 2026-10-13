// Package report defines the stable JSON shape for aggregate management views.
package report

// Summary always returns initialized collections, including for an empty database.
type ManagementSummary struct {
	Total  int64             `json:"total"`
	Counts map[string]int64  `json:"counts"`
	Groups []ManagementGroup `json:"groups"`
	Trend  []ManagementPoint `json:"trend"`
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
