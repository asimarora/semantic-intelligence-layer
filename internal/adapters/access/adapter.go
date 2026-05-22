package access

import (
        "bytes"
        "encoding/json"
        "fmt"
        "strings"
        "time"

        rawevidence "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/evidence"
        unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
        "github.com/asimarora/semantic-intelligence-layer/internal/platform/messaging"
)

const sourceKeyTimeFormat = "20060102T150405.000000"

type AuthEvent struct {
        Username         string    `json:"username"`
        RequestID        string    `json:"request_id,omitempty"`
        Decision         string    `json:"decision"`
        RejectReason     string    `json:"reject_reason,omitempty"`
        NASIPAddress     string    `json:"nas_ip_address,omitempty"`
        ClientIP         string    `json:"client_ip,omitempty"`
        CallingStationID string    `json:"calling_station_id,omitempty"`
        CalledStationID  string    `json:"called_station_id,omitempty"`
        PacketType       string    `json:"packet_type,omitempty"`
        AuthProtocol     string    `json:"auth_protocol,omitempty"`
        PolicyName       string    `json:"policy_name,omitempty"`
        AcctSessionID    string    `json:"acct_session_id,omitempty"`
        Timestamp        time.Time `json:"timestamp"`
}

type AdaptedRecord struct {
        Event    AuthEvent
        Record   rawevidence.Record
        Envelope messaging.Envelope
}

func Adapt(cfg Config, payload []byte, ingestedAt time.Time) (AdaptedRecord, error) {
        if err := cfg.Validate(); err != nil {
                return AdaptedRecord{}, err
        }

        payload = bytes.TrimSpace(payload)
        if len(payload) == 0 {
                return AdaptedRecord{}, fmt.Errorf("access payload is empty")
        }
        if !json.Valid(payload) {
                return AdaptedRecord{}, fmt.Errorf("access payload must be valid JSON")
        }

        var event AuthEvent
        if err := json.Unmarshal(payload, &event); err != nil {
                return AdaptedRecord{}, fmt.Errorf("decode access payload: %w", err)
        }
        if err := event.Validate(); err != nil {
                return AdaptedRecord{}, err
        }

        if ingestedAt.IsZero() {
                ingestedAt = time.Now().UTC()
        } else {
                ingestedAt = ingestedAt.UTC()
        }

        partitionKey, err := event.partitionKey()
        if err != nil {
                return AdaptedRecord{}, err
        }

        record := rawevidence.Record{
                EventID:       event.eventID(),
                Source:        Source,
                SourceKey:     event.sourceKey(),
                SchemaVersion: cfg.SchemaVersion,
                TenantID:      cfg.TenantID,
                OccurredAt:    event.Timestamp.UTC(),
                IngestedAt:    ingestedAt,
                Attributes:    event.attributes(),
                Payload:       append(json.RawMessage(nil), payload...),
        }
        if err := record.Validate(); err != nil {
                return AdaptedRecord{}, err
        }

        envelope := messaging.Envelope{
                EventID:       record.EventID,
                Source:        record.Source,
                SourceKey:     record.SourceKey,
                SchemaVersion: record.SchemaVersion,
                PartitionKey:  partitionKey,
                OccurredAt:    record.OccurredAt,
                IngestedAt:    record.IngestedAt,
                Payload:       append(json.RawMessage(nil), record.Payload...),
        }
        if err := envelope.Validate(); err != nil {
                return AdaptedRecord{}, err
        }

        return AdaptedRecord{
                Event:    event,
                Record:   record,
                Envelope: envelope,
        }, nil
}

func DecodeRecord(record rawevidence.Record) (AuthEvent, error) {
        if err := record.Validate(); err != nil {
                return AuthEvent{}, err
        }
        if !strings.EqualFold(strings.TrimSpace(record.Source), Source) {
                return AuthEvent{}, fmt.Errorf("unsupported evidence source %q", record.Source)
        }

        var event AuthEvent
        if err := json.Unmarshal(record.Payload, &event); err != nil {
                return AuthEvent{}, fmt.Errorf("decode access evidence payload: %w", err)
        }
        if err := event.Validate(); err != nil {
                return AuthEvent{}, err
        }
        return event, nil
}

func NormalizeRecord(record rawevidence.Record) (unifiedaccess.Event, error) {
        event, err := DecodeRecord(record)
        if err != nil {
                return unifiedaccess.Event{}, err
        }

        outcome, err := normalizeOutcome(event.Decision)
        if err != nil {
                return unifiedaccess.Event{}, err
        }

        normalized := unifiedaccess.Event{
                EventID:          record.EventID + ":unified",
                SourceEventID:    record.EventID,
                Source:           record.Source,
                SourceKey:        record.SourceKey,
                SchemaVersion:    unifiedaccess.SchemaVersion,
                EventType:        unifiedaccess.EventType,
                TenantID:         record.TenantID,
                OccurredAt:       record.OccurredAt,
                IngestedAt:       record.IngestedAt,
                SubscriberID:     strings.TrimSpace(event.Username),
                RequestID:        event.effectiveRequestID(),
                SessionID:        strings.TrimSpace(event.AcctSessionID),
                Outcome:          outcome,
                RejectReason:     strings.TrimSpace(event.RejectReason),
                NASIPAddress:     strings.TrimSpace(event.NASIPAddress),
                ClientIPAddress:  strings.TrimSpace(event.ClientIP),
                CallingStationID: strings.TrimSpace(event.CallingStationID),
                CalledStationID:  strings.TrimSpace(event.CalledStationID),
                PacketType:       strings.TrimSpace(event.PacketType),
                AuthProtocol:     strings.TrimSpace(event.AuthProtocol),
                PolicyName:       strings.TrimSpace(event.PolicyName),
                Tags:             tagsForOutcome(outcome),
                SemanticText:     buildSemanticText(event, outcome),
                Attributes:       cloneAttributes(record.Attributes),
        }
        if err := normalized.Validate(); err != nil {
                return unifiedaccess.Event{}, err
        }

        return normalized, nil
}

func (event AuthEvent) Validate() error {
        if strings.TrimSpace(event.Username) == "" {
                return fmt.Errorf("access username is required")
        }
        if strings.TrimSpace(event.Decision) == "" {
                return fmt.Errorf("access decision is required")
        }
        if event.Timestamp.IsZero() {
                return fmt.Errorf("access timestamp is required")
        }
        return nil
}

func (event AuthEvent) eventID() string {
        return fmt.Sprintf(
                "%s:%s:%s",
                Source,
                sanitizeIDToken(event.effectiveRequestID()),
                event.Timestamp.UTC().Format(time.RFC3339Nano),
        )
}

func (event AuthEvent) sourceKey() string {
        return fmt.Sprintf(
                "%s:auth:%s:%s:%s",
                Kind,
                sanitizeIDToken(event.Username),
                sanitizeIDToken(event.effectiveRequestID()),
                event.Timestamp.UTC().Format(sourceKeyTimeFormat),
        )
}

func (event AuthEvent) effectiveRequestID() string {
        if requestID := strings.TrimSpace(event.RequestID); requestID != "" {
                return requestID
        }
        if sessionID := strings.TrimSpace(event.AcctSessionID); sessionID != "" {
                return sessionID
        }
        return strings.TrimSpace(event.Decision)
}

func (event AuthEvent) partitionKey() (string, error) {
        if username := strings.TrimSpace(event.Username); username != "" {
                return messaging.NewPartitionKey("subscriber", username)
        }
        if sessionID := strings.TrimSpace(event.AcctSessionID); sessionID != "" {
                return messaging.NewPartitionKey("session", sessionID)
        }
        return messaging.NewPartitionKey("client_ip", strings.TrimSpace(event.ClientIP))
}

func (event AuthEvent) attributes() map[string]string {
        attributes := make(map[string]string, 10)

        addStringAttr(attributes, "username", event.Username)
        addStringAttr(attributes, "request_id", event.RequestID)
        addStringAttr(attributes, "decision", event.Decision)
        addStringAttr(attributes, "reject_reason", event.RejectReason)
        addStringAttr(attributes, "nas_ip_address", event.NASIPAddress)
        addStringAttr(attributes, "client_ip", event.ClientIP)
        addStringAttr(attributes, "calling_station_id", event.CallingStationID)
        addStringAttr(attributes, "called_station_id", event.CalledStationID)
        addStringAttr(attributes, "packet_type", event.PacketType)
        addStringAttr(attributes, "auth_protocol", event.AuthProtocol)
        addStringAttr(attributes, "policy_name", event.PolicyName)
        addStringAttr(attributes, "acct_session_id", event.AcctSessionID)
        return attributes
}

func addStringAttr(attributes map[string]string, key, value string) {
        value = strings.TrimSpace(value)
        if value == "" {
                return
        }
        attributes[key] = value
}

func sanitizeIDToken(value string) string {
        value = strings.TrimSpace(value)
        value = strings.ReplaceAll(value, " ", "_")
        return value
}

func normalizeOutcome(value string) (unifiedaccess.Outcome, error) {
        switch strings.ToLower(strings.TrimSpace(value)) {
        case "access-accept", "accepted", "accept":
                return unifiedaccess.OutcomeAccepted, nil
        case "access-reject", "rejected", "reject":
                return unifiedaccess.OutcomeRejected, nil
        case "access-challenge", "challenged", "challenge":
                return unifiedaccess.OutcomeChallenged, nil
        default:
                return "", fmt.Errorf("unsupported access decision %q", value)
        }
}

func tagsForOutcome(outcome unifiedaccess.Outcome) []string {
        return []string{"aaa", "auth", string(outcome)}
}

func buildSemanticText(event AuthEvent, outcome unifiedaccess.Outcome) string {
        parts := []string{
                "user " + strings.TrimSpace(event.Username),
        }

        switch outcome {
        case unifiedaccess.OutcomeAccepted:
                parts = append(parts, "authenticated successfully")
        case unifiedaccess.OutcomeRejected:
                parts = append(parts, "authentication was rejected")
        case unifiedaccess.OutcomeChallenged:
                parts = append(parts, "authentication requires additional challenge")
        }

        if nas := strings.TrimSpace(event.NASIPAddress); nas != "" {
                parts = append(parts, "on NAS "+nas)
        }
        if clientIP := strings.TrimSpace(event.ClientIP); clientIP != "" {
                parts = append(parts, "from client "+clientIP)
        }
        if protocol := strings.TrimSpace(event.AuthProtocol); protocol != "" {
                parts = append(parts, "using "+protocol)
        }
        if policyName := strings.TrimSpace(event.PolicyName); policyName != "" {
                parts = append(parts, "under policy "+policyName)
        }
        if outcome == unifiedaccess.OutcomeRejected {
                if reason := strings.TrimSpace(event.RejectReason); reason != "" {
                        parts = append(parts, "because "+reason)
                }
        }
        return strings.Join(parts, " ")
}

func cloneAttributes(attributes map[string]string) map[string]string {
        if attributes == nil {
                return nil
        }
        cloned := make(map[string]string, len(attributes))
        for key, value := range attributes {
                cloned[key] = value
        }
        return cloned
}
