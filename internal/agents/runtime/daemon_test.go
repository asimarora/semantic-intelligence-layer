package runtime

import (
        "context"
        "io"
        "log/slog"
        "testing"
        "time"

        agentharness "github.com/asimarora/semantic-intelligence-layer/internal/agents/harness"
        retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
        unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
        queryservice "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
        agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
        retrievalmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
        sessionmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
)

func TestServiceRunCycleTriggersInvestigationAndStoresCheckpoint(t *testing.T) {
        now := time.Date(2026, 5, 21, 14, 30, 0, 0, time.UTC)
        harnessService, runStore, stateStore := buildTestHarness(t, now)

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
                Sessions:     harnessServiceSessions(t, now),
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
        harnessService, _, stateStore := buildTestHarness(t, now)
        sessions := harnessServiceSessions(t, now)

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
                Sessions:     sessions,
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

func buildTestHarness(t *testing.T, now time.Time) (*agentharness.Service, agentmetadata.Store, agentmetadata.WatcherStateStore) {
        t.Helper()

        retrievalStore := retrievalmetadata.NewMemoryStore()
        sessionStore := sessionmetadata.NewMemoryStore()
        runStore := agentmetadata.NewMemoryStore()
        stateStore := agentmetadata.NewMemoryWatcherStateStore()

        document := retrievalcontracts.Document{
                DocumentID:      "evt-stop-003",
                SchemaVersion:   retrievalcontracts.SchemaVersion,
                TenantID:        "default",
                Source:          "ras",
                SourceEventID:   "raw-stop-003",
                SourceKey:       "radius:acct:john:sess-003:20260521T142900.000000",
                EntityType:      retrievalcontracts.EntityTypeNetworkSession,
                EventType:       unifiedsessions.EventType,
                OccurredAt:      now.Add(-time.Minute),
                IngestedAt:      now,
                Title:           "network session stop for subscriber john in session sess-003 on NAS 192.168.1.1",
                Content:         "user john disconnected from NAS 192.168.1.1 after a short session",
                SubscriberID:    "john",
                SessionID:       "sess-003",
                Status:          "stop",
                NASIPAddress:    "192.168.1.1",
                ClientIPAddress: "10.0.0.25",
                Terms:           retrievalcontracts.NormalizeTerms("repeated short session disconnect subscriber john stop", "192.168.1.1"),
        }
        if err := retrievalStore.Append(context.Background(), document); err != nil {
                t.Fatalf("retrievalStore.Append() error = %v", err)
        }

        for _, event := range repeatedDisconnectEvents(now) {
                if err := sessionStore.Append(context.Background(), event); err != nil {
                        t.Fatalf("sessionStore.Append() error = %v", err)
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

        harnessService, err := agentharness.NewService(agentharness.Dependencies{
                Logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
                Runs:      runStore,
                Retrieval: retrievalService,
                Sessions:  sessionService,
        })
        if err != nil {
                t.Fatalf("harness.NewService() error = %v", err)
        }
        return harnessService, runStore, stateStore
}

func harnessServiceSessions(t *testing.T, now time.Time) *queryservice.SessionService {
        t.Helper()

        sessionStore := sessionmetadata.NewMemoryStore()
        for _, event := range repeatedDisconnectEvents(now) {
                if err := sessionStore.Append(context.Background(), event); err != nil {
                        t.Fatalf("sessionStore.Append() error = %v", err)
                }
        }

        sessionService, err := queryservice.NewSessionService(sessionStore)
        if err != nil {
                t.Fatalf("NewSessionService() error = %v", err)
        }
        return sessionService
}

func repeatedDisconnectEvents(now time.Time) []unifiedsessions.Event {
        short := uint64(120)
        return []unifiedsessions.Event{
                testSessionEvent("evt-stop-001", "sess-001", now.Add(-10*time.Minute), &short),
                testSessionEvent("evt-stop-002", "sess-002", now.Add(-5*time.Minute), &short),
                testSessionEvent("evt-stop-003", "sess-003", now.Add(-time.Minute), &short),
        }
}

func testSessionEvent(eventID, sessionID string, occurredAt time.Time, sessionTime *uint64) unifiedsessions.Event {
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
                SubscriberID:       "john",
                SessionID:          sessionID,
                Status:             unifiedsessions.StatusStop,
                NASIPAddress:       "192.168.1.1",
                ClientIPAddress:    "10.0.0.25",
                SessionTimeSeconds: sessionTime,
                SemanticText:       "user john disconnected from NAS 192.168.1.1 after a short session",
        }
}
