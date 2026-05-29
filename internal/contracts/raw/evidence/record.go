package evidence

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type Record struct {
	EventID       string            `json:"event_id"`
	Source        string            `json:"source"`
	SourceKey     string            `json:"source_key"`
	SchemaVersion string            `json:"schema_version"`
	TenantID      string            `json:"tenant_id"`
	OccurredAt    time.Time         `json:"occurred_at"`
	IngestedAt    time.Time         `json:"ingested_at"`
	Attributes    map[string]string `json:"attributes,omitempty"`
	Payload       json.RawMessage   `json:"payload"`
}

func (record Record) Validate() error {
	if strings.TrimSpace(record.EventID) == "" {
		return fmt.Errorf("event_id is required")
	}
	if strings.TrimSpace(record.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if strings.TrimSpace(record.SourceKey) == "" {
		return fmt.Errorf("source_key is required")
	}
	if strings.TrimSpace(record.SchemaVersion) == "" {
		return fmt.Errorf("schema_version is required")
	}
	if strings.TrimSpace(record.TenantID) == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if record.OccurredAt.IsZero() {
		return fmt.Errorf("occurred_at is required")
	}
	if record.IngestedAt.IsZero() {
		return fmt.Errorf("ingested_at is required")
	}
	if len(record.Payload) == 0 {
		return fmt.Errorf("payload is required")
	}
	if !json.Valid(record.Payload) {
		return fmt.Errorf("payload must be valid JSON")
	}
	return nil
}
