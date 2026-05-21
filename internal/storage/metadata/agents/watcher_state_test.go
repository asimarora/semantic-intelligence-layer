package agents

import (
        "context"
        "errors"
        "testing"
        "time"
)

func TestFileWatcherStateStoreUpsertAndGet(t *testing.T) {
        store, err := NewFileWatcherStateStore(t.TempDir())
        if err != nil {
                t.Fatalf("NewFileWatcherStateStore() error = %v", err)
        }

        firstSeen := time.Date(2026, 5, 21, 14, 0, 0, 0, time.UTC)
        secondSeen := firstSeen.Add(5 * time.Minute)

        if err := store.UpsertWatcherState(context.Background(), WatcherState{
                Key:            "watcher:default:john",
                WatcherName:    "repeated-short-session-disconnects",
                TenantID:       "default",
                SubjectID:      "john",
                LastEventID:    "evt-001",
                LastOccurredAt: &firstSeen,
                LastRunID:      "run-001",
                UpdatedAt:      firstSeen,
        }); err != nil {
                t.Fatalf("UpsertWatcherState(first) error = %v", err)
        }

        if err := store.UpsertWatcherState(context.Background(), WatcherState{
                Key:            "watcher:default:john",
                WatcherName:    "repeated-short-session-disconnects",
                TenantID:       "default",
                SubjectID:      "john",
                LastEventID:    "evt-002",
                LastOccurredAt: &secondSeen,
                LastRunID:      "run-002",
                UpdatedAt:      secondSeen,
        }); err != nil {
                t.Fatalf("UpsertWatcherState(second) error = %v", err)
        }

        state, err := store.GetWatcherState(context.Background(), "watcher:default:john")
        if err != nil {
                t.Fatalf("GetWatcherState() error = %v", err)
        }
        if state.LastEventID != "evt-002" {
                t.Fatalf("expected latest event evt-002, got %q", state.LastEventID)
        }
        if state.LastRunID != "run-002" {
                t.Fatalf("expected latest run run-002, got %q", state.LastRunID)
        }
}

func TestMemoryWatcherStateStoreMissing(t *testing.T) {
        store := NewMemoryWatcherStateStore()

        _, err := store.GetWatcherState(context.Background(), "missing")
        if !errors.Is(err, ErrNotFound) {
                t.Fatalf("expected ErrNotFound, got %v", err)
        }
}
