package runtime

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	agentharness "github.com/asimarora/semantic-intelligence-layer/internal/agents/harness"
	unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
	retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
	unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
	queryservice "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
	accessmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/access"
	agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
	retrievalmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
	sessionmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
)

type watcherFixture struct {
	retrievalDocuments []retrievalcontracts.Document
	sessionEvents      []unifiedsessions.Event
	accessEvents       []unifiedaccess.Event
}

func TestServiceRunCycleTriggersInvestigationAndStoresCheckpoint(t *testing.T) {
	now := time.Date(2026, 5, 21, 14, 30, 0, 0, time.UTC)
	harnessService, sessionService, accessService, runStore, stateStore := buildTestHarness(t, defaultWatcherFixture(now))

	service, err := NewService(Config{
		Tenants:               []string{"default"},
		PollInterval:          time.Minute,
		Window:                15 * time.Minute,
		MinEvents:             3,
		ShortSessionThreshold: 5 * time.Minute,
		MaxRunsPerCycle:       2,
	}, Dependencies{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Harness:      harnessService,
		Sessions:     sessionService,
		Access:       accessService,
		WatcherState: stateStore,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.RunCycle(context.Background(), now)
	if err != nil {
		t.Fatalf("RunCycle() error = %v", err)
	}
	if result.Candidates != 1 || result.Triggered != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.TriggeredRunIDs) != 1 {
		t.Fatalf("expected 1 triggered run id, got %+v", result.TriggeredRunIDs)
	}

	record, err := runStore.Get(context.Background(), result.TriggeredRunIDs[0])
	if err != nil {
		t.Fatalf("runStore.Get() error = %v", err)
	}
	if record.Run.Status != "completed" {
		t.Fatalf("expected completed run, got %q", record.Run.Status)
	}

	state, err := stateStore.GetWatcherState(context.Background(), watcherStateKey("default", "john"))
	if err != nil {
		t.Fatalf("GetWatcherState() error = %v", err)
	}
	if state.LastEventID != "evt-stop-003" {
		t.Fatalf("expected latest event evt-stop-003, got %q", state.LastEventID)
	}
}

func TestServiceRunCycleSkipsAlreadyProcessedCandidate(t *testing.T) {
	now := time.Date(2026, 5, 21, 14, 30, 0, 0, time.UTC)
	harnessService, sessionService, accessService, _, stateStore := buildTestHarness(t, defaultWatcherFixture(now))

	if err := stateStore.UpsertWatcherState(context.Background(), agentmetadata.WatcherState{
		Key:            watcherStateKey("default", "john"),
		WatcherName:    repeatedShortSessionWatcher,
		TenantID:       "default",
		SubjectID:      "john",
		LastEventID:    "evt-stop-003",
		LastOccurredAt: cloneTime(now.Add(-time.Minute)),
		LastRunID:      "run-existing",
		UpdatedAt:      now,
	}); err != nil {
		t.Fatalf("UpsertWatcherState() error = %v", err)
	}

	service, err := NewService(Config{
		Tenants:               []string{"default"},
		PollInterval:          time.Minute,
		Window:                15 * time.Minute,
		MinEvents:             3,
		ShortSessionThreshold: 5 * time.Minute,
		MaxRunsPerCycle:       2,
	}, Dependencies{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Harness:      harnessService,
		Sessions:     sessionService,
		Access:       accessService,
		WatcherState: stateStore,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.RunCycle(context.Background(), now)
	if err != nil {
		t.Fatalf("RunCycle() error = %v", err)
	}
	if result.Triggered != 0 || result.Skipped != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestServiceRunCyclePrioritizesCrossSourceCorrelation(t *testing.T) {
	now := time.Date(2026, 5, 21, 14, 30, 0, 0, time.UTC)
	harnessService, sessionService, accessService, runStore, stateStore := buildTestHarness(t, crossSourceWatcherFixture(now))

	service, err := NewService(Config{
		Tenants:               []string{"default"},
		PollInterval:          time.Minute,
		Window:                15 * time.Minute,
		MinEvents:             3,
		ShortSessionThreshold: 5 * time.Minute,
		MaxRunsPerCycle:       1,
	}, Dependencies{
		Logger:       slog.New(slog.NewTextHandler(io.Discard, nil)),
		Harness:      harnessService,
		Sessions:     sessionService,
		Access:       accessService,
		WatcherState: stateStore,
	})
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}

	result, err := service.RunCycle(context.Background(), now)
	if err != nil {
		t.Fatalf("RunCycle() error = %v", err)
	}
	if result.Candidates != 2 || result.Triggered != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if len(result.TriggeredRunIDs) != 1 {
		t.Fatalf("expected a single triggered run, got %+v", result.TriggeredRunIDs)
	}

	record, err := runStore.Get(context.Background(), result.TriggeredRunIDs[0])
	if err != nil {
		t.Fatalf("runStore.Get() error = %v", err)
	}
	if record.Run.SessionID != composeWatcherSessionID("default", "john") {
		t.Fatalf("expected correlated subscriber john to be prioritized, got session id %q", record.Run.SessionID)
	}
	if record.Run.Goal != "Did john fail auth before connecting?" {
		t.Fatalf("expected cross-source watcher goal, got %q", record.Run.Goal)
	}
	if len(record.Steps) != 4 {
		t.Fatalf("expected 4 watcher steps for cross-source enrichment, got %d", len(record.Steps))
	}
	if record.Steps[1].ToolName != "query.access.search" {
		t.Fatalf("expected access enrichment step, got %q", record.Steps[1].ToolName)
	}
	if record.Steps[2].ToolName != "query.sessions.search" {
		t.Fatalf("expected session enrichment step, got %q", record.Steps[2].ToolName)
	}
	if !strings.Contains(record.Steps[3].Summary, "Yes. Authentication failed before the subscriber connected.") {
		t.Fatalf("expected direct cross-source answer, got %q", record.Steps[3].Summary)
	}
	if !strings.Contains(record.Steps[3].Summary, "later accepted") {
		t.Fatalf("expected accepted-after-reject correlation, got %q", record.Steps[3].Summary)
	}
	if !strings.Contains(record.Steps[3].Summary, "started session sess-001") {
		t.Fatalf("expected session-start correlation, got %q", record.Steps[3].Summary)
	}

	state, err := stateStore.GetWatcherState(context.Background(), watcherStateKey("default", "john"))
	if err != nil {
		t.Fatalf("GetWatcherState() error = %v", err)
	}
	if state.LastEventID != "evt-john-stop-003" {
		t.Fatalf("expected john latest event checkpoint, got %q", state.LastEventID)
	}
}

func buildTestHarness(t *testing.T, fixture watcherFixture) (*agentharness.Service, *queryservice.SessionService, *queryservice.AccessService, agentmetadata.Store, agentmetadata.WatcherStateStore) {
	t.Helper()

	retrievalStore := retrievalmetadata.NewMemoryStore()
	sessionStore := sessionmetadata.NewMemoryStore()
	accessStore := accessmetadata.NewMemoryStore()
	runStore := agentmetadata.NewMemoryStore()
	stateStore := agentmetadata.NewMemoryWatcherStateStore()

	for _, document := range fixture.retrievalDocuments {
		if err := retrievalStore.Append(context.Background(), document); err != nil {
			t.Fatalf("retrievalStore.Append() error = %v", err)
		}
	}

	for _, event := range fixture.sessionEvents {
		if err := sessionStore.Append(context.Background(), event); err != nil {
			t.Fatalf("sessionStore.Append() error = %v", err)
		}
	}
	for _, event := range fixture.accessEvents {
		if err := accessStore.Append(context.Background(), event); err != nil {
			t.Fatalf("accessStore.Append() error = %v", err)
		}
	}

	retrievalService, err := queryservice.NewRetrievalService(retrievalStore)
	if err != nil {
		t.Fatalf("NewRetrievalService() error = %v", err)
	}
	sessionService, err := queryservice.NewSessionService(sessionStore)
	if err != nil {
		t.Fatalf("NewSessionService() error = %v", err)
	}
	accessService, err := queryservice.NewAccessService(accessStore)
	if err != nil {
		t.Fatalf("NewAccessService() error = %v", err)
	}

	harnessService, err := agentharness.NewService(agentharness.Dependencies{
		Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
		Runs:      runStore,
		Retrieval: retrievalService,
		Access:    accessService,
		Sessions:  sessionService,
	})
	if err != nil {
		t.Fatalf("harness.NewService() error = %v", err)
	}
	return harnessService, sessionService, accessService, runStore, stateStore
}

func defaultWatcherFixture(now time.Time) watcherFixture {
	return watcherFixture{
		retrievalDocuments: []retrievalcontracts.Document{
			testRetrievalDocument("evt-stop-003", "john", "sess-003", now.Add(-time.Minute), "192.168.1.1", "10.0.0.25",
				"repeated short session disconnect", "subscriber john", "stop", "192.168.1.1"),
		},
		sessionEvents: repeatedDisconnectEvents(now),
	}
}

func crossSourceWatcherFixture(now time.Time) watcherFixture {
	sessionEvents := append([]unifiedsessions.Event{}, correlatedWatcherSessionEvents(now)...)
	sessionEvents = append(sessionEvents, janeRepeatedDisconnectEvents(now)...)

	return watcherFixture{
		retrievalDocuments: []retrievalcontracts.Document{
			testRetrievalDocument("evt-john-stop-003", "john", "sess-003", now.Add(-time.Minute), "192.168.1.1", "10.0.0.25",
				"repeated short session disconnect", "subscriber john", "stop", "access authentication", "rejected", "accepted", "correlated"),
			testRetrievalDocument("evt-jane-stop-004", "jane", "sess-104", now.Add(-30*time.Second), "192.168.1.2", "10.0.0.26",
				"repeated short session disconnect", "subscriber jane", "stop"),
		},
		sessionEvents: sessionEvents,
		accessEvents:  correlatedWatcherAccessEvents(now),
	}
}

func repeatedDisconnectEvents(now time.Time) []unifiedsessions.Event {
	short := uint64(120)
	return []unifiedsessions.Event{
		testSessionEvent("evt-stop-001", "john", "sess-001", unifiedsessions.StatusStop, now.Add(-10*time.Minute), "192.168.1.1", "10.0.0.25", &short),
		testSessionEvent("evt-stop-002", "john", "sess-002", unifiedsessions.StatusStop, now.Add(-5*time.Minute), "192.168.1.1", "10.0.0.25", &short),
		testSessionEvent("evt-stop-003", "john", "sess-003", unifiedsessions.StatusStop, now.Add(-time.Minute), "192.168.1.1", "10.0.0.25", &short),
	}
}

func correlatedWatcherSessionEvents(now time.Time) []unifiedsessions.Event {
	short := uint64(120)
	return []unifiedsessions.Event{
		testSessionEvent("evt-john-start-001", "john", "sess-001", unifiedsessions.StatusStart, now.Add(-11*time.Minute), "192.168.1.1", "10.0.0.25", nil),
		testSessionEvent("evt-john-stop-001", "john", "sess-001", unifiedsessions.StatusStop, now.Add(-10*time.Minute), "192.168.1.1", "10.0.0.25", &short),
		testSessionEvent("evt-john-start-002", "john", "sess-002", unifiedsessions.StatusStart, now.Add(-6*time.Minute), "192.168.1.1", "10.0.0.25", nil),
		testSessionEvent("evt-john-stop-002", "john", "sess-002", unifiedsessions.StatusStop, now.Add(-5*time.Minute), "192.168.1.1", "10.0.0.25", &short),
		testSessionEvent("evt-john-stop-003", "john", "sess-003", unifiedsessions.StatusStop, now.Add(-time.Minute), "192.168.1.1", "10.0.0.25", &short),
	}
}

func janeRepeatedDisconnectEvents(now time.Time) []unifiedsessions.Event {
	short := uint64(90)
	return []unifiedsessions.Event{
		testSessionEvent("evt-jane-stop-001", "jane", "sess-101", unifiedsessions.StatusStop, now.Add(-8*time.Minute), "192.168.1.2", "10.0.0.26", &short),
		testSessionEvent("evt-jane-stop-002", "jane", "sess-102", unifiedsessions.StatusStop, now.Add(-4*time.Minute), "192.168.1.2", "10.0.0.26", &short),
		testSessionEvent("evt-jane-stop-003", "jane", "sess-103", unifiedsessions.StatusStop, now.Add(-2*time.Minute), "192.168.1.2", "10.0.0.26", &short),
		testSessionEvent("evt-jane-stop-004", "jane", "sess-104", unifiedsessions.StatusStop, now.Add(-30*time.Second), "192.168.1.2", "10.0.0.26", &short),
	}
}

func correlatedWatcherAccessEvents(now time.Time) []unifiedaccess.Event {
	return []unifiedaccess.Event{
		testAccessEvent("evt-access-reject-001", "john", "req-001", unifiedaccess.OutcomeRejected, now.Add(-11*time.Minute-30*time.Second), "192.168.1.1", "10.0.0.25", "invalid password"),
		testAccessEvent("evt-access-accept-002", "john", "req-002", unifiedaccess.OutcomeAccepted, now.Add(-11*time.Minute-10*time.Second), "192.168.1.1", "10.0.0.25", ""),
	}
}

func testSessionEvent(eventID, subscriberID, sessionID string, status unifiedsessions.Status, occurredAt time.Time, nasIPAddress, clientIPAddress string, sessionTime *uint64) unifiedsessions.Event {
	return unifiedsessions.Event{
		EventID:            eventID,
		SourceEventID:      eventID + ":raw",
		Source:             "ras",
		SourceKey:          "radius:" + sessionID,
		SchemaVersion:      unifiedsessions.SchemaVersion,
		EventType:          unifiedsessions.EventType,
		TenantID:           "default",
		OccurredAt:         occurredAt,
		IngestedAt:         occurredAt.Add(time.Minute),
		SubscriberID:       subscriberID,
		SessionID:          sessionID,
		Status:             status,
		NASIPAddress:       nasIPAddress,
		ClientIPAddress:    clientIPAddress,
		SessionTimeSeconds: sessionTime,
		SemanticText:       fmt.Sprintf("subscriber %s %s on NAS %s", subscriberID, status, nasIPAddress),
	}
}

func testAccessEvent(eventID, subscriberID, requestID string, outcome unifiedaccess.Outcome, occurredAt time.Time, nasIPAddress, clientIPAddress, rejectReason string) unifiedaccess.Event {
	return unifiedaccess.Event{
		EventID:         eventID,
		SourceEventID:   eventID + ":raw",
		Source:          "access",
		SourceKey:       "access:" + requestID,
		SchemaVersion:   unifiedaccess.SchemaVersion,
		EventType:       unifiedaccess.EventType,
		TenantID:        "default",
		OccurredAt:      occurredAt,
		IngestedAt:      occurredAt.Add(time.Minute),
		SubscriberID:    subscriberID,
		RequestID:       requestID,
		Outcome:         outcome,
		RejectReason:    rejectReason,
		NASIPAddress:    nasIPAddress,
		ClientIPAddress: clientIPAddress,
		SemanticText:    fmt.Sprintf("subscriber %s authentication %s", subscriberID, outcome),
	}
}

func testRetrievalDocument(documentID, subscriberID, sessionID string, occurredAt time.Time, nasIPAddress, clientIPAddress string, terms ...string) retrievalcontracts.Document {
	return retrievalcontracts.Document{
		DocumentID:      documentID,
		SchemaVersion:   retrievalcontracts.SchemaVersion,
		TenantID:        "default",
		Source:          "ras",
		SourceEventID:   documentID + ":raw",
		SourceKey:       "source:" + documentID,
		EntityType:      retrievalcontracts.EntityTypeNetworkSession,
		EventType:       unifiedsessions.EventType,
		OccurredAt:      occurredAt,
		IngestedAt:      occurredAt.Add(time.Minute),
		Title:           fmt.Sprintf("network session evidence for subscriber %s", subscriberID),
		Content:         strings.Join(terms, " "),
		SubscriberID:    subscriberID,
		SessionID:       sessionID,
		Status:          "stop",
		NASIPAddress:    nasIPAddress,
		ClientIPAddress: clientIPAddress,
		Terms:           retrievalcontracts.NormalizeTerms(terms...),
	}
}
