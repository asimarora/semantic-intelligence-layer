package index

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"testing"
	"time"

	unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
	retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
	queryservice "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
	retrievalmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
)

func TestServiceRunIndexesAccessEvents(t *testing.T) {
	store := retrievalmetadata.NewMemoryStore()
	service := Service{
		Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Store:  store,
	}

	event := unifiedaccess.Event{
		EventID:         "evt-access-001",
		SourceEventID:   "raw-access-001",
		Source:          "aaa_auth",
		SourceKey:       "access:auth:john:req-001:20260505T140000.000000",
		SchemaVersion:   unifiedaccess.SchemaVersion,
		EventType:       unifiedaccess.EventType,
		TenantID:        "default",
		OccurredAt:      time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
		IngestedAt:      time.Date(2026, 5, 5, 14, 1, 0, 0, time.UTC),
		SubscriberID:    "john",
		RequestID:       "req-001",
		SessionID:       "sess-001",
		Outcome:         unifiedaccess.OutcomeRejected,
		RejectReason:    "invalid password",
		NASIPAddress:    "192.168.1.1",
		ClientIPAddress: "10.0.0.25",
		SemanticText:    "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
	}

	data, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}

	result, err := service.Run(context.Background(), bytes.NewReader(append(data, '\n')))
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.Events != 1 || result.Indexed != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}

	retrievalService, err := queryservice.NewRetrievalService(store)
	if err != nil {
		t.Fatalf("NewRetrievalService() error = %v", err)
	}

	hits, err := retrievalService.SearchEvidence(context.Background(), retrievalmetadata.Query{
		TenantID:  "default",
		QueryText: "invalid password",
		Outcome:   "rejected",
	})
	if err != nil {
		t.Fatalf("SearchEvidence() error = %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("expected 1 hit, got %d", len(hits))
	}
	if hits[0].Document.EntityType != retrievalcontracts.EntityTypeNetworkAccess {
		t.Fatalf("expected access entity type, got %q", hits[0].Document.EntityType)
	}
	if hits[0].Document.RequestID != "req-001" {
		t.Fatalf("expected request req-001, got %q", hits[0].Document.RequestID)
	}
}
