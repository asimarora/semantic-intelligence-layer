package runtime

import (
        "context"
        "errors"
        "fmt"
        "io"
        "log/slog"
        "sort"
        "strings"
        "time"

        agenttypes "github.com/asimarora/semantic-intelligence-layer/internal/agents"
        agentharness "github.com/asimarora/semantic-intelligence-layer/internal/agents/harness"
        unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
        sessionquery "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
        agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
        sessionstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
)

const repeatedShortSessionWatcher = "repeated-short-session-disconnects"

type Config struct {
        Tenants               []string
        PollInterval          time.Duration
        Window                time.Duration
        MinEvents             int
        ShortSessionThreshold time.Duration
        MaxRunsPerCycle       int
}

type Dependencies struct {
        Logger       *slog.Logger
        Harness      *agentharness.Service
        Sessions     *sessionquery.SessionService
        WatcherState agentmetadata.WatcherStateStore
}

type Service struct {
        baseLogger   *slog.Logger
        harness      *agentharness.Service
        sessions     *sessionquery.SessionService
        watcherState agentmetadata.WatcherStateStore
        config       Config
}

type Result struct {
        Candidates      int
        Triggered       int
        Skipped         int
        TriggeredRunIDs []string
}

type candidate struct {
        TenantID   string
        Subscriber string
        Count      int
        Latest     unifiedsessions.Event
}

func NewService(cfg Config, deps Dependencies) (*Service, error) {
        cfg = normalizeConfig(cfg)
        if err := validateConfig(cfg); err != nil {
                return nil, err
        }
        if deps.Harness == nil {
                return nil, fmt.Errorf("agent harness is required")
        }
        if deps.Sessions == nil {
                return nil, fmt.Errorf("session query service is required")
        }
        if deps.WatcherState == nil {
                return nil, fmt.Errorf("watcher state store is required")
        }

        return &Service{
                baseLogger:   deps.Logger,
                harness:      deps.Harness,
                sessions:     deps.Sessions,
                watcherState: deps.WatcherState,
                config:       cfg,
        }, nil
}

func (service *Service) Run(ctx context.Context) error {
        if ctx == nil {
                ctx = context.Background()
        }

        logger := service.logger()
        logger.Info(
                "starting agent watcher runtime",
                "tenants", service.config.Tenants,
                "poll_interval", service.config.PollInterval.String(),
                "window", service.config.Window.String(),
                "min_events", service.config.MinEvents,
                "short_session_threshold", service.config.ShortSessionThreshold.String(),
                "max_runs_per_cycle", service.config.MaxRunsPerCycle,
        )

        runCycle := func() error {
                result, err := service.RunCycle(ctx, time.Now().UTC())
                if err != nil {
                        logger.Error("agent watcher cycle failed", "error", err, "triggered", result.Triggered, "skipped", result.Skipped)
                        return err
                }
                logger.Info("agent watcher cycle completed", "candidates", result.Candidates, "triggered", result.Triggered, "skipped", result.Skipped)
                return nil
        }

        if err := runCycle(); err != nil && ctx.Err() != nil {
                return ctx.Err()
        }

        ticker := time.NewTicker(service.config.PollInterval)
        defer ticker.Stop()

        for {
                select {
                case <-ctx.Done():
                        logger.Info("stopping agent watcher runtime")
                        return ctx.Err()
                case <-ticker.C:
                        if err := runCycle(); err != nil && ctx.Err() != nil {
                                return ctx.Err()
                        }
                }
        }
}

func (service *Service) RunCycle(ctx context.Context, now time.Time) (Result, error) {
        if ctx == nil {
                ctx = context.Background()
        }
        now = now.UTC()

        var (
                result Result
                errs   []error
        )

        for _, tenantID := range service.config.Tenants {
                from := now.Add(-service.config.Window)
                events, err := service.sessions.SearchNetworkSessions(ctx, sessionstore.Query{
                        TenantID: tenantID,
                        From:     &from,
                        To:       &now,
                        Limit:    sessionquery.MaxSessionLimit,
                })
                if err != nil {
                        errs = append(errs, fmt.Errorf("search recent sessions for tenant %s: %w", tenantID, err))
                        continue
                }

                candidates := service.detectCandidates(tenantID, events)
                result.Candidates += len(candidates)
                triggeredForTenant := 0

                for _, candidate := range candidates {
                        if triggeredForTenant >= service.config.MaxRunsPerCycle {
                                break
                        }

                        key := watcherStateKey(candidate.TenantID, candidate.Subscriber)
                        state, err := service.watcherState.GetWatcherState(ctx, key)
                        switch {
                        case err == nil && state.LastEventID == candidate.Latest.EventID:
                                result.Skipped++
                                continue
                        case err != nil && !errors.Is(err, agentmetadata.ErrNotFound):
                                errs = append(errs, fmt.Errorf("load watcher state for %s: %w", key, err))
                                continue
                        }

                        runRecord, runErr := service.harness.RunInvestigation(ctx, buildRunRequest(candidate, now, service.config.Window))
                        switch {
                        case runErr == nil:
                        case errors.Is(runErr, agentharness.ErrApprovalRequired):
                        default:
                                errs = append(errs, fmt.Errorf("trigger investigation for subscriber %s: %w", candidate.Subscriber, runErr))
                                continue
                        }

                        state = agentmetadata.WatcherState{
                                Key:            key,
                                WatcherName:    repeatedShortSessionWatcher,
                                TenantID:       candidate.TenantID,
                                SubjectID:      candidate.Subscriber,
                                LastEventID:    candidate.Latest.EventID,
                                LastOccurredAt: cloneTime(candidate.Latest.OccurredAt),
                                LastRunID:      runRecord.Run.ID,
                                UpdatedAt:      now,
                        }
                        if err := service.watcherState.UpsertWatcherState(ctx, state); err != nil {
                                errs = append(errs, fmt.Errorf("store watcher state for %s: %w", key, err))
                                continue
                        }

                        result.Triggered++
                        result.TriggeredRunIDs = append(result.TriggeredRunIDs, runRecord.Run.ID)
                        triggeredForTenant++
                }
        }

        return result, errors.Join(errs...)
}

func (service *Service) detectCandidates(tenantID string, events []unifiedsessions.Event) []candidate {
        grouped := make(map[string]candidate)
        for _, event := range events {
                if !qualifiesForRepeatedDisconnect(event, service.config.ShortSessionThreshold) {
                        continue
                }

                existing, ok := grouped[event.SubscriberID]
                if !ok {
                        grouped[event.SubscriberID] = candidate{
                                TenantID:   tenantID,
                                Subscriber: event.SubscriberID,
                                Count:      1,
                                Latest:     event,
                        }
                        continue
                }

                existing.Count++
                if event.OccurredAt.After(existing.Latest.OccurredAt) {
                        existing.Latest = event
                }
                grouped[event.SubscriberID] = existing
        }

        candidates := make([]candidate, 0, len(grouped))
        for _, candidate := range grouped {
                if candidate.Count < service.config.MinEvents {
                        continue
                }
                candidates = append(candidates, candidate)
        }

        sort.Slice(candidates, func(left, right int) bool {
                if candidates[left].Count == candidates[right].Count {
                        if candidates[left].Latest.OccurredAt.Equal(candidates[right].Latest.OccurredAt) {
                                return candidates[left].Subscriber < candidates[right].Subscriber
                        }
                        return candidates[left].Latest.OccurredAt.After(candidates[right].Latest.OccurredAt)
                }
                return candidates[left].Count > candidates[right].Count
        })
        return candidates
}

func qualifiesForRepeatedDisconnect(event unifiedsessions.Event, threshold time.Duration) bool {
        if strings.TrimSpace(event.SubscriberID) == "" {
                return false
        }
        if event.Status != unifiedsessions.StatusStop {
                return false
        }
        if event.SessionTimeSeconds == nil {
                return false
        }
        return time.Duration(*event.SessionTimeSeconds)*time.Second <= threshold
}

func buildRunRequest(candidate candidate, now time.Time, window time.Duration) agenttypes.RunRequest {
        from := now.Add(-window).Format(time.RFC3339)
        to := now.Format(time.RFC3339)

        queryParts := []string{
                "repeated short session disconnect",
                "subscriber " + candidate.Subscriber,
                "stop",
        }
        if candidate.Latest.NASIPAddress != "" {
                queryParts = append(queryParts, "nas "+candidate.Latest.NASIPAddress)
        }

        inputs := map[string]string{
                "query":         strings.Join(queryParts, " "),
                "subscriber_id": candidate.Subscriber,
                "status":        "stop",
                "from":          from,
                "to":            to,
                "limit":         "10",
        }
        if candidate.Latest.NASIPAddress != "" {
                inputs["nas_ip_address"] = candidate.Latest.NASIPAddress
        }
        if candidate.Latest.ClientIPAddress != "" {
                inputs["client_ip_address"] = candidate.Latest.ClientIPAddress
        }

        return agenttypes.RunRequest{
                TenantID:  candidate.TenantID,
                SessionID: composeWatcherSessionID(candidate.TenantID, candidate.Subscriber),
                Goal:      fmt.Sprintf("Investigate repeated short-session disconnects for subscriber %s", candidate.Subscriber),
                Inputs:    inputs,
                MaxSteps:  3,
        }
}

func composeWatcherSessionID(tenantID, subscriberID string) string {
        return strings.Join([]string{
                "watcher",
                sanitizeWatcherIDPart(repeatedShortSessionWatcher),
                sanitizeWatcherIDPart(tenantID),
                sanitizeWatcherIDPart(subscriberID),
        }, ":")
}

func watcherStateKey(tenantID, subscriberID string) string {
        return strings.Join([]string{
                "watcher-state",
                sanitizeWatcherIDPart(repeatedShortSessionWatcher),
                sanitizeWatcherIDPart(tenantID),
                sanitizeWatcherIDPart(subscriberID),
        }, ":")
}

func normalizeConfig(cfg Config) Config {
        cfg.Tenants = normalizeTenants(cfg.Tenants)
        return cfg
}

func validateConfig(cfg Config) error {
        if len(cfg.Tenants) == 0 {
                return fmt.Errorf("at least one tenant is required")
        }
        if cfg.PollInterval <= 0 {
                return fmt.Errorf("poll interval must be positive")
        }
        if cfg.Window <= 0 {
                return fmt.Errorf("window must be positive")
        }
        if cfg.MinEvents <= 0 {
                return fmt.Errorf("min events must be positive")
        }
        if cfg.ShortSessionThreshold <= 0 {
                return fmt.Errorf("short session threshold must be positive")
        }
        if cfg.MaxRunsPerCycle <= 0 {
                return fmt.Errorf("max runs per cycle must be positive")
        }
        return nil
}

func normalizeTenants(values []string) []string {
        seen := make(map[string]struct{})
        tenants := make([]string, 0, len(values))
        for _, value := range values {
                value = strings.TrimSpace(value)
                if value == "" {
                        continue
                }
                if _, ok := seen[value]; ok {
                        continue
                }
                seen[value] = struct{}{}
                tenants = append(tenants, value)
        }
        sort.Strings(tenants)
        return tenants
}

func sanitizeWatcherIDPart(value string) string {
        value = strings.TrimSpace(strings.ToLower(value))
        if value == "" {
                return ""
        }

        var builder strings.Builder
        for _, r := range value {
                switch {
                case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
                        builder.WriteRune(r)
                case r == '-', r == '_', r == '.':
                        builder.WriteRune(r)
                default:
                        builder.WriteRune('-')
                }
        }
        return strings.Trim(builder.String(), "-")
}

func cloneTime(value time.Time) *time.Time {
        value = value.UTC()
        return &value
}

func (service *Service) logger() *slog.Logger {
        if service != nil && service.baseLogger != nil {
                return service.baseLogger
        }
        return slog.New(slog.NewTextHandler(io.Discard, nil))
}
