package logging

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"runtime"
	"strconv"
	"testing"
)

func TestCallerRecordsLogStatement(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	if err := Init(&output, "info"); err != nil {
		t.Fatal(err)
	}
	_, _, line, _ := runtime.Caller(0)
	slog.Info("caller test")
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	want := "internal/common/logging/caller_test.go:" + strconv.Itoa(line+1)
	if entry["caller"] != want {
		t.Fatalf("caller = %v, want %s", entry["caller"], want)
	}
	if _, ok := entry["source"]; ok {
		t.Fatalf("unexpected source object: %#v", entry)
	}
	if entry["v"] == nil || entry["msg"] != "caller test" {
		t.Fatalf("metadata: %#v", entry)
	}
}

func TestCallerSurvivesLoggerAttributesAndGroups(t *testing.T) {
	var output bytes.Buffer
	previous := slog.Default()
	t.Cleanup(func() { slog.SetDefault(previous) })
	if err := Init(&output, "debug"); err != nil {
		t.Fatal(err)
	}
	logger := slog.Default().With("component", "worker").WithGroup("job")
	_, _, line, _ := runtime.Caller(0)
	logger.LogAttrs(t.Context(), slog.LevelDebug, "job started", slog.Int("id", 7), slog.String("source", "queue"))
	var entry map[string]any
	if err := json.Unmarshal(output.Bytes(), &entry); err != nil {
		t.Fatal(err)
	}
	want := "internal/common/logging/caller_test.go:" + strconv.Itoa(line+1)
	if entry["caller"] != want || entry["component"] != "worker" {
		t.Fatalf("log: %#v", entry)
	}
	job, ok := entry["job"].(map[string]any)
	if !ok || job["id"] != float64(7) || job["source"] != "queue" {
		t.Fatalf("job: %#v", entry)
	}
}
