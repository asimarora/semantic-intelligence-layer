package accessnormalize

import (
        "bytes"
        "context"
        "encoding/json"
        "io"
        "log/slog"
        "testing"
        "time"

        rawevidence "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/evidence"
        unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
        "github.com/asimarora/semantic-intelligence-layer/internal/platform/messaging"
        accessmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/access"
        rawstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/raw"
        "github.com/nats-io/nats.go"
)

func TestServiceRunNormalizesAndPublishes(t *testing.T) {
        store := rawstore.NewMemoryStore()
        if err := store.Append(context.Background(), testAccessRecord()); err != nil {
                t.Fatalf("Append() error = %v", err)
        }

        projectionStore := accessmetadata.NewMemoryStore()
        publisher := &stubPublisher{}
        var output bytes.Buffer
        service := Service{
                Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
                Store:           store,
                ProjectionStore: projectionStore,
                Publisher:       publisher,
                UnifiedSubject:  messaging.SubjectUnifiedEvents,
        }

        result, err := service.Run(context.Background(), &output)
        if err != nil {
                t.Fatalf("Run() error = %v", err)
        }
        if result.Records != 1 || result.Projected != 1 || result.Published != 1 {
                t.Fatalf("unexpected result: %+v", result)
        }

        var event unifiedaccess.Event
        if err := json.Unmarshal(bytes.TrimSpace(output.Bytes()), &event); err != nil {
                t.Fatalf("Unmarshal() error = %v", err)
        }
        if event.Outcome != unifiedaccess.OutcomeRejected {
                t.Fatalf("unexpected outcome %q", event.Outcome)
        }
        if len(publisher.envelopes) != 1 {
                t.Fatalf("expected 1 published envelope, got %d", len(publisher.envelopes))
        }
        if publisher.envelopes[0].PartitionKey != "subscriber:john" {
                t.Fatalf("unexpected partition key %q", publisher.envelopes[0].PartitionKey)
        }

        projected, err := projectionStore.Search(context.Background(), accessmetadata.Query{
                TenantID:     "default",
                SubscriberID: "john",
        })
        if err != nil {
                t.Fatalf("Search() error = %v", err)
        }
        if len(projected) != 1 {
                t.Fatalf("expected 1 projected event, got %d", len(projected))
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

func testAccessRecord() rawevidence.Record {
        now := time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC)
        return rawevidence.Record{
                EventID:       "aaa_auth:req-001:2026-05-05T14:00:00Z",
                Source:        "aaa_auth",
                SourceKey:     "access:auth:john:req-001:20260505T140000.000000",
                SchemaVersion: "sil.evidence.access.v1",
                TenantID:      "default",
                OccurredAt:    now,
                IngestedAt:    now,
                Attributes: map[string]string{
                        "username": "john",
                },
                Payload: json.RawMessage(`{
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
}
