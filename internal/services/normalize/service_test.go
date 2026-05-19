package normalize

import (
        "bytes"
        "context"
        "encoding/json"
        "io"
        "log/slog"
        "testing"
        "time"

        rawevidence "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/evidence"
        unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
        "github.com/asimarora/semantic-intelligence-layer/internal/platform/messaging"
        rawstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/raw"
        "github.com/nats-io/nats.go"
)

func TestServiceRunNormalizesAndPublishes(t *testing.T) {
        store := rawstore.NewMemoryStore()
        if err := store.Append(context.Background(), testRadiusRecord()); err != nil {
                t.Fatalf("Append() error = %v", err)
        }

        publisher := &stubPublisher{}
        var output bytes.Buffer
        service := Service{
                Logger:         slog.New(slog.NewTextHandler(io.Discard, nil)),
                Store:          store,
                Publisher:      publisher,
                UnifiedSubject: messaging.SubjectUnifiedEvents,
        }

        result, err := service.Run(context.Background(), &output)
        if err != nil {
                t.Fatalf("Run() error = %v", err)
        }
        if result.Records != 1 || result.Published != 1 {
                t.Fatalf("unexpected result: %+v", result)
        }

        var event unifiedsessions.Event
        if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &event); err != nil {
                t.Fatalf("Unmarshal() error = %v", err)
        }
        if event.Status != unifiedsessions.StatusStop {
                t.Fatalf("unexpected status %q", event.Status)
        }
        if len(publisher.envelopes) != 1 {
                t.Fatalf("expected 1 published envelope, got %d", len(publisher.envelopes))
        }
        if publisher.envelopes[0].PartitionKey != "subscriber:john" {
                t.Fatalf("unexpected partition key %q", publisher.envelopes[0].PartitionKey)
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

func testRadiusRecord() rawevidence.Record {
        now := time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC)
        return rawevidence.Record{
                EventID:       "ras:sess-001:2026-05-05T14:00:00Z",
                Source:        "ras",
                SourceKey:     "radius:acct:john:sess-001:20260505T140000.000000",
                SchemaVersion: "sil.evidence.radius.v1",
                TenantID:      "default",
                OccurredAt:    now,
                IngestedAt:    now,
                Attributes: map[string]string{
                        "username": "john",
                },
                Payload: json.RawMessage(`{
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
}`),
        }
}
