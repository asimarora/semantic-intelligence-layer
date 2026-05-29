package access

import (
	"fmt"
	"strings"
	"time"
)

const (
	SchemaVersion = "sil.unified.network-access.v1"
	EventType     = "network.access.authentication"
)

type Outcome string

const (
	OutcomeAccepted   Outcome = "accepted"
	OutcomeRejected   Outcome = "rejected"
	OutcomeChallenged Outcome = "challenged"
)

type Event struct {
	EventID          string            `json:"event_id"`
	SourceEventID    string            `json:"source_event_id"`
	Source           string            `json:"source"`
	SourceKey        string            `json:"source_key"`
	SchemaVersion    string            `json:"schema_version"`
	EventType        string            `json:"event_type"`
	TenantID         string            `json:"tenant_id"`
	OccurredAt       time.Time         `json:"occurred_at"`
	IngestedAt       time.Time         `json:"ingested_at"`
	SubscriberID     string            `json:"subscriber_id"`
	RequestID        string            `json:"request_id"`
	SessionID        string            `json:"session_id,omitempty"`
	Outcome          Outcome           `json:"outcome"`
	RejectReason     string            `json:"reject_reason,omitempty"`
	NASIPAddress     string            `json:"nas_ip_address,omitempty"`
	ClientIPAddress  string            `json:"client_ip_address,omitempty"`
	CallingStationID string            `json:"calling_station_id,omitempty"`
	CalledStationID  string            `json:"called_station_id,omitempty"`
	PacketType       string            `json:"packet_type,omitempty"`
	AuthProtocol     string            `json:"auth_protocol,omitempty"`
	PolicyName       string            `json:"policy_name,omitempty"`
	Tags             []string          `json:"tags,omitempty"`
	SemanticText     string            `json:"semantic_text"`
	Attributes       map[string]string `json:"attributes,omitempty"`
}

func (event Event) Validate() error {
	if strings.TrimSpace(event.EventID) == "" {
		return fmt.Errorf("event_id is required")
	}
	if strings.TrimSpace(event.SourceEventID) == "" {
		return fmt.Errorf("source_event_id is required")
	}
	if strings.TrimSpace(event.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if strings.TrimSpace(event.SourceKey) == "" {
		return fmt.Errorf("source_key is required")
	}
	if strings.TrimSpace(event.SchemaVersion) == "" {
		return fmt.Errorf("schema_version is required")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return fmt.Errorf("event_type is required")
	}
	if strings.TrimSpace(event.TenantID) == "" {
		return fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(event.SubscriberID) == "" {
		return fmt.Errorf("subscriber_id is required")
	}
	if strings.TrimSpace(event.RequestID) == "" {
		return fmt.Errorf("request_id is required")
	}
	if !validOutcome(event.Outcome) {
		return fmt.Errorf("unsupported outcome %q", event.Outcome)
	}
	if event.OccurredAt.IsZero() {
		return fmt.Errorf("occurred_at is required")
	}
	if event.IngestedAt.IsZero() {
		return fmt.Errorf("ingested_at is required")
	}
	if strings.TrimSpace(event.SemanticText) == "" {
		return fmt.Errorf("semantic_text is required")
	}
	return nil
}

func validOutcome(outcome Outcome) bool {
	switch outcome {
	case OutcomeAccepted, OutcomeRejected, OutcomeChallenged:
		return true
	default:
		return false
	}
}
