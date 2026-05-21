package agents

import (
        "bufio"
        "context"
        "encoding/json"
        "fmt"
        "os"
        "path/filepath"
        "strings"
        "sync"
        "time"

        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
)

const watcherStateFileName = "agent-watcher-states.jsonl"

type WatcherState struct {
        Key            string     `json:"key"`
        WatcherName    string     `json:"watcher_name"`
        TenantID       string     `json:"tenant_id"`
        SubjectID      string     `json:"subject_id"`
        LastEventID    string     `json:"last_event_id"`
        LastOccurredAt *time.Time `json:"last_occurred_at,omitempty"`
        LastRunID      string     `json:"last_run_id,omitempty"`
        UpdatedAt      time.Time  `json:"updated_at"`
}

type WatcherStateStore interface {
        UpsertWatcherState(context.Context, WatcherState) error
        GetWatcherState(context.Context, string) (WatcherState, error)
}

func NewWatcherStateStore(cfg silconfig.StoreConfig) (WatcherStateStore, error) {
        switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
        case "", "memory":
                return NewMemoryWatcherStateStore(), nil
        case "file":
                return NewFileWatcherStateStore(cfg.Path)
        default:
                return nil, fmt.Errorf("agent watcher state backend %q is not implemented", cfg.Backend)
        }
}

type FileWatcherStateStore struct {
        root string
        mu   sync.Mutex
}

func NewFileWatcherStateStore(root string) (*FileWatcherStateStore, error) {
        root = strings.TrimSpace(root)
        if root == "" {
                return nil, fmt.Errorf("agent watcher state path is required")
        }
        return &FileWatcherStateStore{root: filepath.Clean(root)}, nil
}

func (store *FileWatcherStateStore) UpsertWatcherState(ctx context.Context, state WatcherState) error {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return err
        }

        state = normalizeWatcherState(state)
        if err := validateWatcherState(state); err != nil {
                return err
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        if err := os.MkdirAll(store.root, 0o755); err != nil {
                return fmt.Errorf("create watcher state root %s: %w", store.root, err)
        }

        file, err := os.OpenFile(store.path(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
        if err != nil {
                return fmt.Errorf("open watcher state file %s: %w", store.path(), err)
        }
        defer file.Close()

        encoder := json.NewEncoder(file)
        encoder.SetEscapeHTML(false)
        if err := encoder.Encode(cloneWatcherState(state)); err != nil {
                return fmt.Errorf("append watcher state to %s: %w", store.path(), err)
        }
        return nil
}

func (store *FileWatcherStateStore) GetWatcherState(ctx context.Context, key string) (WatcherState, error) {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return WatcherState{}, err
        }

        key = strings.TrimSpace(key)
        if key == "" {
                return WatcherState{}, fmt.Errorf("watcher state key is required")
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        file, err := os.Open(store.path())
        if err != nil {
                if os.IsNotExist(err) {
                        return WatcherState{}, ErrNotFound
                }
                return WatcherState{}, fmt.Errorf("open watcher state file %s: %w", store.path(), err)
        }
        defer file.Close()

        scanner := bufio.NewScanner(file)
        buffer := make([]byte, 0, 64*1024)
        scanner.Buffer(buffer, 1024*1024)

        var latest WatcherState
        found := false
        for scanner.Scan() {
                if err := ctx.Err(); err != nil {
                        return WatcherState{}, err
                }

                var state WatcherState
                if err := json.Unmarshal(scanner.Bytes(), &state); err != nil {
                        return WatcherState{}, fmt.Errorf("decode watcher state from %s: %w", store.path(), err)
                }
                state = normalizeWatcherState(state)
                if err := validateWatcherState(state); err != nil {
                        return WatcherState{}, fmt.Errorf("validate watcher state from %s: %w", store.path(), err)
                }
                if state.Key != key {
                        continue
                }
                if !found || state.UpdatedAt.After(latest.UpdatedAt) {
                        latest = cloneWatcherState(state)
                        found = true
                }
        }
        if err := scanner.Err(); err != nil {
                return WatcherState{}, fmt.Errorf("scan watcher state file %s: %w", store.path(), err)
        }
        if !found {
                return WatcherState{}, ErrNotFound
        }
        return latest, nil
}

func (store *FileWatcherStateStore) path() string {
        return filepath.Join(store.root, watcherStateFileName)
}

type MemoryWatcherStateStore struct {
        mu     sync.Mutex
        states map[string]WatcherState
}

func NewMemoryWatcherStateStore() *MemoryWatcherStateStore {
        return &MemoryWatcherStateStore{
                states: make(map[string]WatcherState),
        }
}

func (store *MemoryWatcherStateStore) UpsertWatcherState(ctx context.Context, state WatcherState) error {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return err
        }

        state = normalizeWatcherState(state)
        if err := validateWatcherState(state); err != nil {
                return err
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        if store.states == nil {
                store.states = make(map[string]WatcherState)
        }
        store.states[state.Key] = cloneWatcherState(state)
        return nil
}

func (store *MemoryWatcherStateStore) GetWatcherState(ctx context.Context, key string) (WatcherState, error) {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return WatcherState{}, err
        }

        key = strings.TrimSpace(key)
        if key == "" {
                return WatcherState{}, fmt.Errorf("watcher state key is required")
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        state, ok := store.states[key]
        if !ok {
                return WatcherState{}, ErrNotFound
        }
        return cloneWatcherState(state), nil
}

func validateWatcherState(state WatcherState) error {
        if strings.TrimSpace(state.Key) == "" {
                return fmt.Errorf("key is required")
        }
        if strings.TrimSpace(state.WatcherName) == "" {
                return fmt.Errorf("watcher_name is required")
        }
        if strings.TrimSpace(state.TenantID) == "" {
                return fmt.Errorf("tenant_id is required")
        }
        if strings.TrimSpace(state.SubjectID) == "" {
                return fmt.Errorf("subject_id is required")
        }
        if strings.TrimSpace(state.LastEventID) == "" {
                return fmt.Errorf("last_event_id is required")
        }
        if state.UpdatedAt.IsZero() {
                return fmt.Errorf("updated_at is required")
        }
        return nil
}

func normalizeWatcherState(state WatcherState) WatcherState {
        state.Key = strings.TrimSpace(state.Key)
        state.WatcherName = strings.TrimSpace(state.WatcherName)
        state.TenantID = strings.TrimSpace(state.TenantID)
        state.SubjectID = strings.TrimSpace(state.SubjectID)
        state.LastEventID = strings.TrimSpace(state.LastEventID)
        state.LastRunID = strings.TrimSpace(state.LastRunID)
        state.UpdatedAt = state.UpdatedAt.UTC()
        if state.LastOccurredAt != nil {
                value := state.LastOccurredAt.UTC()
                state.LastOccurredAt = &value
        }
        return state
}

func cloneWatcherState(state WatcherState) WatcherState {
        cloned := state
        if state.LastOccurredAt != nil {
                value := *state.LastOccurredAt
                cloned.LastOccurredAt = &value
        }
        return cloned
}
