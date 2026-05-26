package radius

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	rawevidence "github.com/asimarora/semantic-intelligence-layer/internal/contracts/raw/evidence"
	unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
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

func NormalizeRecord(record rawevidence.Record) (unifiedsessions.Event, error) {
	event, err := DecodeRecord(record)
	if err != nil {
		return unifiedsessions.Event{}, err
	}

	status, err := normalizeStatus(event.AcctStatusType)
	if err != nil {
		return unifiedsessions.Event{}, err
	}

	normalized := unifiedsessions.Event{
		EventID:            record.EventID + ":unified",
		SourceEventID:      record.EventID,
		Source:             record.Source,
		SourceKey:          record.SourceKey,
		SchemaVersion:      unifiedsessions.SchemaVersion,
		EventType:          unifiedsessions.EventType,
		TenantID:           record.TenantID,
		OccurredAt:         record.OccurredAt,
		IngestedAt:         record.IngestedAt,
		SubscriberID:       strings.TrimSpace(event.Username),
		SessionID:          strings.TrimSpace(event.AcctSessionID),
		Status:             status,
		NASIPAddress:       strings.TrimSpace(event.NASIPAddress),
		NASPort:            cloneInt(event.NASPort),
		ClientIPAddress:    strings.TrimSpace(event.ClientIP),
		AssignedIPAddress:  strings.TrimSpace(event.FramedIPAddress),
		CallingStationID:   strings.TrimSpace(event.CallingStationID),
		CalledStationID:    strings.TrimSpace(event.CalledStationID),
		PacketType:         strings.TrimSpace(event.PacketType),
		SessionTimeSeconds: cloneUint64(event.AcctSessionTime),
		InputOctets:        cloneUint64(event.AcctInputOctets),
		OutputOctets:       cloneUint64(event.AcctOutputOctets),
		Tags:               tagsForStatus(status),
		SemanticText:       buildSemanticText(event, status),
		Attributes:         cloneAttributes(record.Attributes),
	}
	if err := normalized.Validate(); err != nil {
		return unifiedsessions.Event{}, err
	}
	return normalized, nil
}

func DecodeRecord(record rawevidence.Record) (AccountingEvent, error) {
	if err := record.Validate(); err != nil {
		return AccountingEvent{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(record.Source), Source) {
		return AccountingEvent{}, fmt.Errorf("unsupported evidence source %q", record.Source)
	}

	var event AccountingEvent
	if err := json.Unmarshal(record.Payload, &event); err != nil {
		return AccountingEvent{}, fmt.Errorf("decode radius evidence payload: %w", err)
	}
	if err := event.Validate(); err != nil {
		return AccountingEvent{}, err
	}
	return event, nil
}

func normalizeStatus(value string) (unifiedsessions.Status, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "accounting-start", "start":
		return unifiedsessions.StatusStart, nil
	case "interim-update", "interim", "interim_update":
		return unifiedsessions.StatusInterimUpdate, nil
	case "accounting-stop", "stop":
		return unifiedsessions.StatusStop, nil
	default:
		return "", fmt.Errorf("unsupported accounting status %q", value)
	}
}

func tagsForStatus(status unifiedsessions.Status) []string {
	return []string{"radius", string(status)}
}

func buildSemanticText(event AccountingEvent, status unifiedsessions.Status) string {
	parts := []string{"user " + strings.TrimSpace(event.Username)}
	switch status {
	case unifiedsessions.StatusStart:
		parts = append(parts, "started a network session")
	case unifiedsessions.StatusInterimUpdate:
		parts = append(parts, "reported an interim network session update")
	case unifiedsessions.StatusStop:
		parts = append(parts, "disconnected")
	}
	if nas := strings.TrimSpace(event.NASIPAddress); nas != "" {
		parts = append(parts, "from NAS "+nas)
	}
	if status == unifiedsessions.StatusStop && event.AcctSessionTime != nil {
		parts = append(parts, fmt.Sprintf("after a %d second session", *event.AcctSessionTime))
	}
	if event.AcctInputOctets != nil {
		parts = append(parts, fmt.Sprintf("with %d input octets", *event.AcctInputOctets))
	}
	if event.AcctOutputOctets != nil {
		parts = append(parts, fmt.Sprintf("and %d output octets", *event.AcctOutputOctets))
	}
	return strings.Join(parts, " ")
}

func cloneInt(value *int) *int {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneUint64(value *uint64) *uint64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func cloneAttributes(values map[string]string) map[string]string {
	if values == nil {
		return nil
	}
	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}
