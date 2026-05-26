package access

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
	silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
)

const fileName = "network-access-events.jsonl"

type Query struct {
	TenantID        string
	SubscriberID    string
	RequestID       string
	SessionID       string
	Outcome         string
	NASIPAddress    string
	ClientIPAddress string
	From            *time.Time
	To              *time.Time
	Limit           int
}

type Store interface {
	Append(context.Context, unifiedaccess.Event) error
	Search(context.Context, Query) ([]unifiedaccess.Event, error)
}

func NewStore(cfg silconfig.StoreConfig) (Store, error) {
	switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
	case "", "memory":
		return NewMemoryStore(), nil
	case "file":
		return NewFileStore(cfg.Path)
	default:
		return nil, fmt.Errorf("access metadata backend %q is not implemented", cfg.Backend)
	}
}

type FileStore struct {
	root string
	mu   sync.Mutex
}

func NewFileStore(root string) (*FileStore, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, fmt.Errorf("access metadata path is required")
	}
	return &FileStore{root: filepath.Clean(root)}, nil
}

func (store *FileStore) Append(ctx context.Context, event unifiedaccess.Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := event.Validate(); err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	if err := os.MkdirAll(store.root, 0o755); err != nil {
		return fmt.Errorf("create access metadata root %s: %w", store.root, err)
	}

	file, err := os.OpenFile(store.path(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open access metadata file %s: %w", store.path(), err)
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(cloneEvent(event)); err != nil {
		return fmt.Errorf("append access metadata to %s: %w", store.path(), err)
	}
	return nil
}

func (store *FileStore) Search(ctx context.Context, filter Query) ([]unifiedaccess.Event, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	file, err := os.Open(store.path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open access metadata file %s: %w", store.path(), err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)

	var results []unifiedaccess.Event
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		var event unifiedaccess.Event
		if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
			return nil, fmt.Errorf("decode access metadata from %s: %w", store.path(), err)
		}
		if err := event.Validate(); err != nil {
			return nil, fmt.Errorf("validate access metadata from %s: %w", store.path(), err)
		}
		if !matches(filter, event) {
			continue
		}

		results = append(results, cloneEvent(event))
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scan access metadata file %s: %w", store.path(), err)
	}

	sortEvents(results)
	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}
	return results, nil
}

func (store *FileStore) path() string {
	return filepath.Join(store.root, fileName)
}

type MemoryStore struct {
	mu     sync.Mutex
	events []unifiedaccess.Event
}

func NewMemoryStore() *MemoryStore {
	return &MemoryStore{}
}

func (store *MemoryStore) Append(ctx context.Context, event unifiedaccess.Event) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := event.Validate(); err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	store.events = append(store.events, cloneEvent(event))
	return nil
}

func (store *MemoryStore) Search(ctx context.Context, filter Query) ([]unifiedaccess.Event, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	store.mu.Lock()
	defer store.mu.Unlock()

	results := make([]unifiedaccess.Event, 0, len(store.events))
	for _, event := range store.events {
		if !matches(filter, event) {
			continue
		}
		results = append(results, cloneEvent(event))
	}

	sortEvents(results)
	if filter.Limit > 0 && len(results) > filter.Limit {
		results = results[:filter.Limit]
	}
	return results, nil
}

func matches(filter Query, event unifiedaccess.Event) bool {
	if tenantID := strings.TrimSpace(filter.TenantID); tenantID != "" && !strings.EqualFold(strings.TrimSpace(event.TenantID), tenantID) {
		return false
	}
	if subscriberID := strings.TrimSpace(filter.SubscriberID); subscriberID != "" && !strings.EqualFold(strings.TrimSpace(event.SubscriberID), subscriberID) {
		return false
	}
	if requestID := strings.TrimSpace(filter.RequestID); requestID != "" && !strings.EqualFold(strings.TrimSpace(event.RequestID), requestID) {
		return false
	}
	if sessionID := strings.TrimSpace(filter.SessionID); sessionID != "" && !strings.EqualFold(strings.TrimSpace(event.SessionID), sessionID) {
		return false
	}
	if outcome := strings.TrimSpace(filter.Outcome); outcome != "" && !strings.EqualFold(strings.TrimSpace(string(event.Outcome)), outcome) {
		return false
	}
	if nasIPAddress := strings.TrimSpace(filter.NASIPAddress); nasIPAddress != "" && !strings.EqualFold(strings.TrimSpace(event.NASIPAddress), nasIPAddress) {
		return false
	}
	if clientIPAddress := strings.TrimSpace(filter.ClientIPAddress); clientIPAddress != "" && !strings.EqualFold(strings.TrimSpace(event.ClientIPAddress), clientIPAddress) {
		return false
	}
	if filter.From != nil && event.OccurredAt.Before(filter.From.UTC()) {
		return false
	}
	if filter.To != nil && event.OccurredAt.After(filter.To.UTC()) {
		return false
	}
	return true
}

func sortEvents(events []unifiedaccess.Event) {
	sort.Slice(events, func(left, right int) bool {
		if events[left].OccurredAt.Equal(events[right].OccurredAt) {
			return events[left].IngestedAt.After(events[right].IngestedAt)
		}
		return events[left].OccurredAt.After(events[right].OccurredAt)
	})
}

func cloneEvent(event unifiedaccess.Event) unifiedaccess.Event {
	cloned := event
	if event.Tags != nil {
		cloned.Tags = append([]string(nil), event.Tags...)
	}
	if event.Attributes != nil {
		cloned.Attributes = make(map[string]string, len(event.Attributes))
		for key, value := range event.Attributes {
			cloned.Attributes[key] = value
		}
	}
	return cloned
}
