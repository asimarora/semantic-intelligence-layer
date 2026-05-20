package logging

import (
        "bytes"
        "encoding/json"
        "os"
        "path/filepath"
        "testing"
        "time"

        "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
)

func TestNewJSONLoggerIncludesBaseFields(t *testing.T) {
        var buffer bytes.Buffer

        runtime, err := New(
                config.AppConfig{Name: "semantic-intelligence-layer", Env: "testing"},
                config.LoggingConfig{
                        Level:          "info",
                        Format:         "json",
                        AsyncEnabled:   true,
                        AsyncQueueSize: 16,
                        FlushTimeout:   time.Second,
                },
                "backfill",
                &buffer,
        )
        if err != nil {
                t.Fatalf("New returned error: %v", err)
        }

        runtime.Logger.Info("bootstrap ready", "source", "ras")
        if err := runtime.Close(); err != nil {
                t.Fatalf("Close returned error: %v", err)
        }

        var payload map[string]any
        if err := json.Unmarshal(buffer.Bytes(), &payload); err != nil {
                t.Fatalf("unmarshal logger output: %v", err)
        }

        if payload["msg"] != "bootstrap ready" {
                t.Fatalf("expected message field, got %#v", payload["msg"])
        }
        if payload["service"] != "semantic-intelligence-layer" {
                t.Fatalf("expected service field, got %#v", payload["service"])
        }
        if payload["env"] != "testing" {
                t.Fatalf("expected env field, got %#v", payload["env"])
        }
        if payload["component"] != "backfill" {
                t.Fatalf("expected component field, got %#v", payload["component"])
        }
        if payload["source"] != "ras" {
                t.Fatalf("expected source field, got %#v", payload["source"])
        }
}

func TestNewMirrorsLogsToFile(t *testing.T) {
        var buffer bytes.Buffer
        dir := t.TempDir()

        runtime, err := New(
                config.AppConfig{Name: "semantic-intelligence-layer", Env: "testing"},
                config.LoggingConfig{
                        Level:          "info",
                        Format:         "json",
                        AsyncEnabled:   true,
                        AsyncQueueSize: 16,
                        FlushTimeout:   time.Second,
                        OutputDir:      dir,
                },
                "normalizerd",
                &buffer,
        )
        if err != nil {
                t.Fatalf("New returned error: %v", err)
        }

        runtime.Logger.Info("projection ready", "tenant_id", "tenant-a")
        if err := runtime.Close(); err != nil {
                t.Fatalf("Close returned error: %v", err)
        }

        contents, err := os.ReadFile(filepath.Join(dir, "normalizerd.jsonl"))
        if err != nil {
                t.Fatalf("read mirrored log file: %v", err)
        }
        if !bytes.Contains(contents, []byte(`"msg":"projection ready"`)) {
                t.Fatalf("expected mirrored log message, got %s", contents)
        }
}
