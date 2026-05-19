package normalize

import (
        "context"
        "encoding/json"
        "fmt"
        "io"
        "log/slog"

        "github.com/asimarora/semantic-intelligence-layer/internal/adapters/radius"
        unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
        "github.com/asimarora/semantic-intelligence-layer/internal/platform/messaging"
        rawstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/raw"
)

type Service struct {
        Logger         *slog.Logger
        Store          rawstore.Store
        Publisher      messaging.Publisher
        UnifiedSubject string
}

type Result struct {
        Records   int
        Published int
}

func (service Service) Run(ctx context.Context, writer io.Writer) (Result, error) {
        if ctx == nil {
                ctx = context.Background()
        }
        if service.Store == nil {
                return Result{}, fmt.Errorf("raw store is required")
        }
        if writer == nil {
                return Result{}, fmt.Errorf("output writer is required")
        }

        records, err := service.Store.Replay(ctx, radius.Source)
        if err != nil {
                return Result{}, err
        }
        if len(records) == 0 {
                return Result{}, fmt.Errorf("no raw %s evidence records available", radius.Source)
        }

        logger := service.logger().With("source", radius.Source, "records", len(records))
        logger.Info("starting normalization")

        encoder := json.NewEncoder(writer)
        encoder.SetEscapeHTML(false)

        var result Result
        for _, record := range records {
                if err := ctx.Err(); err != nil {
                        return result, err
                }

                event, err := radius.NormalizeRecord(record)
                if err != nil {
                        return result, fmt.Errorf("normalize %s: %w", record.EventID, err)
                }
                if err := encoder.Encode(event); err != nil {
                        return result, fmt.Errorf("write unified event %s: %w", event.EventID, err)
                }

                result.Records++

                if service.Publisher != nil {
                        envelope, err := buildEnvelope(event)
                        if err != nil {
                                return result, err
                        }
                        if _, err := service.Publisher.PublishEnvelope(ctx, service.UnifiedSubject, envelope); err != nil {
                                return result, fmt.Errorf("publish unified event %s: %w", event.EventID, err)
                        }
                        result.Published++
                }

                logger.Debug(
                        "normalized radius evidence",
                        "source_event_id", event.SourceEventID,
                        "event_id", event.EventID,
                        "session_id", event.SessionID,
                        "status", event.Status,
                )
        }

        logger.Info("normalization completed", "records", result.Records, "published", result.Published)
        return result, nil
}

func buildEnvelope(event unifiedsessions.Event) (messaging.Envelope, error) {
        payload, err := json.Marshal(event)
        if err != nil {
                return messaging.Envelope{}, fmt.Errorf("marshal unified event %s: %w", event.EventID, err)
        }

        partitionKey, err := partitionKey(event)
        if err != nil {
                return messaging.Envelope{}, err
        }

        envelope := messaging.Envelope{
                EventID:       event.EventID,
                Source:        event.Source,
                SourceKey:     event.SourceKey,
                SchemaVersion: event.SchemaVersion,
                PartitionKey:  partitionKey,
                OccurredAt:    event.OccurredAt,
                IngestedAt:    event.IngestedAt,
                Payload:       payload,
        }
        if err := envelope.Validate(); err != nil {
                return messaging.Envelope{}, err
        }
        return envelope, nil
}

func partitionKey(event unifiedsessions.Event) (string, error) {
        if event.SubscriberID != "" {
                return messaging.NewPartitionKey("subscriber", event.SubscriberID)
        }
        return messaging.NewPartitionKey("session", event.SessionID)
}

func (service Service) logger() *slog.Logger {
        if service.Logger != nil {
                return service.Logger
        }
        return slog.New(slog.NewTextHandler(io.Discard, nil))
}
