package report

import (
	"encoding/json"
	"testing"
)

func TestSummaryStableEmptyShape(t *testing.T) {
	value := New("enabled", "disabled")
	encoded, err := json.Marshal(value)
	if err != nil || string(encoded) != `{"counts":{"disabled":0,"enabled":0},"groups":[],"total":0,"trend":[]}` {
		t.Fatalf("%s: %v", encoded, err)
	}
}

func TestManagementPointZeroLoginSeriesIsPresent(t *testing.T) {
	encoded, err := json.Marshal(ManagementPoint{Date: "2026-10-13", From: 1, To: 2})
	if err != nil || string(encoded) != `{"date":"2026-10-13","from":1,"to":2,"count":0,"recent_logged_in_users":0}` {
		t.Fatalf("point: %s %v", encoded, err)
	}
}
