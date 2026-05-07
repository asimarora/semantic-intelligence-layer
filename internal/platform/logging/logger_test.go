package logging

import (
        "bytes"
        "encoding/json"
        "testing"

        "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
)

func TestNewJSONLoggerIncludesBaseFields(t *testing.T) {
        var buffer bytes.Buffer

        logger, err := New(
                config.AppConfig{Name: "semantic-intelligence-layer", Env: "testing"},
                config.LoggingConfig{Level: "info", Format: "json"},
                "backfill",
                &buffer,
        )
        if err != nil {
                t.Fatalf("New returned error: %v", err)
        }

        logger.Info("bootstrap ready", "source", "ras")

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
