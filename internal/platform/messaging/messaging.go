package messaging

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	silconfig "github.com/asimarora/semantic-intelligence-layer/internal/platform/config"
	"github.com/nats-io/nats.go"
)

const (
	DefaultStreamName    = "SIL_EVENTS"
	SubjectRawEvents     = "sil.raw.events"
	SubjectUnifiedEvents = "sil.unified.events"
	SubjectIndexJobs     = "sil.index.jobs"
	SubjectDeadLetter    = "sil.deadletter"
)

type Envelope struct {
	EventID       string          `json:"event_id"`
	Source        string          `json:"source"`
	SourceKey     string          `json:"source_key"`
	SchemaVersion string          `json:"schema_version"`
	PartitionKey  string          `json:"partition_key"`
	OccurredAt    time.Time       `json:"occurred_at"`
	IngestedAt    time.Time       `json:"ingested_at"`
	Payload       json.RawMessage `json:"payload"`
}

func (envelope Envelope) Validate() error {
	if strings.TrimSpace(envelope.EventID) == "" {
		return fmt.Errorf("event_id is required")
	}
	if strings.TrimSpace(envelope.Source) == "" {
		return fmt.Errorf("source is required")
	}
	if strings.TrimSpace(envelope.SourceKey) == "" {
		return fmt.Errorf("source_key is required")
	}
	if strings.TrimSpace(envelope.SchemaVersion) == "" {
		return fmt.Errorf("schema_version is required")
	}
	if strings.TrimSpace(envelope.PartitionKey) == "" {
		return fmt.Errorf("partition_key is required")
	}
	if envelope.OccurredAt.IsZero() {
		return fmt.Errorf("occurred_at is required")
	}
	if envelope.IngestedAt.IsZero() {
		return fmt.Errorf("ingested_at is required")
	}
	if len(envelope.Payload) == 0 {
		return fmt.Errorf("payload is required")
	}
	if !json.Valid(envelope.Payload) {
		return fmt.Errorf("payload must be valid JSON")
	}
	return nil
}

type Publisher interface {
	PublishEnvelope(context.Context, string, Envelope) (*nats.PubAck, error)
}

type Client struct {
	conn *nats.Conn
}

func Connect(cfg silconfig.StreamConfig) (*Client, error) {
	if strings.EqualFold(strings.TrimSpace(cfg.Backend), "disabled") {
		return nil, fmt.Errorf("stream backend is disabled")
	}
	if strings.TrimSpace(cfg.Addr) == "" {
		return nil, fmt.Errorf("stream address is required")
	}

	options := []nats.Option{
		nats.Name("semantic-intelligence-layer"),
		nats.Timeout(timeoutOrDefault(cfg.ConnectTimeout, 5*time.Second)),
	}
	if cfg.Username != "" {
		options = append(options, nats.UserInfo(cfg.Username, cfg.Password))
	}
	if cfg.TLSEnabled {
		options = append(options, nats.Secure(&tls.Config{MinVersion: tls.VersionTLS12}))
	}

	conn, err := nats.Connect(cfg.Addr, options...)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

func (client *Client) Close() {
	if client != nil && client.conn != nil {
		client.conn.Close()
	}
}

func (client *Client) PublishEnvelope(ctx context.Context, subject string, envelope Envelope) (*nats.PubAck, error) {
	if client == nil || client.conn == nil {
		return nil, fmt.Errorf("messaging client is not configured")
	}
	if err := envelope.Validate(); err != nil {
		return nil, err
	}
	subject = strings.TrimSpace(subject)
	if subject == "" {
		return nil, fmt.Errorf("subject is required")
	}

	js, err := client.conn.JetStream()
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return js.Publish(subject, envelope.Payload)
}

func (client *Client) EnsureConfiguredStream(ctx context.Context, cfg silconfig.StreamConfig) error {
	if client == nil || client.conn == nil {
		return fmt.Errorf("messaging client is not configured")
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Backend), "disabled") {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	js, err := client.conn.JetStream()
	if err != nil {
		return err
	}

	streamName := firstNonEmpty(strings.TrimSpace(cfg.StreamName), DefaultStreamName)
	subjects := []string{
		firstNonEmpty(strings.TrimSpace(cfg.RawSubject), SubjectRawEvents),
		firstNonEmpty(strings.TrimSpace(cfg.UnifiedSubject), SubjectUnifiedEvents),
		firstNonEmpty(strings.TrimSpace(cfg.IndexSubject), SubjectIndexJobs),
		firstNonEmpty(strings.TrimSpace(cfg.DeadLetterSubject), SubjectDeadLetter),
	}

	streamConfig := &nats.StreamConfig{
		Name:      streamName,
		Subjects:  subjects,
		Retention: nats.LimitsPolicy,
		Storage:   nats.FileStorage,
	}
	if strings.EqualFold(strings.TrimSpace(cfg.Storage), "memory") {
		streamConfig.Storage = nats.MemoryStorage
	}

	_, err = js.StreamInfo(streamName)
	switch {
	case err == nil:
		_, err = js.UpdateStream(streamConfig)
		return err
	case errors.Is(err, nats.ErrStreamNotFound):
		_, err = js.AddStream(streamConfig)
		return err
	default:
		return err
	}
}

func NewPartitionKey(kind, value string) (string, error) {
	kind = strings.TrimSpace(kind)
	value = strings.TrimSpace(value)
	if kind == "" {
		return "", fmt.Errorf("partition key kind is required")
	}
	if value == "" {
		return "", fmt.Errorf("partition key value is required")
	}
	return kind + ":" + value, nil
}

func timeoutOrDefault(value, fallback time.Duration) time.Duration {
	if value > 0 {
		return value
	}
	return fallback
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			return value
		}
	}
	return ""
}
