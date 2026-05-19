package radius

import (
        "bytes"
        "encoding/json"
        "fmt"
        "strconv"
        "strings"
        "time"

        rawevidence "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/evidence"
        "github.com/asimarora/semantic-intelligence-layer/internal/platform/messaging"
)

const sourceKeyTimeFormat = "20060102T150405.000000"

type AccountingEvent struct {
        Username         string    `json:"username"`
        NASIPAddress     string    `json:"nas_ip_address"`
        NASPort          *int      `json:"nas_port"`
        AcctStatusType   string    `json:"acct_status_type"`
        AcctSessionID    string    `json:"acct_session_id"`
        FramedIPAddress  string    `json:"framed_ip_address"`
        CallingStationID string    `json:"calling_station_id"`
        CalledStationID  string    `json:"called_station_id"`
        Timestamp        time.Time `json:"timestamp"`
        ClientIP         string    `json:"client_ip"`
        PacketType       string    `json:"packet_type"`
        AcctInputOctets  *uint64   `json:"acct_input_octets"`
        AcctOutputOctets *uint64   `json:"acct_output_octets"`
        AcctSessionTime  *uint64   `json:"acct_session_time"`
}

type AdaptedRecord struct {
        Event    AccountingEvent
        Record   rawevidence.Record
        Envelope messaging.Envelope
}

func Adapt(cfg Config, payload []byte, ingestedAt time.Time) (AdaptedRecord, error) {
        if err := cfg.Validate(); err != nil {
                return AdaptedRecord{}, err
        }

        payload = bytes.TrimSpace(payload)
        if len(payload) == 0 {
                return AdaptedRecord{}, fmt.Errorf("radius payload is empty")
        }
        if !json.Valid(payload) {
                return AdaptedRecord{}, fmt.Errorf("radius payload must be valid JSON")
        }

        var event AccountingEvent
        if err := json.Unmarshal(payload, &event); err != nil {
                return AdaptedRecord{}, fmt.Errorf("decode radius payload: %w", err)
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

func (event AccountingEvent) Validate() error {
        if strings.TrimSpace(event.Username) == "" {
                return fmt.Errorf("radius username is required")
        }
        if strings.TrimSpace(event.AcctSessionID) == "" {
                return fmt.Errorf("radius acct_session_id is required")
        }
        if strings.TrimSpace(event.AcctStatusType) == "" {
                return fmt.Errorf("radius acct_status_type is required")
        }
        if event.Timestamp.IsZero() {
                return fmt.Errorf("radius timestamp is required")
        }
        return nil
}

func (event AccountingEvent) eventID() string {
        return fmt.Sprintf(
                "%s:%s:%s",
                Source,
                sanitizeIDToken(event.AcctSessionID),
                event.Timestamp.UTC().Format(time.RFC3339Nano),
        )
}

func (event AccountingEvent) sourceKey() string {
        return fmt.Sprintf(
                "%s:acct:%s:%s:%s",
                Kind,
                sanitizeIDToken(event.Username),
                sanitizeIDToken(event.AcctSessionID),
                event.Timestamp.UTC().Format(sourceKeyTimeFormat),
        )
}

func (event AccountingEvent) partitionKey() (string, error) {
        if username := strings.TrimSpace(event.Username); username != "" {
                return messaging.NewPartitionKey("subscriber", username)
        }
        return messaging.NewPartitionKey("session", strings.TrimSpace(event.AcctSessionID))
}

func (event AccountingEvent) attributes() map[string]string {
        attributes := make(map[string]string, 11)

        addStringAttr(attributes, "username", event.Username)
        addStringAttr(attributes, "acct_status_type", event.AcctStatusType)
        addStringAttr(attributes, "acct_session_id", event.AcctSessionID)
        addStringAttr(attributes, "nas_ip_address", event.NASIPAddress)
        addStringAttr(attributes, "framed_ip_address", event.FramedIPAddress)
        addStringAttr(attributes, "calling_station_id", event.CallingStationID)
        addStringAttr(attributes, "called_station_id", event.CalledStationID)
        addStringAttr(attributes, "client_ip", event.ClientIP)
        addStringAttr(attributes, "packet_type", event.PacketType)
        addIntAttr(attributes, "nas_port", event.NASPort)
        addUint64Attr(attributes, "acct_input_octets", event.AcctInputOctets)
        addUint64Attr(attributes, "acct_output_octets", event.AcctOutputOctets)
        addUint64Attr(attributes, "acct_session_time", event.AcctSessionTime)

        return attributes
}

func addStringAttr(attributes map[string]string, key, value string) {
        value = strings.TrimSpace(value)
        if value == "" {
                return
        }
        attributes[key] = value
}

func addIntAttr(attributes map[string]string, key string, value *int) {
        if value == nil {
                return
        }
        attributes[key] = strconv.Itoa(*value)
}

func addUint64Attr(attributes map[string]string, key string, value *uint64) {
        if value == nil {
                return
        }
        attributes[key] = strconv.FormatUint(*value, 10)
}

func sanitizeIDToken(value string) string {
        value = strings.TrimSpace(value)
        value = strings.ReplaceAll(value, " ", "_")
        return value
}
