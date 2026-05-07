package config

import (
        "os"
        "path/filepath"
        "strings"
        "testing"
)

func TestLoadFromMergesOverlayAndEnvOverrides(t *testing.T) {
        t.Setenv("SIL_LOG_LEVEL", "debug")
        t.Setenv("SIL_PIPELINE_WORKERS", "11")

        configDir := t.TempDir()
        writeTestFile(t, filepath.Join(configDir, "base.yaml"), `
app:
  name: "semantic-intelligence-layer"
logging:
  level: "info"
  format: "text"
  add_source: false
pipeline:
  workers: 2
  queue_size: 128
  batch_size: 1
ingest:
  source_type: "ras"
  input_path: "./testdata/ras"
storage:
  raw:
    backend: "file"
    path: "./var/raw"
  metadata:
    backend: "memory"
  vector:
    backend: "memory"
stream:
  backend: "disabled"
cache:
  backend: "disabled"
`)

        writeTestFile(t, filepath.Join(configDir, "staging.yaml"), `
app:
  env: "staging"
logging:
  format: "json"
pipeline:
  queue_size: 1024
storage:
  metadata:
    backend: "postgres"
    addr: "postgres://sil-staging:5432"
    database: "sil"
  vector:
    backend: "milvus"
    addr: "milvus:19530"
    collection: "sil_events"
stream:
  backend: "jetstream"
  addr: "nats://nats:4222"
  subject: "sil.raw.events"
cache:
  backend: "redis"
  addr: "redis:6379"
`)

        cfg, err := LoadFrom(configDir, "staging")
        if err != nil {
                t.Fatalf("LoadFrom returned error: %v", err)
        }

        if cfg.App.Env != "staging" {
                t.Fatalf("expected staging env, got %q", cfg.App.Env)
        }
        if cfg.Logging.Format != "json" {
                t.Fatalf("expected json logging format, got %q", cfg.Logging.Format)
        }
        if cfg.Logging.Level != "debug" {
                t.Fatalf("expected env override for logging level, got %q", cfg.Logging.Level)
        }
        if cfg.Pipeline.Workers != 11 {
                t.Fatalf("expected env override for workers, got %d", cfg.Pipeline.Workers)
        }
        if cfg.Pipeline.QueueSize != 1024 {
                t.Fatalf("expected overlay queue size, got %d", cfg.Pipeline.QueueSize)
        }
        if cfg.Storage.Metadata.Backend != "postgres" {
                t.Fatalf("expected postgres metadata backend, got %q", cfg.Storage.Metadata.Backend)
        }
        if cfg.Storage.Vector.Collection != "sil_events" {
                t.Fatalf("expected vector collection sil_events, got %q", cfg.Storage.Vector.Collection)
        }
        if cfg.Stream.Backend != "jetstream" {
                t.Fatalf("expected jetstream stream backend, got %q", cfg.Stream.Backend)
        }
        if cfg.Cache.Backend != "redis" {
                t.Fatalf("expected redis cache backend, got %q", cfg.Cache.Backend)
        }
}

func TestLoadFromSupportsLegacyQAEnvAlias(t *testing.T) {
        configDir := t.TempDir()
        writeTestFile(t, filepath.Join(configDir, "base.yaml"), `
app:
  name: "semantic-intelligence-layer"
logging:
  level: "info"
  format: "text"
pipeline:
  workers: 1
  queue_size: 1
  batch_size: 1
ingest:
  source_type: "ras"
  input_path: "./testdata/ras"
storage:
  raw:
    backend: "file"
    path: "./var/raw"
  metadata:
    backend: "memory"
  vector:
    backend: "memory"
stream:
  backend: "disabled"
cache:
  backend: "disabled"
`)

        writeTestFile(t, filepath.Join(configDir, "staging.yaml"), `
app:
  env: "staging"
`)

        cfg, err := LoadFrom(configDir, "qa")
        if err != nil {
                t.Fatalf("LoadFrom returned error: %v", err)
        }

        if cfg.App.Env != "staging" {
                t.Fatalf("expected staging env after qa alias, got %q", cfg.App.Env)
        }
}

func TestLoadFromRejectsInvalidLoggingFormat(t *testing.T) {
        configDir := t.TempDir()
        writeTestFile(t, filepath.Join(configDir, "base.yaml"), `
app:
  name: "semantic-intelligence-layer"
logging:
  level: "info"
  format: "yaml"
pipeline:
  workers: 1
  queue_size: 1
  batch_size: 1
ingest:
  source_type: "ras"
  input_path: "./testdata/ras"
storage:
  raw:
    backend: "file"
    path: "./var/raw"
  metadata:
    backend: "memory"
  vector:
    backend: "memory"
stream:
  backend: "disabled"
cache:
  backend: "disabled"
`)

        _, err := LoadFrom(configDir, "development")
        if err == nil {
                t.Fatal("expected validation error, got nil")
        }
        if !strings.Contains(err.Error(), "unsupported logging.format") {
                t.Fatalf("expected logging format validation error, got %v", err)
        }
}

func TestRepositoryConfigsLoad(t *testing.T) {
        configDir := filepath.Clean(filepath.Join("..", "..", "..", "configs"))

        tests := []struct {
                env string
        }{
                {env: "development"},
                {env: "testing"},
                {env: "staging"},
                {env: "production"},
                {env: "qa"},
        }

        for _, tc := range tests {
                t.Run(tc.env, func(t *testing.T) {
                        cfg, err := LoadFrom(configDir, tc.env)
                        if err != nil {
                                t.Fatalf("LoadFrom(%q) returned error: %v", tc.env, err)
                        }

                        if cfg.App.Name == "" {
                                t.Fatal("expected app name to be set")
                        }
                        if cfg.Logging.Level == "" || cfg.Logging.Format == "" {
                                t.Fatal("expected logging settings to be set")
                        }
                })
        }
}

func writeTestFile(t *testing.T, path, content string) {
        t.Helper()
        if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
                t.Fatalf("write test file %s: %v", path, err)
        }
}
