package radius

import (
        "os"
        "path/filepath"
        "testing"
        "time"
)

func TestLoadConfigAndAdapt(t *testing.T) {
        configDir := t.TempDir()
        if err := os.WriteFile(filepath.Join(configDir, "radius.yaml"), []byte(`
enabled: true
tenant_id: "default"
provider: "radius-accounting-server"
input_path: "./testdata/ras"
`), 0o644); err != nil {
                t.Fatalf("write config: %v", err)
        }

        cfg, err := LoadConfig(configDir)
        if err != nil {
                t.Fatalf("LoadConfig() error = %v", err)
        }

        if cfg.ID != Kind {
                t.Fatalf("expected id %q, got %q", Kind, cfg.ID)
        }
        if cfg.Subject != "sil.raw.events" {
                t.Fatalf("expected default subject, got %q", cfg.Subject)
        }
        if cfg.SchemaVersion != DefaultSchemaVersion {
                t.Fatalf("expected default schema version, got %q", cfg.SchemaVersion)
        }

        ingestedAt := time.Date(2026, 5, 19, 8, 0, 0, 0, time.UTC)
        adapted, err := Adapt(cfg, []byte(`{
  "username": "john",
  "nas_ip_address": "192.168.1.1",
  "nas_port": 42,
  "acct_status_type": "Accounting-Stop",
  "acct_session_id": "sess-001",
  "framed_ip_address": "10.10.10.1",
  "calling_station_id": "00-11-22-33-44-55",
  "called_station_id": "bbng-01",
  "timestamp": "2026-05-05T14:00:00Z",
  "client_ip": "10.0.0.25",
  "packet_type": "Accounting-Request",
  "acct_input_octets": 12000,
  "acct_output_octets": 54000,
  "acct_session_time": 100
}`), ingestedAt)
        if err != nil {
                t.Fatalf("Adapt() error = %v", err)
        }

        if adapted.Record.Source != Source {
                t.Fatalf("expected source %q, got %q", Source, adapted.Record.Source)
        }
        if adapted.Record.EventID != "ras:sess-001:2026-05-05T14:00:00Z" {
                t.Fatalf("unexpected event id %q", adapted.Record.EventID)
        }
        if adapted.Record.SourceKey != "radius:acct:john:sess-001:20260505T140000.000000" {
                t.Fatalf("unexpected source key %q", adapted.Record.SourceKey)
        }
        if adapted.Record.IngestedAt != ingestedAt {
                t.Fatalf("expected ingested_at %s, got %s", ingestedAt, adapted.Record.IngestedAt)
        }
        if adapted.Record.Attributes["acct_session_id"] != "sess-001" {
                t.Fatalf("expected session attribute, got %#v", adapted.Record.Attributes["acct_session_id"])
        }
        if adapted.Envelope.PartitionKey != "subscriber:john" {
                t.Fatalf("unexpected partition key %q", adapted.Envelope.PartitionKey)
        }
}
