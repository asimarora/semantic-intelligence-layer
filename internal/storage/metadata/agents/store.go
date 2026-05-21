package agents

import (
        "bufio"
        "context"
        "encoding/json"
        "errors"
        "fmt"
        "os"
        "path/filepath"
        "sort"
        "strings"
        "sync"

        agenttypes "github.com/asimarora/semantic-intelligence-layer/internal/agents"
        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
)

const fileName = "agent-runs.jsonl"

var ErrNotFound = errors.New("agent run not found")

type Record struct {
        Run   agenttypes.Run    `json:"run"`
        Steps []agenttypes.Step `json:"steps,omitempty"`
}

type Store interface {
        Upsert(context.Context, Record) error
        Get(context.Context, string) (Record, error)
}

func NewStore(cfg silconfig.StoreConfig) (Store, error) {
        switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
        case "", "memory":
                return NewMemoryStore(), nil
        case "file":
                return NewFileStore(cfg.Path)
        default:
                return nil, fmt.Errorf("agent metadata backend %q is not implemented", cfg.Backend)
        }
}

type FileStore struct {
        root string
        mu   sync.Mutex
}

func NewFileStore(root string) (*FileStore, error) {
        root = strings.TrimSpace(root)
        if root == "" {
                return nil, fmt.Errorf("agent metadata path is required")
        }
        return &FileStore{root: filepath.Clean(root)}, nil
}

func (store *FileStore) Upsert(ctx context.Context, record Record) error {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return err
        }

        record = normalizeRecord(record)
        if err := validateRecord(record); err != nil {
                return err
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        if err := os.MkdirAll(store.root, 0o755); err != nil {
                return fmt.Errorf("create agent metadata root %s: %w", store.root, err)
        }

        file, err := os.OpenFile(store.path(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
        if err != nil {
                return fmt.Errorf("open agent metadata file %s: %w", store.path(), err)
        }
        defer file.Close()

        encoder := json.NewEncoder(file)
        encoder.SetEscapeHTML(false)
        if err := encoder.Encode(cloneRecord(record)); err != nil {
                return fmt.Errorf("append agent metadata to %s: %w", store.path(), err)
        }
        return nil
}

func (store *FileStore) Get(ctx context.Context, runID string) (Record, error) {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return Record{}, err
        }

        runID = strings.TrimSpace(runID)
        if runID == "" {
                return Record{}, fmt.Errorf("run id is required")
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        file, err := os.Open(store.path())
        if err != nil {
                if os.IsNotExist(err) {
                        return Record{}, ErrNotFound
                }
                return Record{}, fmt.Errorf("open agent metadata file %s: %w", store.path(), err)
        }
        defer file.Close()

        scanner := bufio.NewScanner(file)
        buffer := make([]byte, 0, 64*1024)
        scanner.Buffer(buffer, 1024*1024)

        var latest Record
        found := false
        for scanner.Scan() {
                if err := ctx.Err(); err != nil {
                        return Record{}, err
                }

                var record Record
                if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
                        return Record{}, fmt.Errorf("decode agent metadata from %s: %w", store.path(), err)
                }
                record = normalizeRecord(record)
                if err := validateRecord(record); err != nil {
                        return Record{}, fmt.Errorf("validate agent metadata from %s: %w", store.path(), err)
                }
                if record.Run.ID != runID {
                        continue
                }
                if !found {
                        latest = cloneRecord(record)
                        found = true
                        continue
                }
                latest = pickLatest(latest, record)
        }
        if err := scanner.Err(); err != nil {
                return Record{}, fmt.Errorf("scan agent metadata file %s: %w", store.path(), err)
        }
        if !found {
                return Record{}, ErrNotFound
        }
        return latest, nil
}

func (store *FileStore) path() string {
        return filepath.Join(store.root, fileName)
}

type MemoryStore struct {
        mu      sync.Mutex
        records map[string]Record
}

func NewMemoryStore() *MemoryStore {
        return &MemoryStore{
                records: make(map[string]Record),
        }
}

func (store *MemoryStore) Upsert(ctx context.Context, record Record) error {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return err
        }

        record = normalizeRecord(record)
        if err := validateRecord(record); err != nil {
                return err
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        if store.records == nil {
                store.records = make(map[string]Record)
        }
        store.records[record.Run.ID] = cloneRecord(record)
        return nil
}

func (store *MemoryStore) Get(ctx context.Context, runID string) (Record, error) {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return Record{}, err
        }

        runID = strings.TrimSpace(runID)
        if runID == "" {
                return Record{}, fmt.Errorf("run id is required")
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        record, ok := store.records[runID]
        if !ok {
                return Record{}, ErrNotFound
        }
        return cloneRecord(record), nil
}

func validateRecord(record Record) error {
        if strings.TrimSpace(record.Run.ID) == "" {
                return fmt.Errorf("run.id is required")
        }
        if strings.TrimSpace(record.Run.TenantID) == "" {
                return fmt.Errorf("run.tenant_id is required")
        }
        if strings.TrimSpace(record.Run.SessionID) == "" {
                return fmt.Errorf("run.session_id is required")
        }
        if strings.TrimSpace(record.Run.Goal) == "" {
                return fmt.Errorf("run.goal is required")
        }
        if !validRunStatus(record.Run.Status) {
                return fmt.Errorf("unsupported run status %q", record.Run.Status)
        }
        if record.Run.StartedAt.IsZero() {
                return fmt.Errorf("run.started_at is required")
        }
        if record.Run.UpdatedAt.IsZero() {
                return fmt.Errorf("run.updated_at is required")
        }

        for _, step := range record.Steps {
                if strings.TrimSpace(step.ID) == "" {
                        return fmt.Errorf("step.id is required")
                }
                if strings.TrimSpace(step.Kind) == "" {
                        return fmt.Errorf("step.kind is required")
                }
                if strings.TrimSpace(step.Summary) == "" {
                        return fmt.Errorf("step.summary is required")
                }
                if !validStepStatus(step.Status) {
                        return fmt.Errorf("unsupported step status %q", step.Status)
                }
                if step.StartedAt.IsZero() {
                        return fmt.Errorf("step.started_at is required")
                }
        }
        return nil
}

func validRunStatus(status agenttypes.RunStatus) bool {
        switch status {
        case agenttypes.RunStatusPending, agenttypes.RunStatusRunning, agenttypes.RunStatusAwaitingApproval, agenttypes.RunStatusCompleted, agenttypes.RunStatusFailed:
                return true
        default:
                return false
        }
}

func validStepStatus(status agenttypes.StepStatus) bool {
        switch status {
        case agenttypes.StepStatusPending, agenttypes.StepStatusRunning, agenttypes.StepStatusBlocked, agenttypes.StepStatusCompleted, agenttypes.StepStatusFailed:
                return true
        default:
                return false
        }
}

func normalizeRecord(record Record) Record {
        record.Run.ID = strings.TrimSpace(record.Run.ID)
        record.Run.TenantID = strings.TrimSpace(record.Run.TenantID)
        record.Run.CaseID = strings.TrimSpace(record.Run.CaseID)
        record.Run.SessionID = strings.TrimSpace(record.Run.SessionID)
        record.Run.Goal = strings.TrimSpace(record.Run.Goal)
        record.Run.RuntimeName = strings.TrimSpace(record.Run.RuntimeName)
        record.Run.PlannerName = strings.TrimSpace(record.Run.PlannerName)
        record.Run.StartedAt = record.Run.StartedAt.UTC()
        record.Run.UpdatedAt = record.Run.UpdatedAt.UTC()
        if record.Run.CompletedAt != nil {
                value := record.Run.CompletedAt.UTC()
                record.Run.CompletedAt = &value
        }

        normalizedSteps := make([]agenttypes.Step, 0, len(record.Steps))
        for _, step := range record.Steps {
                step.ID = strings.TrimSpace(step.ID)
                step.Kind = strings.TrimSpace(step.Kind)
                step.Summary = strings.TrimSpace(step.Summary)
                step.ToolName = strings.TrimSpace(step.ToolName)
                step.StartedAt = step.StartedAt.UTC()
                if step.CompletedAt != nil {
                        value := step.CompletedAt.UTC()
                        step.CompletedAt = &value
                }
                step.Evidence = cloneEvidence(step.Evidence)
                normalizedSteps = append(normalizedSteps, step)
        }
        record.Steps = normalizedSteps
        return record
}

func pickLatest(existing, candidate Record) Record {
        if candidate.Run.UpdatedAt.After(existing.Run.UpdatedAt) {
                return cloneRecord(candidate)
        }
        if candidate.Run.UpdatedAt.Equal(existing.Run.UpdatedAt) {
                if candidate.Run.CompletedAt != nil && existing.Run.CompletedAt == nil {
                        return cloneRecord(candidate)
                }
                if candidate.Run.CompletedAt != nil && existing.Run.CompletedAt != nil && candidate.Run.CompletedAt.After(*existing.Run.CompletedAt) {
                        return cloneRecord(candidate)
                }
        }
        return cloneRecord(existing)
}

func cloneRecord(record Record) Record {
        cloned := record
        if record.Run.CompletedAt != nil {
                value := *record.Run.CompletedAt
                cloned.Run.CompletedAt = &value
        }
        if record.Steps != nil {
                cloned.Steps = make([]agenttypes.Step, 0, len(record.Steps))
                for _, step := range record.Steps {
                        cloned.Steps = append(cloned.Steps, cloneStep(step))
                }
        }
        return cloned
}

func cloneStep(step agenttypes.Step) agenttypes.Step {
        cloned := step
        if step.CompletedAt != nil {
                value := *step.CompletedAt
                cloned.CompletedAt = &value
        }
        cloned.Evidence = cloneEvidence(step.Evidence)
        return cloned
}

func cloneEvidence(values []agenttypes.EvidenceRef) []agenttypes.EvidenceRef {
        if values == nil {
                return nil
        }
        cloned := make([]agenttypes.EvidenceRef, 0, len(values))
        cloned = append(cloned, values...)
        sort.SliceStable(cloned, func(left, right int) bool {
                if cloned[left].Score == cloned[right].Score {
                        return cloned[left].ID < cloned[right].ID
                }
                return cloned[left].Score > cloned[right].Score
        })
        return cloned
}
