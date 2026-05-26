package index

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"sort"
	"strings"

	unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
	retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
	unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
	retrievalstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
)

type Service struct {
	Logger *slog.Logger
	Store  retrievalstore.Store
}

type Result struct {
	Events  int
	Indexed int
}

func (service Service) Run(ctx context.Context, reader io.Reader) (Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if service.Store == nil {
		return Result{}, fmt.Errorf("retrieval metadata store is required")
	}
	if reader == nil {
		return Result{}, fmt.Errorf("input reader is required")
	}

	logger := service.logger()
	logger.Info("starting retrieval indexing")

	scanner := bufio.NewScanner(reader)
	buffer := make([]byte, 0, 64*1024)
	scanner.Buffer(buffer, 1024*1024)

	var result Result
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return result, err
		}

		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}

		document, eventID, err := buildDocument(line)
		if err != nil {
			return result, fmt.Errorf("index unified event: %w", err)
		}
		if err := service.Store.Append(ctx, document); err != nil {
			return result, fmt.Errorf("store retrieval document %s: %w", document.DocumentID, err)
		}

		result.Events++
		result.Indexed++
		logger.Debug(
			"indexed unified evidence event",
			"tenant_id", document.TenantID,
			"event_id", eventID,
			"document_id", document.DocumentID,
			"entity_type", document.EntityType,
			"subscriber_id", document.SubscriberID,
		)
	}
	if err := scanner.Err(); err != nil {
		return result, fmt.Errorf("scan unified evidence events: %w", err)
	}

	logger.Info("retrieval indexing completed", "events", result.Events, "indexed", result.Indexed)
	return result, nil
}

func buildDocument(line []byte) (retrievalcontracts.Document, string, error) {
	var probe struct {
		EventID   string `json:"event_id"`
		EventType string `json:"event_type"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return retrievalcontracts.Document{}, "", fmt.Errorf("decode unified event: %w", err)
	}

	switch strings.TrimSpace(probe.EventType) {
	case unifiedsessions.EventType:
		var event unifiedsessions.Event
		if err := json.Unmarshal(line, &event); err != nil {
			return retrievalcontracts.Document{}, "", fmt.Errorf("decode unified session event: %w", err)
		}
		if err := event.Validate(); err != nil {
			return retrievalcontracts.Document{}, "", fmt.Errorf("validate unified session event %s: %w", event.EventID, err)
		}
		document, err := buildSessionDocument(event)
		return document, event.EventID, err
	case unifiedaccess.EventType:
		var event unifiedaccess.Event
		if err := json.Unmarshal(line, &event); err != nil {
			return retrievalcontracts.Document{}, "", fmt.Errorf("decode unified access event: %w", err)
		}
		if err := event.Validate(); err != nil {
			return retrievalcontracts.Document{}, "", fmt.Errorf("validate unified access event %s: %w", event.EventID, err)
		}
		document, err := buildAccessDocument(event)
		return document, event.EventID, err
	default:
		return retrievalcontracts.Document{}, probe.EventID, fmt.Errorf("unsupported unified event type %q", probe.EventType)
	}
}

func buildSessionDocument(event unifiedsessions.Event) (retrievalcontracts.Document, error) {
	if err := event.Validate(); err != nil {
		return retrievalcontracts.Document{}, err
	}

	attributes := make(map[string]string, len(event.Attributes))
	termValues := []string{
		buildSessionTitle(event),
		event.SemanticText,
		event.Source,
		event.EventType,
		event.TenantID,
		event.SubscriberID,
		event.SessionID,
		string(event.Status),
		event.NASIPAddress,
		event.ClientIPAddress,
	}

	for _, tag := range event.Tags {
		termValues = append(termValues, tag)
	}

	keys := make([]string, 0, len(event.Attributes))
	for key := range event.Attributes {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := strings.TrimSpace(event.Attributes[key])
		attributes[key] = value
		termValues = append(termValues, key, value)
	}

	document := retrievalcontracts.Document{
		DocumentID:      event.EventID,
		SchemaVersion:   retrievalcontracts.SchemaVersion,
		TenantID:        event.TenantID,
		Source:          event.Source,
		SourceEventID:   event.SourceEventID,
		SourceKey:       event.SourceKey,
		EntityType:      retrievalcontracts.EntityTypeNetworkSession,
		EventType:       event.EventType,
		OccurredAt:      event.OccurredAt,
		IngestedAt:      event.IngestedAt,
		Title:           buildSessionTitle(event),
		Content:         event.SemanticText,
		SubscriberID:    event.SubscriberID,
		SessionID:       event.SessionID,
		Status:          string(event.Status),
		NASIPAddress:    event.NASIPAddress,
		ClientIPAddress: event.ClientIPAddress,
		Tags:            append([]string(nil), event.Tags...),
		Terms:           retrievalcontracts.NormalizeTerms(termValues...),
		Attributes:      attributes,
	}
	return document, document.Validate()
}

func buildAccessDocument(event unifiedaccess.Event) (retrievalcontracts.Document, error) {
	if err := event.Validate(); err != nil {
		return retrievalcontracts.Document{}, err
	}

	attributes, attributeTerms := cloneAttributes(event.Attributes)
	termValues := []string{
		buildAccessTitle(event),
		event.SemanticText,
		event.Source,
		event.EventType,
		event.TenantID,
		event.SubscriberID,
		event.SessionID,
		event.RequestID,
		string(event.Outcome),
		event.NASIPAddress,
		event.ClientIPAddress,
		event.PolicyName,
		event.RejectReason,
	}
	termValues = append(termValues, attributeTerms...)
	for _, tag := range event.Tags {
		termValues = append(termValues, tag)
	}

	document := retrievalcontracts.Document{
		DocumentID:      event.EventID,
		SchemaVersion:   retrievalcontracts.SchemaVersion,
		TenantID:        event.TenantID,
		Source:          event.Source,
		SourceEventID:   event.SourceEventID,
		SourceKey:       event.SourceKey,
		EntityType:      retrievalcontracts.EntityTypeNetworkAccess,
		EventType:       event.EventType,
		OccurredAt:      event.OccurredAt,
		IngestedAt:      event.IngestedAt,
		Title:           buildAccessTitle(event),
		Content:         event.SemanticText,
		SubscriberID:    event.SubscriberID,
		SessionID:       event.SessionID,
		RequestID:       event.RequestID,
		Outcome:         string(event.Outcome),
		NASIPAddress:    event.NASIPAddress,
		ClientIPAddress: event.ClientIPAddress,
		PolicyName:      event.PolicyName,
		RejectReason:    event.RejectReason,
		Tags:            append([]string(nil), event.Tags...),
		Terms:           retrievalcontracts.NormalizeTerms(termValues...),
		Attributes:      attributes,
	}
	return document, document.Validate()
}

func buildSessionTitle(event unifiedsessions.Event) string {
	status := strings.ReplaceAll(string(event.Status), "_", " ")
	title := fmt.Sprintf("network session %s for subscriber %s", status, event.SubscriberID)
	if event.SessionID != "" {
		title += " in session " + event.SessionID
	}
	if event.NASIPAddress != "" {
		title += " on NAS " + event.NASIPAddress
	}
	return title
}

func buildAccessTitle(event unifiedaccess.Event) string {
	title := fmt.Sprintf("network access %s for subscriber %s", event.Outcome, event.SubscriberID)
	if event.RequestID != "" {
		title += " on request " + event.RequestID
	}
	if event.NASIPAddress != "" {
		title += " on NAS " + event.NASIPAddress
	}
	return title
}

func cloneAttributes(values map[string]string) (map[string]string, []string) {
	if len(values) == 0 {
		return nil, nil
	}

	attributes := make(map[string]string, len(values))
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	terms := make([]string, 0, len(values)*2)
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		attributes[key] = value
		terms = append(terms, key, value)
	}
	return attributes, terms
}

func (service Service) logger() *slog.Logger {
	if service.Logger != nil {
		return service.Logger
	}
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
