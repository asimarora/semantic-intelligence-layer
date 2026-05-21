package retrieval

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

        retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
        silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
)

const fileName = "retrieval-documents.jsonl"

type Query struct {
        TenantID        string
        QueryText       string
        SubscriberID    string
        SessionID       string
        Status          string
        NASIPAddress    string
        ClientIPAddress string
        From            *time.Time
        To              *time.Time
        Limit           int
}

type Store interface {
        Append(context.Context, retrievalcontracts.Document) error
        Search(context.Context, Query) ([]retrievalcontracts.Hit, error)
}

func NewStore(cfg silconfig.StoreConfig) (Store, error) {
        switch strings.ToLower(strings.TrimSpace(cfg.Backend)) {
        case "", "memory":
                return NewMemoryStore(), nil
        case "file":
                return NewFileStore(cfg.Path)
        default:
                return nil, fmt.Errorf("retrieval metadata backend %q is not implemented", cfg.Backend)
        }
}

type FileStore struct {
        root string
        mu   sync.Mutex
}

func NewFileStore(root string) (*FileStore, error) {
        root = strings.TrimSpace(root)
        if root == "" {
                return nil, fmt.Errorf("retrieval metadata path is required")
        }
        return &FileStore{root: filepath.Clean(root)}, nil
}

func (store *FileStore) Append(ctx context.Context, document retrievalcontracts.Document) error {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return err
        }

        document = normalizeDocument(document)
        if err := document.Validate(); err != nil {
                return err
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        if err := os.MkdirAll(store.root, 0o755); err != nil {
                return fmt.Errorf("create retrieval metadata root %s: %w", store.root, err)
        }

        file, err := os.OpenFile(store.path(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
        if err != nil {
                return fmt.Errorf("open retrieval metadata file %s: %w", store.path(), err)
        }
        defer file.Close()

        encoder := json.NewEncoder(file)
        encoder.SetEscapeHTML(false)
        if err := encoder.Encode(cloneDocument(document)); err != nil {
                return fmt.Errorf("append retrieval metadata to %s: %w", store.path(), err)
        }

        return nil
}

func (store *FileStore) Search(ctx context.Context, filter Query) ([]retrievalcontracts.Hit, error) {
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
                return nil, fmt.Errorf("open retrieval metadata file %s: %w", store.path(), err)
        }
        defer file.Close()

        scanner := bufio.NewScanner(file)
        buffer := make([]byte, 0, 64*1024)
        scanner.Buffer(buffer, 1024*1024)

        documents := make(map[string]retrievalcontracts.Document)
        for scanner.Scan() {
                if err := ctx.Err(); err != nil {
                        return nil, err
                }

                var document retrievalcontracts.Document
                if err := json.Unmarshal(scanner.Bytes(), &document); err != nil {
                        return nil, fmt.Errorf("decode retrieval metadata from %s: %w", store.path(), err)
                }
                document = normalizeDocument(document)
                if err := document.Validate(); err != nil {
                        return nil, fmt.Errorf("validate retrieval metadata from %s: %w", store.path(), err)
                }
                if !matches(filter, document) {
                        continue
                }

                if existing, ok := documents[document.DocumentID]; ok {
                        documents[document.DocumentID] = pickLatest(existing, document)
                        continue
                }
                documents[document.DocumentID] = cloneDocument(document)
        }
        if err := scanner.Err(); err != nil {
                return nil, fmt.Errorf("scan retrieval metadata file %s: %w", store.path(), err)
        }

        return scoreResults(filter, documents), nil
}

func (store *FileStore) path() string {
        return filepath.Join(store.root, fileName)
}

type MemoryStore struct {
        mu        sync.Mutex
        documents map[string]retrievalcontracts.Document
}

func NewMemoryStore() *MemoryStore {
        return &MemoryStore{
                documents: make(map[string]retrievalcontracts.Document),
        }
}

func (store *MemoryStore) Append(ctx context.Context, document retrievalcontracts.Document) error {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return err
        }

        document = normalizeDocument(document)
        if err := document.Validate(); err != nil {
                return err
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        if store.documents == nil {
                store.documents = make(map[string]retrievalcontracts.Document)
        }
        store.documents[document.DocumentID] = cloneDocument(document)
        return nil
}

func (store *MemoryStore) Search(ctx context.Context, filter Query) ([]retrievalcontracts.Hit, error) {
        if ctx == nil {
                ctx = context.Background()
        }
        if err := ctx.Err(); err != nil {
                return nil, err
        }

        store.mu.Lock()
        defer store.mu.Unlock()

        documents := make(map[string]retrievalcontracts.Document, len(store.documents))
        for id, document := range store.documents {
                if !matches(filter, document) {
                        continue
                }
                documents[id] = cloneDocument(document)
        }

        return scoreResults(filter, documents), nil
}

func matches(filter Query, document retrievalcontracts.Document) bool {
        if tenantID := strings.TrimSpace(filter.TenantID); tenantID != "" && !strings.EqualFold(strings.TrimSpace(document.TenantID), tenantID) {
                return false
        }
        if subscriberID := strings.TrimSpace(filter.SubscriberID); subscriberID != "" && !strings.EqualFold(strings.TrimSpace(document.SubscriberID), subscriberID) {
                return false
        }
        if sessionID := strings.TrimSpace(filter.SessionID); sessionID != "" && !strings.EqualFold(strings.TrimSpace(document.SessionID), sessionID) {
                return false
        }
        if status := strings.TrimSpace(filter.Status); status != "" && !strings.EqualFold(strings.TrimSpace(document.Status), status) {
                return false
        }
        if nasIPAddress := strings.TrimSpace(filter.NASIPAddress); nasIPAddress != "" && !strings.EqualFold(strings.TrimSpace(document.NASIPAddress), nasIPAddress) {
                return false
        }
        if clientIPAddress := strings.TrimSpace(filter.ClientIPAddress); clientIPAddress != "" && !strings.EqualFold(strings.TrimSpace(document.ClientIPAddress), clientIPAddress) {
                return false
        }
        if filter.From != nil && document.OccurredAt.Before(filter.From.UTC()) {
                return false
        }
        if filter.To != nil && document.OccurredAt.After(filter.To.UTC()) {
                return false
        }
        return true
}

func scoreResults(filter Query, documents map[string]retrievalcontracts.Document) []retrievalcontracts.Hit {
        queryTerms := retrievalcontracts.NormalizeTerms(filter.QueryText)
        if len(queryTerms) == 0 {
                return nil
        }

        hits := make([]retrievalcontracts.Hit, 0, len(documents))
        for _, document := range documents {
                score, matchedTerms := scoreDocument(strings.TrimSpace(filter.QueryText), queryTerms, document)
                if score <= 0 {
                        continue
                }
                hits = append(hits, retrievalcontracts.Hit{
                        Score:        score,
                        MatchedTerms: matchedTerms,
                        Document:     cloneDocument(document),
                })
        }

        sort.Slice(hits, func(left, right int) bool {
                if hits[left].Score == hits[right].Score {
                        if hits[left].Document.OccurredAt.Equal(hits[right].Document.OccurredAt) {
                                return hits[left].Document.IngestedAt.After(hits[right].Document.IngestedAt)
                        }
                        return hits[left].Document.OccurredAt.After(hits[right].Document.OccurredAt)
                }
                return hits[left].Score > hits[right].Score
        })
        if filter.Limit > 0 && len(hits) > filter.Limit {
                hits = hits[:filter.Limit]
        }

        cloned := make([]retrievalcontracts.Hit, 0, len(hits))
        for _, hit := range hits {
                cloned = append(cloned, cloneHit(hit))
        }
        return cloned
}

func scoreDocument(rawQuery string, queryTerms []string, document retrievalcontracts.Document) (float64, []string) {
        termSet := make(map[string]struct{}, len(document.Terms))
        for _, term := range document.Terms {
                termSet[strings.ToLower(strings.TrimSpace(term))] = struct{}{}
        }

        matched := make([]string, 0, len(queryTerms))
        score := 0.0
        for _, term := range queryTerms {
                if _, ok := termSet[term]; !ok {
                        continue
                }
                matched = append(matched, term)
                score += 2
        }

        searchText := strings.ToLower(strings.Join([]string{document.Title, document.Content}, " "))
        if query := strings.ToLower(strings.TrimSpace(rawQuery)); query != "" && strings.Contains(searchText, query) {
                score += 1.5
        }
        if len(matched) == len(queryTerms) && len(queryTerms) > 0 {
                score += 1
        }

        if len(matched) == 0 {
                return 0, nil
        }
        sort.Strings(matched)
        return score, matched
}

func pickLatest(existing, candidate retrievalcontracts.Document) retrievalcontracts.Document {
        if candidate.IngestedAt.After(existing.IngestedAt) {
                return cloneDocument(candidate)
        }
        if candidate.IngestedAt.Equal(existing.IngestedAt) && candidate.OccurredAt.After(existing.OccurredAt) {
                return cloneDocument(candidate)
        }
        return cloneDocument(existing)
}

func normalizeDocument(document retrievalcontracts.Document) retrievalcontracts.Document {
        document.DocumentID = strings.TrimSpace(document.DocumentID)
        document.SchemaVersion = strings.TrimSpace(document.SchemaVersion)
        document.TenantID = strings.TrimSpace(document.TenantID)
        document.Source = strings.TrimSpace(document.Source)
        document.SourceEventID = strings.TrimSpace(document.SourceEventID)
        document.SourceKey = strings.TrimSpace(document.SourceKey)
        document.EntityType = strings.TrimSpace(document.EntityType)
        document.EventType = strings.TrimSpace(document.EventType)
        document.Title = strings.TrimSpace(document.Title)
        document.Content = strings.TrimSpace(document.Content)
        document.SubscriberID = strings.TrimSpace(document.SubscriberID)
        document.SessionID = strings.TrimSpace(document.SessionID)
        document.Status = strings.TrimSpace(document.Status)
        document.NASIPAddress = strings.TrimSpace(document.NASIPAddress)
        document.ClientIPAddress = strings.TrimSpace(document.ClientIPAddress)
        document.Terms = retrievalcontracts.NormalizeTerms(document.Terms...)

        if document.Tags != nil {
                document.Tags = append([]string(nil), document.Tags...)
                sort.Strings(document.Tags)
        }
        if document.Attributes != nil {
                cloned := make(map[string]string, len(document.Attributes))
                for key, value := range document.Attributes {
                        key = strings.TrimSpace(key)
                        if key == "" {
                                continue
                        }
                        cloned[key] = strings.TrimSpace(value)
                }
                document.Attributes = cloned
        }

        return document
}

func cloneDocument(document retrievalcontracts.Document) retrievalcontracts.Document {
        cloned := document
        if document.Tags != nil {
                cloned.Tags = append([]string(nil), document.Tags...)
        }
        if document.Terms != nil {
                cloned.Terms = append([]string(nil), document.Terms...)
        }
        if document.Attributes != nil {
                cloned.Attributes = make(map[string]string, len(document.Attributes))
                for key, value := range document.Attributes {
                        cloned.Attributes[key] = value
                }
        }
        return cloned
}

func cloneHit(hit retrievalcontracts.Hit) retrievalcontracts.Hit {
        cloned := hit
        if hit.MatchedTerms != nil {
                cloned.MatchedTerms = append([]string(nil), hit.MatchedTerms...)
        }
        cloned.Document = cloneDocument(hit.Document)
        return cloned
}
