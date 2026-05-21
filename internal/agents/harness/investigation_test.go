package harness

import (
        "context"
        "io"
        "log/slog"
        "testing"
        "time"

        agenttypes "github.com/asimarora/semantic-intelligence-layer/internal/agents"
        "github.com/asimarora/semantic-intelligence-layer/internal/agents/policy"
        retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
        unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
        queryservice "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
        agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
        retrievalmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
        sessionmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
)

func TestServiceRunInvestigationProducesCompletedRun(t *testing.T) {
        retrievalStore := retrievalmetadata.NewMemoryStore()
        sessionStore := sessionmetadata.NewMemoryStore()
        runStore := agentmetadata.NewMemoryStore()

        document := retrievalcontracts.Document{
                DocumentID:      "evt-001",
                SchemaVersion:   retrievalcontracts.SchemaVersion,
                TenantID:        "default",
                Source:          "ras",
                SourceEventID:   "raw-001",
                SourceKey:       "radius:acct:john:sess-001:20260505T140000.000000",
                EntityType:      retrievalcontracts.EntityTypeNetworkSession,
                EventType:       unifiedsessions.EventType,
                OccurredAt:      time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
                IngestedAt:      time.Date(2026, 5, 5, 14, 1, 0, 0, time.UTC),
                Title:           "network session stop for subscriber john in session sess-001 on NAS 192.168.1.1",
                Content:         "user john disconnected from NAS 192.168.1.1 after a 100 second session",
                SubscriberID:    "john",
                SessionID:       "sess-001",
                Status:          "stop",
                NASIPAddress:    "192.168.1.1",
                ClientIPAddress: "10.0.0.25",
                Terms:           retrievalcontracts.NormalizeTerms("john disconnected", "sess-001", "192.168.1.1"),
        }
        if err := retrievalStore.Append(context.Background(), document); err != nil {
                t.Fatalf("retrievalStore.Append() error = %v", err)
        }

        sessionEvent := unifiedsessions.Event{
                EventID:         "sess-evt-001",
                SourceEventID:   "raw-001",
                Source:          "ras",
                SourceKey:       "radius:acct:john:sess-001:20260505T140000.000000",
                SchemaVersion:   unifiedsessions.SchemaVersion,
                EventType:       unifiedsessions.EventType,
                TenantID:        "default",
                OccurredAt:      time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
                IngestedAt:      time.Date(2026, 5, 5, 14, 1, 0, 0, time.UTC),
                SubscriberID:    "john",
                SessionID:       "sess-001",
                Status:          unifiedsessions.StatusStop,
                NASIPAddress:    "192.168.1.1",
                ClientIPAddress: "10.0.0.25",
                SemanticText:    "user john disconnected from NAS 192.168.1.1 after a 100 second session",
        }
        if err := sessionStore.Append(context.Background(), sessionEvent); err != nil {
                t.Fatalf("sessionStore.Append() error = %v", err)
        }

        retrievalService, err := queryservice.NewRetrievalService(retrievalStore)
        if err != nil {
                t.Fatalf("NewRetrievalService() error = %v", err)
        }
        sessionService, err := queryservice.NewSessionService(sessionStore)
        if err != nil {
                t.Fatalf("NewSessionService() error = %v", err)
        }

        service, err := NewService(Dependencies{
                Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
                Runs:      runStore,
                Policy:    policy.NewDefaultEvaluator(),
                Retrieval: retrievalService,
                Sessions:  sessionService,
        })
        if err != nil {
                t.Fatalf("NewService() error = %v", err)
        }

        record, err := service.RunInvestigation(context.Background(), agenttypes.RunRequest{
                TenantID: "default",
                Goal:     "Investigate why john disconnected",
                Inputs: map[string]string{
                        "query":         "john disconnected",
                        "subscriber_id": "john",
                },
        })
        if err != nil {
                t.Fatalf("RunInvestigation() error = %v", err)
        }
        if record.Run.Status != agenttypes.RunStatusCompleted {
                t.Fatalf("expected completed run status, got %q", record.Run.Status)
        }
        if len(record.Steps) != 3 {
                t.Fatalf("expected 3 steps, got %d", len(record.Steps))
        }
        if record.Steps[0].ToolName != "retrieve.evidence.search" {
                t.Fatalf("unexpected first tool %q", record.Steps[0].ToolName)
        }
        if record.Steps[2].Kind != "explain" {
                t.Fatalf("expected final explain step, got %q", record.Steps[2].Kind)
        }
}
