package sessions

import (
        "fmt"
        "strings"
        "time"
)

const (
        SchemaVersion = "sil.unified.network-session.v1"
        EventType     = "network.session.accounting"
)

type Status string

const (
        StatusStart         Status = "start"
        StatusInterimUpdate Status = "interim_update"
        StatusStop          Status = "stop"
)

type Event struct {
        EventID            string            `json:"event_id"`
        SourceEventID      string            `json:"source_event_id"`
        Source             string            `json:"source"`
        SourceKey          string            `json:"source_key"`
        SchemaVersion      string            `json:"schema_version"`
        EventType          string            `json:"event_type"`
        TenantID           string            `json:"tenant_id"`
        OccurredAt         time.Time         `json:"occurred_at"`
        IngestedAt         time.Time         `json:"ingested_at"`
        SubscriberID       string            `json:"subscriber_id"`
        SessionID          string            `json:"session_id"`
        Status             Status            `json:"status"`
        NASIPAddress       string            `json:"nas_ip_address,omitempty"`
        NASPort            *int              `json:"nas_port,omitempty"`
        ClientIPAddress    string            `json:"client_ip_address,omitempty"`
        AssignedIPAddress  string            `json:"assigned_ip_address,omitempty"`
        CallingStationID   string            `json:"calling_station_id,omitempty"`
        CalledStationID    string            `json:"called_station_id,omitempty"`
        PacketType         string            `json:"packet_type,omitempty"`
        SessionTimeSeconds *uint64           `json:"session_time_seconds,omitempty"`
        InputOctets        *uint64           `json:"input_octets,omitempty"`
        OutputOctets       *uint64           `json:"output_octets,omitempty"`
        Tags               []string          `json:"tags,omitempty"`
        SemanticText       string            `json:"semantic_text"`
        Attributes         map[string]string `json:"attributes,omitempty"`
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
        if strings.TrimSpace(event.SessionID) == "" {
                return fmt.Errorf("session_id is required")
        }
        if !validStatus(event.Status) {
                return fmt.Errorf("unsupported status %q", event.Status)
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

func validStatus(status Status) bool {
        switch status {
        case StatusStart, StatusInterimUpdate, StatusStop:
                return true
        default:
                return false
        }
}
