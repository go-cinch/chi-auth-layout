package logging

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGinStartupLogs(t *testing.T) {
	previousLogger, previousMode, previousWriter := slog.Default(), gin.Mode(), gin.DefaultWriter
	t.Cleanup(func() {
		slog.SetDefault(previousLogger)
		gin.SetMode(previousMode)
		gin.DefaultWriter = previousWriter
	})
	gin.SetMode(gin.DebugMode)
	var raw bytes.Buffer
	gin.DefaultWriter = &raw
	var output bytes.Buffer
	if err := Init(&output, "info"); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Next() })
	r.GET("/user/:id", func(c *gin.Context) { c.Status(204) })
	entries := readGinLogs(t, &output)
	if len(entries) != 2 {
		t.Fatalf("logs: %#v", entries)
	}
	warning, route := entries[0], entries[1]
	if warning["level"] != "WARN" || warning["msg"] != "gin: debug mode; use GIN_MODE=release in production" {
		t.Fatalf("warning: %#v", warning)
	}
	if route["level"] != "INFO" || route["msg"] != "register route: GET /user/:id" || route["handlers"] != float64(2) {
		t.Fatalf("route: %#v", route)
	}
	if !strings.Contains(route["handler"].(string), "TestGinStartupLogs") {
		t.Fatalf("handler: %#v", route)
	}
	for _, entry := range entries {
		for _, key := range []string{"detail", "method", "route"} {
			if _, exists := entry[key]; exists {
				t.Fatalf("redundant field %s: %#v", key, entry)
			}
		}
		caller, ok := entry["caller"].(string)
		if !ok || !strings.HasPrefix(caller, "internal/common/logging/gin.go:") {
			t.Fatalf("Gin caller: %#v", entry)
		}
		if _, ok := entry["v"]; !ok {
			t.Fatalf("version missing: %#v", entry)
		}
	}
	if raw.Len() != 0 {
		t.Fatalf("raw Gin output: %s", raw.String())
	}

	// The installed hooks must follow the new slog handler after reinitialization.
	var next bytes.Buffer
	if err := Init(&next, "info"); err != nil {
		t.Fatal(err)
	}
	r.GET("/later", func(c *gin.Context) { c.Status(204) })
	entries = readGinLogs(t, &next)
	if len(entries) != 1 || entries[0]["msg"] != "register route: GET /later" {
		t.Fatalf("new logger: %#v", entries)
	}
	if output.Len() != 0 {
		t.Fatalf("old logger received output: %s", output.String())
	}
}

func TestGinLogLevels(t *testing.T) {
	previousLogger, previousMode := slog.Default(), gin.Mode()
	t.Cleanup(func() { slog.SetDefault(previousLogger); gin.SetMode(previousMode) })
	gin.SetMode(gin.DebugMode)
	for _, test := range []struct {
		level string
		want  int
	}{
		{"debug", 4}, {"info", 3}, {"warn", 2}, {"error", 1},
	} {
		t.Run(test.level, func(t *testing.T) {
			var output bytes.Buffer
			if err := Init(&output, test.level); err != nil {
				t.Fatal(err)
			}
			r := gin.New()
			r.GET("/ready", func(c *gin.Context) { c.Status(204) })
			gin.DebugPrintFunc("loaded template %s\n", "Login.html")
			data := []byte("[GIN-debug] [ERROR] connection failed\n")
			if n, err := gin.DefaultErrorWriter.Write(data); n != len(data) || err != nil {
				t.Fatalf("write: %d %v", n, err)
			}
			if _, err := fmt.Fprint(gin.DefaultErrorWriter, "\n"); err != nil {
				t.Fatal(err)
			}
			entries := readGinLogs(t, &output)
			if len(entries) != test.want {
				t.Fatalf("logs: %#v", entries)
			}
			for _, entry := range entries {
				if _, exists := entry["detail"]; exists {
					t.Fatalf("diagnostic content left outside msg: %#v", entry)
				}
			}
			last := entries[len(entries)-1]
			if last["level"] != "ERROR" || last["msg"] != "gin: connection failed" {
				t.Fatalf("error: %#v", last)
			}
			if test.level == "debug" && entries[2]["msg"] != "gin: loaded template Login.html" {
				t.Fatalf("debug: %#v", entries[2])
			}
		})
	}
}

func TestGinReleaseMode(t *testing.T) {
	previousLogger, previousMode := slog.Default(), gin.Mode()
	t.Cleanup(func() { slog.SetDefault(previousLogger); gin.SetMode(previousMode) })
	gin.SetMode(gin.ReleaseMode)
	var output bytes.Buffer
	if err := Init(&output, "debug"); err != nil {
		t.Fatal(err)
	}
	r := gin.New()
	r.GET("/ready", func(c *gin.Context) { c.Status(204) })
	if output.Len() != 0 {
		t.Fatalf("release emitted debug startup logs: %s", output.String())
	}
}

func readGinLogs(t *testing.T, output *bytes.Buffer) []map[string]any {
	t.Helper()
	decoder := json.NewDecoder(output)
	var entries []map[string]any
	for {
		var entry map[string]any
		err := decoder.Decode(&entry)
		if err == io.EOF {
			return entries
		}
		if err != nil {
			t.Fatalf("invalid JSON log: %v", err)
		}
		entries = append(entries, entry)
	}
}
