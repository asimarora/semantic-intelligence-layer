package ingest

import (
        "context"
        "io"
        "log/slog"
        "os"
        "path/filepath"
        "testing"

        "github.com/asimarora/semantic-intelligence-layer/internal/adapters/radius"
        "github.com/asimarora/semantic-intelligence-layer/internal/platform/messaging"
        rawstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/raw"
        "github.com/nats-io/nats.go"
)

func TestServiceRunPersistsAndPublishesRadiusEvidence(t *testing.T) {
        inputDir := t.TempDir()
        if err := os.WriteFile(filepath.Join(inputDir, "sample.json"), []byte(`{
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
}`), 0o644); err != nil {
                t.Fatalf("write input: %v", err)
        }

        store := rawstore.NewMemoryStore()
        publisher := &stubPublisher{}
        service := Service{
                Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
                Source: radius.Config{
                        ID:            radius.Kind,
                        Kind:          radius.Kind,
                        Enabled:       true,
                        TenantID:      "default",
                        Provider:      "radius-accounting-server",
                        InputPath:     inputDir,
                        SchemaVersion: radius.DefaultSchemaVersion,
                        Subject:       messaging.SubjectRawEvents,
                },
                Store:     store,
                Publisher: publisher,
        }

        result, err := service.Run(context.Background(), "")
        if err != nil {
                t.Fatalf("Run() error = %v", err)
        }

        if result.Files != 1 || result.Persisted != 1 || result.Published != 1 {
                t.Fatalf("unexpected result: %+v", result)
        }

        records := store.Snapshot()
        if len(records) != 1 {
                t.Fatalf("expected 1 stored record, got %d", len(records))
        }
        if records[0].Attributes["username"] != "john" {
                t.Fatalf("expected username attribute, got %#v", records[0].Attributes["username"])
        }

        if len(publisher.subjects) != 1 || publisher.subjects[0] != messaging.SubjectRawEvents {
                t.Fatalf("unexpected published subjects: %#v", publisher.subjects)
        }
        if len(publisher.envelopes) != 1 || publisher.envelopes[0].Source != radius.Source {
                t.Fatalf("unexpected published envelopes: %#v", publisher.envelopes)
        }
}

type stubPublisher struct {
        subjects  []string
        envelopes []messaging.Envelope
}

func (publisher *stubPublisher) PublishEnvelope(_ context.Context, subject string, envelope messaging.Envelope) (*nats.PubAck, error) {
        publisher.subjects = append(publisher.subjects, subject)
        publisher.envelopes = append(publisher.envelopes, envelope)
        return &nats.PubAck{Stream: messaging.DefaultStreamName}, nil
}
