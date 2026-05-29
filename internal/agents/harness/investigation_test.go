package harness

import (
	"context"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	agenttypes "github.com/asimarora/semantic-intelligence-layer/internal/agents"
	"github.com/asimarora/semantic-intelligence-layer/internal/agents/policy"
	unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
	retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
	unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
	queryservice "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
	accessmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/access"
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

func TestServiceRunInvestigationSummarizesAccessEvidence(t *testing.T) {
	retrievalStore := retrievalmetadata.NewMemoryStore()
	accessStore := accessmetadata.NewMemoryStore()
	runStore := agentmetadata.NewMemoryStore()

	document := retrievalcontracts.Document{
		DocumentID:      "evt-access-001",
		SchemaVersion:   retrievalcontracts.SchemaVersion,
		TenantID:        "default",
		Source:          "aaa_auth",
		SourceEventID:   "raw-access-001",
		SourceKey:       "access:auth:john:req-001:20260505T140000.000000",
		EntityType:      retrievalcontracts.EntityTypeNetworkAccess,
		EventType:       unifiedaccess.EventType,
		OccurredAt:      time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
		IngestedAt:      time.Date(2026, 5, 5, 14, 1, 0, 0, time.UTC),
		Title:           "network access rejected for subscriber john on request req-001 on NAS 192.168.1.1",
		Content:         "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
		SubscriberID:    "john",
		SessionID:       "sess-001",
		RequestID:       "req-001",
		Outcome:         "rejected",
		NASIPAddress:    "192.168.1.1",
		ClientIPAddress: "10.0.0.25",
		RejectReason:    "invalid password",
		Terms:           retrievalcontracts.NormalizeTerms("invalid password", "rejected", "john", "req-001"),
	}
	if err := retrievalStore.Append(context.Background(), document); err != nil {
		t.Fatalf("retrievalStore.Append() error = %v", err)
	}
	if err := accessStore.Append(context.Background(), unifiedaccess.Event{
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
	}); err != nil {
		t.Fatalf("accessStore.Append() error = %v", err)
	}

	retrievalService, err := queryservice.NewRetrievalService(retrievalStore)
	if err != nil {
		t.Fatalf("NewRetrievalService() error = %v", err)
	}
	accessService, err := queryservice.NewAccessService(accessStore)
	if err != nil {
		t.Fatalf("NewAccessService() error = %v", err)
	}

	service, err := NewService(Dependencies{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Runs:      runStore,
		Policy:    policy.NewDefaultEvaluator(),
		Retrieval: retrievalService,
		Access:    accessService,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	record, err := service.RunInvestigation(context.Background(), agenttypes.RunRequest{
		TenantID: "default",
		Goal:     "Investigate why john authentication failed",
		Inputs: map[string]string{
			"query":         "invalid password",
			"subscriber_id": "john",
			"outcome":       "rejected",
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
	if record.Steps[1].ToolName != "query.access.search" {
		t.Fatalf("expected access enrichment step, got %q", record.Steps[1].ToolName)
	}
	if !strings.Contains(record.Steps[2].Summary, "outcome rejected") {
		t.Fatalf("expected access outcome in summary, got %q", record.Steps[2].Summary)
	}
	if !strings.Contains(record.Steps[2].Summary, "invalid password") {
		t.Fatalf("expected reject reason in summary, got %q", record.Steps[2].Summary)
	}
}

func TestServiceRunInvestigationCorrelatesAccessAndSessionTimeline(t *testing.T) {
	retrievalStore := retrievalmetadata.NewMemoryStore()
	accessStore := accessmetadata.NewMemoryStore()
	sessionStore := sessionmetadata.NewMemoryStore()
	runStore := agentmetadata.NewMemoryStore()

	for _, document := range []retrievalcontracts.Document{
		{
			DocumentID:      "evt-access-001",
			SchemaVersion:   retrievalcontracts.SchemaVersion,
			TenantID:        "default",
			Source:          "aaa_auth",
			SourceEventID:   "raw-access-001",
			SourceKey:       "access:auth:john:req-001:20260505T135500.000000",
			EntityType:      retrievalcontracts.EntityTypeNetworkAccess,
			EventType:       unifiedaccess.EventType,
			OccurredAt:      time.Date(2026, 5, 5, 13, 55, 0, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 56, 0, 0, time.UTC),
			Title:           "network access rejected for subscriber john on request req-001 on NAS 192.168.1.1",
			Content:         "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
			SubscriberID:    "john",
			SessionID:       "sess-001",
			RequestID:       "req-001",
			Outcome:         "rejected",
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			RejectReason:    "invalid password",
			Terms:           retrievalcontracts.NormalizeTerms("john invalid password", "rejected", "sess-001"),
		},
		{
			DocumentID:      "evt-session-001",
			SchemaVersion:   retrievalcontracts.SchemaVersion,
			TenantID:        "default",
			Source:          "ras",
			SourceEventID:   "raw-session-001",
			SourceKey:       "radius:acct:john:sess-001:20260505T135820.000000",
			EntityType:      retrievalcontracts.EntityTypeNetworkSession,
			EventType:       unifiedsessions.EventType,
			OccurredAt:      time.Date(2026, 5, 5, 13, 58, 20, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 59, 0, 0, time.UTC),
			Title:           "network session start for subscriber john in session sess-001 on NAS 192.168.1.1",
			Content:         "user john started a network session from NAS 192.168.1.1",
			SubscriberID:    "john",
			SessionID:       "sess-001",
			Status:          "start",
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			Terms:           retrievalcontracts.NormalizeTerms("john connected", "network session", "sess-001"),
		},
	} {
		if err := retrievalStore.Append(context.Background(), document); err != nil {
			t.Fatalf("retrievalStore.Append() error = %v", err)
		}
	}

	for _, event := range []unifiedaccess.Event{
		{
			EventID:         "evt-access-001",
			SourceEventID:   "raw-access-001",
			Source:          "aaa_auth",
			SourceKey:       "access:auth:john:req-001:20260505T135500.000000",
			SchemaVersion:   unifiedaccess.SchemaVersion,
			EventType:       unifiedaccess.EventType,
			TenantID:        "default",
			OccurredAt:      time.Date(2026, 5, 5, 13, 55, 0, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 56, 0, 0, time.UTC),
			SubscriberID:    "john",
			RequestID:       "req-001",
			SessionID:       "sess-001",
			Outcome:         unifiedaccess.OutcomeRejected,
			RejectReason:    "invalid password",
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			SemanticText:    "user john authentication was rejected on NAS 192.168.1.1 from client 10.0.0.25 because invalid password",
		},
		{
			EventID:         "evt-access-002",
			SourceEventID:   "raw-access-002",
			Source:          "aaa_auth",
			SourceKey:       "access:auth:john:req-002:20260505T135800.000000",
			SchemaVersion:   unifiedaccess.SchemaVersion,
			EventType:       unifiedaccess.EventType,
			TenantID:        "default",
			OccurredAt:      time.Date(2026, 5, 5, 13, 58, 0, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 58, 30, 0, time.UTC),
			SubscriberID:    "john",
			RequestID:       "req-002",
			SessionID:       "sess-001",
			Outcome:         unifiedaccess.OutcomeAccepted,
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			SemanticText:    "user john authenticated successfully on NAS 192.168.1.1 from client 10.0.0.25",
		},
	} {
		if err := accessStore.Append(context.Background(), event); err != nil {
			t.Fatalf("accessStore.Append() error = %v", err)
		}
	}

	sessionTime := uint64(100)
	for _, event := range []unifiedsessions.Event{
		{
			EventID:         "evt-session-001",
			SourceEventID:   "raw-session-001",
			Source:          "ras",
			SourceKey:       "radius:acct:john:sess-001:20260505T135820.000000",
			SchemaVersion:   unifiedsessions.SchemaVersion,
			EventType:       unifiedsessions.EventType,
			TenantID:        "default",
			OccurredAt:      time.Date(2026, 5, 5, 13, 58, 20, 0, time.UTC),
			IngestedAt:      time.Date(2026, 5, 5, 13, 59, 0, 0, time.UTC),
			SubscriberID:    "john",
			SessionID:       "sess-001",
			Status:          unifiedsessions.StatusStart,
			NASIPAddress:    "192.168.1.1",
			ClientIPAddress: "10.0.0.25",
			SemanticText:    "user john started a network session from NAS 192.168.1.1",
		},
		{
			EventID:            "evt-session-002",
			SourceEventID:      "raw-session-002",
			Source:             "ras",
			SourceKey:          "radius:acct:john:sess-001:20260505T140000.000000",
			SchemaVersion:      unifiedsessions.SchemaVersion,
			EventType:          unifiedsessions.EventType,
			TenantID:           "default",
			OccurredAt:         time.Date(2026, 5, 5, 14, 0, 0, 0, time.UTC),
			IngestedAt:         time.Date(2026, 5, 5, 14, 1, 0, 0, time.UTC),
			SubscriberID:       "john",
			SessionID:          "sess-001",
			Status:             unifiedsessions.StatusStop,
			NASIPAddress:       "192.168.1.1",
			ClientIPAddress:    "10.0.0.25",
			SessionTimeSeconds: &sessionTime,
			SemanticText:       "user john disconnected from NAS 192.168.1.1 after a 100 second session",
		},
	} {
		if err := sessionStore.Append(context.Background(), event); err != nil {
			t.Fatalf("sessionStore.Append() error = %v", err)
		}
	}

	retrievalService, err := queryservice.NewRetrievalService(retrievalStore)
	if err != nil {
		t.Fatalf("NewRetrievalService() error = %v", err)
	}
	accessService, err := queryservice.NewAccessService(accessStore)
	if err != nil {
		t.Fatalf("NewAccessService() error = %v", err)
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
		Access:    accessService,
		Sessions:  sessionService,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	record, err := service.RunInvestigation(context.Background(), agenttypes.RunRequest{
		TenantID: "default",
		Goal:     "Did john fail auth before connecting?",
		Inputs: map[string]string{
			"query":         "john invalid password then connected",
			"subscriber_id": "john",
		},
	})
	if err != nil {
		t.Fatalf("RunInvestigation() error = %v", err)
	}
	if record.Run.Status != agenttypes.RunStatusCompleted {
		t.Fatalf("expected completed run status, got %q", record.Run.Status)
	}
	if len(record.Steps) != 4 {
		t.Fatalf("expected 4 steps, got %d", len(record.Steps))
	}
	if record.Steps[1].ToolName != "query.access.search" {
		t.Fatalf("expected second step to enrich access events, got %q", record.Steps[1].ToolName)
	}
	if record.Steps[2].ToolName != "query.sessions.search" {
		t.Fatalf("expected third step to enrich sessions, got %q", record.Steps[2].ToolName)
	}
	if !strings.Contains(record.Steps[3].Summary, "later accepted") {
		t.Fatalf("expected correlated acceptance in summary, got %q", record.Steps[3].Summary)
	}
	if !strings.Contains(record.Steps[3].Summary, "started session sess-001") {
		t.Fatalf("expected session start correlation in summary, got %q", record.Steps[3].Summary)
	}
	if !strings.HasPrefix(record.Steps[3].Summary, "Yes.") {
		t.Fatalf("expected direct answer prefix in summary, got %q", record.Steps[3].Summary)
	}

	statusRecord, err := service.RunInvestigation(context.Background(), agenttypes.RunRequest{
		TenantID: "default",
		Goal:     "How is John?",
		Inputs: map[string]string{
			"subscriber_id": "john",
		},
	})
	if err != nil {
		t.Fatalf("RunInvestigation() status question error = %v", err)
	}
	if len(statusRecord.Steps) != 3 {
		t.Fatalf("expected 3 steps for status question, got %d", len(statusRecord.Steps))
	}
	if statusRecord.Steps[0].ToolName != "query.sessions.search" {
		t.Fatalf("expected status question to start with session enrichment, got %q", statusRecord.Steps[0].ToolName)
	}
	if statusRecord.Steps[1].ToolName != "query.access.search" {
		t.Fatalf("expected status question to include access enrichment second, got %q", statusRecord.Steps[1].ToolName)
	}
	if !strings.HasPrefix(statusRecord.Steps[2].Summary, "Subscriber john is not currently connected in the current investigation window.") {
		t.Fatalf("expected concise status summary, got %q", statusRecord.Steps[2].Summary)
	}
	if strings.HasPrefix(statusRecord.Steps[2].Summary, "Investigated goal") {
		t.Fatalf("expected status answer instead of generic summary, got %q", statusRecord.Steps[2].Summary)
	}

	tenantRecord, err := service.RunInvestigation(context.Background(), agenttypes.RunRequest{
		TenantID: "default",
		Goal:     "Does john belong to this tenant?",
		Inputs: map[string]string{
			"subscriber_id": "john",
		},
	})
	if err != nil {
		t.Fatalf("RunInvestigation() tenant question error = %v", err)
	}
	if len(tenantRecord.Steps) != 2 {
		t.Fatalf("expected 2 steps for tenant question, got %d", len(tenantRecord.Steps))
	}
	if tenantRecord.Steps[0].ToolName != "retrieve.evidence.search" {
		t.Fatalf("expected tenant question to stay retrieval-only, got %q", tenantRecord.Steps[0].ToolName)
	}
	if !strings.HasPrefix(tenantRecord.Steps[1].Summary, "Yes. Subscriber john has evidence in this tenant.") {
		t.Fatalf("expected tenant membership answer, got %q", tenantRecord.Steps[1].Summary)
	}
	if strings.HasPrefix(tenantRecord.Steps[1].Summary, "Investigated goal") {
		t.Fatalf("expected tenant answer instead of generic summary, got %q", tenantRecord.Steps[1].Summary)
	}
	if !strings.Contains(tenantRecord.Steps[0].Summary, `query "john subscriber tenant membership"`) {
		t.Fatalf("expected bounded tenant query rewrite, got %q", tenantRecord.Steps[0].Summary)
	}

	unsupportedRecord, err := service.RunInvestigation(context.Background(), agenttypes.RunRequest{
		TenantID: "default",
		Goal:     "Who manages John?",
		Inputs: map[string]string{
			"subscriber_id": "john",
		},
	})
	if err != nil {
		t.Fatalf("RunInvestigation() unsupported question error = %v", err)
	}
	if !strings.HasPrefix(unsupportedRecord.Steps[3].Summary, "I don't have a grounded answer template for that question yet.") {
		t.Fatalf("expected bounded unsupported-question answer, got %q", unsupportedRecord.Steps[3].Summary)
	}
	if strings.HasPrefix(unsupportedRecord.Steps[3].Summary, "Investigated goal") {
		t.Fatalf("expected unsupported-question fallback instead of generic summary, got %q", unsupportedRecord.Steps[3].Summary)
	}
}
