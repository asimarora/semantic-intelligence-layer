package raw

import (
        "context"
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "sync"

        rawevidence "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/evidence"
        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
)

type Store interface {
        Append(context.Context, rawevidence.Record) error
}

func NewStore(cfg silconfig.StoreConfig) (Store, error) {
        switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
        case "file":
                return NewFileStore(cfg.Path)
        case "memory":
                return NewMemoryStore(), nil
        default:
                return nil, fmt.Errorf("raw storage backend %q is not implemented", cfg.Backend)
        }
}

type FileStore struct {
        root string
        mu   sync.Mutex
}

func NewFileStore(root string) (*FileStore, error) {
        root = strings.TrimSpace(root)
        if root == "" {
                return nil, fmt.Errorf("raw storage path is required")
        }
        return &FileStore{root: filepath.Clean(root)}, nil
}

func (store *FileStore) Append(ctx context.Context, record rawevidence.Record) error {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return err
        }
        if err := record.Validate(); err != nil {
                return err
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        if err := os.MkdirAll(store.root, 0o755); err != nil {
                return fmt.Errorf("create raw storage root %s: %w", store.root, err)
        }

        path := filepath.Join(store.root, sourceFileName(record.Source))
        file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
        if err != nil {
                return fmt.Errorf("open raw storage file %s: %w", path, err)
        }
        defer file.Close()

        encoder := json.NewEncoder(file)
        encoder.SetEscapeHTML(false)
        if err := encoder.Encode(cloneRecord(record)); err != nil {
                return fmt.Errorf("append raw record to %s: %w", path, err)
        }

        return nil
}

type MemoryStore struct {
        mu      sync.Mutex
        records []rawevidence.Record
}

func NewMemoryStore() *MemoryStore {
        return &MemoryStore{}
}

func (store *MemoryStore) Append(ctx context.Context, record rawevidence.Record) error {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return err
        }
        if err := record.Validate(); err != nil {
                return err
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        store.records = append(store.records, cloneRecord(record))
        return nil
}

func (store *MemoryStore) Snapshot() []rawevidence.Record {
        store.mu.Lock()
        defer store.mu.Unlock()

        snapshot := make([]rawevidence.Record, len(store.records))
        for index, record := range store.records {
                snapshot[index] = cloneRecord(record)
        }
        return snapshot
}

func cloneRecord(record rawevidence.Record) rawevidence.Record {
        cloned := record
        if record.Attributes != nil {
                cloned.Attributes = make(map[string]string, len(record.Attributes))
                for key, value := range record.Attributes {
                        cloned.Attributes[key] = value
                }
        }
        if record.Payload != nil {
                cloned.Payload = append(json.RawMessage(nil), record.Payload...)
        }
        return cloned
}

func sourceFileName(source string) string {
        source = strings.ToLower(strings.TrimSpace(source))
        if source == "" {
                source = "events"
        }

        var builder strings.Builder
        for _, value := range source {
                switch {
                case value >= 'a' && value <= 'z':
                        builder.WriteRune(value)
                case value >= '0' && value <= '9':
                        builder.WriteRune(value)
                default:
                        builder.WriteByte('-')
                }
        }

        filename := strings.Trim(builder.String(), "-")
        if filename == "" {
                filename = "events"
        }
        return filename + ".jsonl"
}
