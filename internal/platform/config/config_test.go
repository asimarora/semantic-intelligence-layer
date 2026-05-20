package config

import (
        "os"
        "path/filepath"
        "strings"
        "testing"
)

func TestLoadFromMergesOverlayAndEnvOverrides(t *testing.T) {
        t.Setenv("SIL_LOG_LEVEL", "debug")
        t.Setenv("SIL_LOG_ASYNC_QUEUE_SIZE", "2048")
        t.Setenv("SIL_PIPELINE_WORKERS", "11")

        configDir := t.TempDir()
        writeTestFile(t, filepath.Join(configDir, "base.yaml"), `
app:
  name: "semantic-intelligence-layer"
api:
  listen_addr: ":8080"
  read_timeout: "5s"
  write_timeout: "15s"
  idle_timeout: "30s"
logging:
  level: "info"
  format: "text"
  add_source: false
  async_enabled: true
  async_queue_size: 1024
  flush_timeout: "5s"
pipeline:
  workers: 2
  queue_size: 128
  batch_size: 1
ingest:
  source_type: "ras"
  input_path: "./testdata/ras"
sources:
  config_dir: "./configs/sources"
  enabled:
    - "radius"
channels:
  config_dir: "./configs/channels"
  enabled:
    - "chat"
    - "email"
  webhook_base_path: "/v1/channels"
  request_body_limit_bytes: 1048576
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
  stream_name: "SIL_EVENTS"
  storage: "file"
  raw_subject: "sil.raw.events"
  unified_subject: "sil.unified.events"
  index_subject: "sil.index.jobs"
  deadletter_subject: "sil.deadletter"
  consumer_prefix: "sil"
  ack_wait: "30s"
  max_deliveries: 5
  batch_size: 16
  max_ack_pending: 256
  connect_timeout: "5s"
cache:
  backend: "disabled"
`)

        writeTestFile(t, filepath.Join(configDir, "staging.yaml"), `
app:
  env: "staging"
api:
  listen_addr: ":9090"
logging:
  format: "json"
  flush_timeout: "7s"
pipeline:
  queue_size: 1024
channels:
  enabled:
    - "chat"
    - "email"
    - "voice"
    - "messaging"
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
  stream_name: "SIL_EVENTS"
  raw_subject: "sil.raw.events"
  unified_subject: "sil.unified.events"
  index_subject: "sil.index.jobs"
  deadletter_subject: "sil.deadletter"
  consumer_prefix: "sil"
  ack_wait: "45s"
  max_deliveries: 7
  batch_size: 32
  max_ack_pending: 1024
  connect_timeout: "10s"
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
        if cfg.API.ListenAddr != ":9090" {
                t.Fatalf("expected overlay api listen address, got %q", cfg.API.ListenAddr)
        }
        if cfg.Logging.Format != "json" {
                t.Fatalf("expected json logging format, got %q", cfg.Logging.Format)
        }
        if cfg.Logging.Level != "debug" {
                t.Fatalf("expected env override for logging level, got %q", cfg.Logging.Level)
        }
        if !cfg.Logging.AsyncEnabled {
                t.Fatal("expected async logging to be enabled")
        }
        if cfg.Logging.AsyncQueueSize != 2048 {
                t.Fatalf("expected env override for async queue size, got %d", cfg.Logging.AsyncQueueSize)
        }
        if cfg.Logging.FlushTimeout.String() != "7s" {
                t.Fatalf("expected overlay flush timeout 7s, got %s", cfg.Logging.FlushTimeout)
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
        if cfg.Stream.StreamName != "SIL_EVENTS" {
                t.Fatalf("expected stream name SIL_EVENTS, got %q", cfg.Stream.StreamName)
        }
        if cfg.Stream.RawSubject != "sil.raw.events" || cfg.Stream.DeadLetterSubject != "sil.deadletter" {
                t.Fatalf("expected stream subjects to be loaded, got raw=%q dlq=%q", cfg.Stream.RawSubject, cfg.Stream.DeadLetterSubject)
        }
        if cfg.Stream.AckWait.String() != "45s" {
                t.Fatalf("expected stream ack wait 45s, got %s", cfg.Stream.AckWait)
        }
        if cfg.Stream.MaxDeliveries != 7 {
                t.Fatalf("expected max deliveries 7, got %d", cfg.Stream.MaxDeliveries)
        }
        if cfg.Cache.Backend != "redis" {
                t.Fatalf("expected redis cache backend, got %q", cfg.Cache.Backend)
        }
        if len(cfg.Channels.Enabled) != 4 {
                t.Fatalf("expected four enabled channels, got %d", len(cfg.Channels.Enabled))
        }
        if cfg.Sources.ConfigDir != "./configs/sources" {
                t.Fatalf("expected sources config dir to be preserved, got %q", cfg.Sources.ConfigDir)
        }
}

func TestLoadFromSupportsLegacyQAEnvAlias(t *testing.T) {
        configDir := t.TempDir()
        writeTestFile(t, filepath.Join(configDir, "base.yaml"), `
app:
  name: "semantic-intelligence-layer"
api:
  listen_addr: ":8080"
  read_timeout: "5s"
  write_timeout: "15s"
  idle_timeout: "30s"
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
sources:
  config_dir: "./configs/sources"
  enabled:
    - "radius"
channels:
  config_dir: "./configs/channels"
  enabled:
    - "chat"
  webhook_base_path: "/v1/channels"
  request_body_limit_bytes: 1048576
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
  stream_name: "SIL_EVENTS"
  storage: "file"
  raw_subject: "sil.raw.events"
  unified_subject: "sil.unified.events"
  index_subject: "sil.index.jobs"
  deadletter_subject: "sil.deadletter"
  consumer_prefix: "sil"
  ack_wait: "30s"
  max_deliveries: 5
  batch_size: 16
  max_ack_pending: 256
  connect_timeout: "5s"
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
api:
  listen_addr: ":8080"
  read_timeout: "5s"
  write_timeout: "15s"
  idle_timeout: "30s"
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
sources:
  config_dir: "./configs/sources"
  enabled:
    - "radius"
channels:
  config_dir: "./configs/channels"
  enabled:
    - "chat"
  webhook_base_path: "/v1/channels"
  request_body_limit_bytes: 1048576
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
  stream_name: "SIL_EVENTS"
  storage: "file"
  raw_subject: "sil.raw.events"
  unified_subject: "sil.unified.events"
  index_subject: "sil.index.jobs"
  deadletter_subject: "sil.deadletter"
  consumer_prefix: "sil"
  ack_wait: "30s"
  max_deliveries: 5
  batch_size: 16
  max_ack_pending: 256
  connect_timeout: "5s"
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
                        if cfg.API.ListenAddr == "" {
                                t.Fatal("expected api listen address to be set")
                        }
                        if cfg.Logging.Level == "" || cfg.Logging.Format == "" {
                                t.Fatal("expected logging settings to be set")
                        }
                        if cfg.Sources.ConfigDir == "" || len(cfg.Sources.Enabled) == 0 {
                                t.Fatal("expected sources config to be set")
                        }
                        if cfg.Channels.ConfigDir == "" || cfg.Channels.WebhookBasePath == "" {
                                t.Fatal("expected channels config to be set")
                        }
                        if cfg.Stream.Backend == "jetstream" {
                                if cfg.Stream.StreamName == "" || cfg.Stream.RawSubject == "" || cfg.Stream.DeadLetterSubject == "" {
                                   t.Fatal("expected stream topology settings to be set")
                                }
                                if cfg.Stream.AckWait <= 0 || cfg.Stream.MaxDeliveries <= 0 {
                                   t.Fatal("expected stream delivery settings to be positive")
                                }
                        }
                })
        }
}

func TestLoadFromRejectsDuplicateChannels(t *testing.T) {
        configDir := t.TempDir()
        writeTestFile(t, filepath.Join(configDir, "base.yaml"), `
app:
  name: "semantic-intelligence-layer"
api:
  listen_addr: ":8080"
  read_timeout: "5s"
  write_timeout: "15s"
  idle_timeout: "30s"
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
sources:
  config_dir: "./configs/sources"
  enabled:
    - "radius"
channels:
  config_dir: "./configs/channels"
  enabled:
    - "chat"
    - "chat"
  webhook_base_path: "/v1/channels"
  request_body_limit_bytes: 1048576
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
  stream_name: "SIL_EVENTS"
  storage: "file"
  raw_subject: "sil.raw.events"
  unified_subject: "sil.unified.events"
  index_subject: "sil.index.jobs"
  deadletter_subject: "sil.deadletter"
  consumer_prefix: "sil"
  ack_wait: "30s"
  max_deliveries: 5
  batch_size: 16
  max_ack_pending: 256
  connect_timeout: "5s"
cache:
  backend: "disabled"
`)

        _, err := LoadFrom(configDir, "development")
        if err == nil {
                t.Fatal("expected duplicate channel validation error, got nil")
        }
        if !strings.Contains(err.Error(), "channels.enabled must be unique") {
                t.Fatalf("expected duplicate channel validation error, got %v", err)
        }
}

func writeTestFile(t *testing.T, path, content string) {
        t.Helper()
        if err := os.WriteFile(path, []byte(strings.TrimSpace(content)+"\n"), 0o644); err != nil {
                t.Fatalf("write test file %s: %v", path, err)
        }
}
