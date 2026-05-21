package retrieval

import (
        "fmt"
        "sort"
        "strings"
        "time"
        "unicode"
)

const (
        SchemaVersion            = "sil.retrieval.document.v1"
        EntityTypeNetworkSession = "network_session"
)

type Document struct {
        DocumentID      string            `json:"document_id"`
        SchemaVersion   string            `json:"schema_version"`
        TenantID        string            `json:"tenant_id"`
        Source          string            `json:"source"`
        SourceEventID   string            `json:"source_event_id"`
        SourceKey       string            `json:"source_key"`
        EntityType      string            `json:"entity_type"`
        EventType       string            `json:"event_type"`
        OccurredAt      time.Time         `json:"occurred_at"`
        IngestedAt      time.Time         `json:"ingested_at"`
        Title           string            `json:"title"`
        Content         string            `json:"content"`
        SubscriberID    string            `json:"subscriber_id,omitempty"`
        SessionID       string            `json:"session_id,omitempty"`
        Status          string            `json:"status,omitempty"`
        NASIPAddress    string            `json:"nas_ip_address,omitempty"`
        ClientIPAddress string            `json:"client_ip_address,omitempty"`
        Tags            []string          `json:"tags,omitempty"`
        Terms           []string          `json:"terms"`
        Attributes      map[string]string `json:"attributes,omitempty"`
}

type Hit struct {
        Score        float64  `json:"score"`
        MatchedTerms []string `json:"matched_terms,omitempty"`
        Document     Document `json:"document"`
}

func (document Document) Validate() error {
        if strings.TrimSpace(document.DocumentID) == "" {
                return fmt.Errorf("document_id is required")
        }
        if strings.TrimSpace(document.SchemaVersion) == "" {
                return fmt.Errorf("schema_version is required")
        }
        if strings.TrimSpace(document.TenantID) == "" {
                return fmt.Errorf("tenant_id is required")
        }
        if strings.TrimSpace(document.Source) == "" {
                return fmt.Errorf("source is required")
        }
        if strings.TrimSpace(document.SourceEventID) == "" {
                return fmt.Errorf("source_event_id is required")
        }
        if strings.TrimSpace(document.SourceKey) == "" {
                return fmt.Errorf("source_key is required")
        }
        if strings.TrimSpace(document.EntityType) == "" {
                return fmt.Errorf("entity_type is required")
        }
        if strings.TrimSpace(document.EventType) == "" {
                return fmt.Errorf("event_type is required")
        }
        if document.OccurredAt.IsZero() {
                return fmt.Errorf("occurred_at is required")
        }
        if document.IngestedAt.IsZero() {
                return fmt.Errorf("ingested_at is required")
        }
        if strings.TrimSpace(document.Title) == "" {
                return fmt.Errorf("title is required")
        }
        if strings.TrimSpace(document.Content) == "" {
                return fmt.Errorf("content is required")
        }
        if strings.TrimSpace(document.SubscriberID) == "" {
                return fmt.Errorf("subscriber_id is required")
        }
        if strings.TrimSpace(document.SessionID) == "" {
                return fmt.Errorf("session_id is required")
        }
        if strings.TrimSpace(document.Status) == "" {
                return fmt.Errorf("status is required")
        }
        if len(document.Terms) == 0 {
                return fmt.Errorf("terms are required")
        }

        return nil
}

func NormalizeTerms(values ...string) []string {
        terms := make(map[string]struct{})
        for _, value := range values {
                value = strings.TrimSpace(strings.ToLower(value))
                if value == "" {
                        continue
                }

                for _, field := range strings.Fields(value) {
                        field = normalizeFieldTerm(field)
                        if field != "" {
                                terms[field] = struct{}{}
                        }
                }

                for _, token := range strings.FieldsFunc(value, func(r rune) bool {
                        return !unicode.IsLetter(r) && !unicode.IsDigit(r)
                }) {
                        token = strings.TrimSpace(token)
                        if len(token) < 2 {
                                continue
                        }
                        terms[token] = struct{}{}
                }
        }

        normalized := make([]string, 0, len(terms))
        for term := range terms {
                normalized = append(normalized, term)
        }
        sort.Strings(normalized)
        return normalized
}

func normalizeFieldTerm(value string) string {
        value = strings.TrimSpace(value)
        return strings.TrimFunc(value, func(r rune) bool {
                switch {
                case unicode.IsLetter(r), unicode.IsDigit(r):
                        return false
                case r == '.', r == '-', r == '_', r == ':', r == '/':
                        return false
                default:
                        return true
                }
        })
}
