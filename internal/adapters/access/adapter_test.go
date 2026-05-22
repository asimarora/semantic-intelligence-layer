package access

import (
        "os"
        "path/filepath"
        "testing"
        "time"

        rawevidence "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/evidence"
        unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
)

func TestLoadConfigAndAdapt(t *testing.T) {
        configDir := t.TempDir()
        if err := os.WriteFile(filepath.Join(configDir, "access.yaml"), []byte(`
enabled: true
tenant_id: "default"
provider: "aaa-access-gateway"
input_path: "./testdata/access"
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
  "request_id": "req-001",
  "decision": "Access-Reject",
  "reject_reason": "invalid password",
  "nas_ip_address": "192.168.1.1",
  "client_ip": "10.0.0.25",
  "packet_type": "Access-Request",
  "auth_protocol": "pap",
  "acct_session_id": "sess-001",
  "timestamp": "2026-05-05T14:00:00Z"
}`), ingestedAt)
        if err != nil {
                t.Fatalf("Adapt() error = %v", err)
        }

        if adapted.Record.Source != Source {
                t.Fatalf("expected source %q, got %q", Source, adapted.Record.Source)
        }
        if adapted.Record.SourceKey != "access:auth:john:req-001:20260505T140000.000000" {
                t.Fatalf("unexpected source key %q", adapted.Record.SourceKey)
        }
        if adapted.Record.IngestedAt != ingestedAt {
                t.Fatalf("expected ingested_at %s, got %s", ingestedAt, adapted.Record.IngestedAt)
        }
        if adapted.Record.Attributes["decision"] != "Access-Reject" {
                t.Fatalf("expected decision attribute, got %#v", adapted.Record.Attributes["decision"])
        }
        if adapted.Envelope.PartitionKey != "subscriber:john" {
                t.Fatalf("unexpected partition key %q", adapted.Envelope.PartitionKey)
        }
}

func TestNormalizeRecord(t *testing.T) {
        now := time.Date(2026, 5, 19, 8, 0, 0, 0, time.UTC)
        record := rawevidence.Record{
                EventID:       "aaa_auth:req-001:2026-05-05T14:00:00Z",
                Source:        Source,
                SourceKey:     "access:auth:john:req-001:20260505T140000.000000",
                SchemaVersion: DefaultSchemaVersion,
                TenantID:      "default",
                OccurredAt:    time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
                IngestedAt:    now,
                Attributes: map[string]string{
                        "username": "john",
                },
                Payload: []byte(`{
  "username": "john",
  "request_id": "req-001",
  "decision": "Access-Reject",
  "reject_reason": "invalid password",
  "nas_ip_address": "192.168.1.1",
  "client_ip": "10.0.0.25",
  "packet_type": "Access-Request",
  "auth_protocol": "pap",
  "acct_session_id": "sess-001",
  "timestamp": "2026-05-05T14:00:00Z"
}`),
        }

        normalized, err := NormalizeRecord(record)
        if err != nil {
                t.Fatalf("NormalizeRecord() error = %v", err)
        }

        if normalized.SourceEventID != record.EventID {
                t.Fatalf("unexpected source event id %q", normalized.SourceEventID)
        }
        if normalized.SchemaVersion != unifiedaccess.SchemaVersion {
                t.Fatalf("unexpected schema version %q", normalized.SchemaVersion)
        }
        if normalized.Outcome != unifiedaccess.OutcomeRejected {
                t.Fatalf("unexpected outcome %q", normalized.Outcome)
        }
        if normalized.EventType != unifiedaccess.EventType {
                t.Fatalf("unexpected event type %q", normalized.EventType)
        }
        if normalized.SemanticText != "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 using pap because invalid password" {
                t.Fatalf("unexpected semantic text %q", normalized.SemanticText)
        }
}
