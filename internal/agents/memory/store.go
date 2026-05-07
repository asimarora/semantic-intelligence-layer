package memory

import (
        "context"
        "time"

        "github.com/asimarora/semantic-intelligence-layer/internal/agents"
)

type Scope string

const (
        ScopeWorking  Scope = "working"
        ScopeSession  Scope = "session"
        ScopeLongTerm Scope = "long_term"
)

type Entry struct {
        Key       string
        SessionID string
        RunID     string
        Scope     Scope
        Summary   string
        Messages  []agents.Message
        Evidence  []agents.EvidenceRef
        ExpiresAt *time.Time
        UpdatedAt time.Time
}

type SearchQuery struct {
        SessionID string
        Text      string
        Limit     int
}

type SearchHit struct {
        Entry Entry
        Score float64
}

type Store interface {
        LoadSession(ctx context.Context, sessionID string) ([]Entry, error)
        Upsert(ctx context.Context, entry Entry) error
        Search(ctx context.Context, query SearchQuery) ([]SearchHit, error)
}
