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

                var event unifiedsessions.Event
                if err := json.Unmarshal(line, &event); err != nil {
                        return result, fmt.Errorf("decode unified event: %w", err)
                }
                if err := event.Validate(); err != nil {
                        return result, fmt.Errorf("validate unified event %s: %w", event.EventID, err)
                }

                document, err := buildDocument(event)
                if err != nil {
                        return result, fmt.Errorf("index unified event %s: %w", event.EventID, err)
                }
                if err := service.Store.Append(ctx, document); err != nil {
                        return result, fmt.Errorf("store retrieval document %s: %w", document.DocumentID, err)
                }

                result.Events++
                result.Indexed++
                logger.Debug(
                        "indexed unified session event",
                        "tenant_id", event.TenantID,
                        "event_id", event.EventID,
                        "session_id", event.SessionID,
                        "subscriber_id", event.SubscriberID,
                )
        }
        if err := scanner.Err(); err != nil {
                return result, fmt.Errorf("scan unified session events: %w", err)
        }

        logger.Info("retrieval indexing completed", "events", result.Events, "indexed", result.Indexed)
        return result, nil
}

func buildDocument(event unifiedsessions.Event) (retrievalcontracts.Document, error) {
        if err := event.Validate(); err != nil {
                return retrievalcontracts.Document{}, err
        }

        attributes := make(map[string]string, len(event.Attributes))
        termValues := []string{
                buildTitle(event),
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
                Title:           buildTitle(event),
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

func buildTitle(event unifiedsessions.Event) string {
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

func (service Service) logger() *slog.Logger {
        if service.Logger != nil {
                return service.Logger
        }
        return slog.New(slog.NewTextHandler(io.Discard, nil))
}
