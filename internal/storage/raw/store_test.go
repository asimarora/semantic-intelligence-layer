package raw

import (
        "context"
        "encoding/json"
        "os"
        "path/filepath"
        "strings"
        "testing"
        "time"

        rawevidence "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/evidence"
)

func TestFileStoreAppendWritesJSONL(t *testing.T) {
        store, err := NewFileStore(t.TempDir())
        if err != nil {
                t.Fatalf("NewFileStore() error = %v", err)
        }

        record := testRecord()
        if err := store.Append(context.Background(), record); err != nil {
                t.Fatalf("Append() error = %v", err)
        }

        data, err := os.ReadFile(filepath.Join(store.root, "ras.jsonl"))
        if err != nil {
                t.Fatalf("ReadFile() error = %v", err)
        }

        lines := strings.Split(strings.TrimSpace(string(data)), "\n")
        if len(lines) != 1 {
                t.Fatalf("expected 1 line, got %d", len(lines))
        }

        var decoded rawevidence.Record
        if err := json.Unmarshal([]byte(lines[0]), &decoded); err != nil {
                t.Fatalf("Unmarshal() error = %v", err)
        }
        if decoded.SourceKey != record.SourceKey {
                t.Fatalf("expected source key %q, got %q", record.SourceKey, decoded.SourceKey)
        }
}

func TestMemoryStoreSnapshotReturnsCopies(t *testing.T) {
        store := NewMemoryStore()
        if err := store.Append(context.Background(), testRecord()); err != nil {
                t.Fatalf("Append() error = %v", err)
        }

        snapshot := store.Snapshot()
        if len(snapshot) != 1 {
                t.Fatalf("expected 1 record, got %d", len(snapshot))
        }

        snapshot[0].Attributes["username"] = "changed"
        next := store.Snapshot()
        if next[0].Attributes["username"] != "john" {
                t.Fatalf("expected snapshot copy isolation, got %q", next[0].Attributes["username"])
        }
}

func testRecord() rawevidence.Record {
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
                Payload: json.RawMessage(`{"username":"john","acct_session_id":"sess-001"}`),
        }
}
