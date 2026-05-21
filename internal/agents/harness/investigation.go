package harness

import (
        "context"
        "errors"
        "fmt"
        "io"
        "log/slog"
        "sort"
        "strconv"
        "strings"
        "time"
        "unicode"

        agenttypes "github.com/asimarora/semantic-intelligence-layer/internal/agents"
        "github.com/asimarora/semantic-intelligence-layer/internal/agents/memory"
        "github.com/asimarora/semantic-intelligence-layer/internal/agents/policy"
        "github.com/asimarora/semantic-intelligence-layer/internal/agents/tools"
        retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
        unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
        sessionquery "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
        agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
        retrievalstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
        sessionstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
)

const (
        defaultMaxSteps       = 3
        maxEvidencePerStep    = 5
        maxSessionLookups     = 3
        defaultHarnessRuntime = "deterministic-harness"
        defaultHarnessPlanner = "deterministic-investigation-v1"
        inputKeyQuery         = "query"
        inputKeySubscriberID  = "subscriber_id"
        inputKeySessionID     = "session_id"
        inputKeyStatus        = "status"
        inputKeyNASIPAddress  = "nas_ip_address"
        inputKeyClientIP      = "client_ip_address"
        inputKeyFrom          = "from"
        inputKeyTo            = "to"
        inputKeyLimit         = "limit"
)

var (
        ErrApprovalRequired = errors.New("agent run awaiting approval")
        ErrPolicyDenied     = errors.New("agent action denied by policy")
)

type Service struct {
        baseLogger  *slog.Logger
        memory      memory.Store
        policy      policy.Evaluator
        tools       tools.Registry
        runs        agentmetadata.Store
        retrieval   *sessionquery.RetrievalService
        sessions    *sessionquery.SessionService
        runtimeName string
        plannerName string
}

func NewService(deps Dependencies) (*Service, error) {
        if deps.Runs == nil {
                return nil, fmt.Errorf("agent run store is required")
        }
        if deps.Retrieval == nil {
                return nil, fmt.Errorf("retrieval service is required")
        }

        policyEvaluator := deps.Policy
        if policyEvaluator == nil {
                policyEvaluator = policy.NewDefaultEvaluator()
        }

        return &Service{
                baseLogger:  deps.Logger,
                memory:      deps.Memory,
                policy:      policyEvaluator,
                tools:       deps.Tools,
                runs:        deps.Runs,
                retrieval:   deps.Retrieval,
                sessions:    deps.Sessions,
                runtimeName: firstNonEmpty(strings.TrimSpace(deps.RuntimeName), defaultHarnessRuntime),
                plannerName: firstNonEmpty(strings.TrimSpace(deps.PlannerName), defaultHarnessPlanner),
        }, nil
}

func (service *Service) StartRun(ctx context.Context, request agenttypes.RunRequest) (*agenttypes.Run, error) {
        record, err := service.RunInvestigation(ctx, request)
        if err != nil {
                return &record.Run, err
        }
        return &record.Run, nil
}

func (service *Service) ContinueRun(ctx context.Context, runID string) (*agenttypes.Run, error) {
        record, err := service.GetRun(ctx, runID)
        if err != nil {
                return nil, err
        }
        return &record.Run, nil
}

func (service *Service) GetRun(ctx context.Context, runID string) (agentmetadata.Record, error) {
        if service == nil || service.runs == nil {
                return agentmetadata.Record{}, fmt.Errorf("agent harness is not configured")
        }
        return service.runs.Get(ctx, runID)
}

func (service *Service) RunInvestigation(ctx context.Context, request agenttypes.RunRequest) (agentmetadata.Record, error) {
        if ctx == nil {
                ctx = context.Background()
        }
        if service == nil || service.runs == nil || service.retrieval == nil {
                return agentmetadata.Record{}, fmt.Errorf("agent harness is not configured")
        }

        now := time.Now().UTC()
        request, err := normalizeRequest(request, now)
        if err != nil {
                return agentmetadata.Record{}, err
        }

        run := agenttypes.Run{
                ID:          composeID("run", request.TenantID, request.SessionID, now),
                TenantID:    request.TenantID,
                CaseID:      request.CaseID,
                SessionID:   request.SessionID,
                Goal:        request.Goal,
                Status:      agenttypes.RunStatusRunning,
                RuntimeName: service.runtimeName,
                PlannerName: service.plannerName,
                StartedAt:   now,
                UpdatedAt:   now,
        }
        record := agentmetadata.Record{Run: run}
        if err := service.runs.Upsert(ctx, record); err != nil {
                return agentmetadata.Record{}, err
        }

        logger := service.logger().With("run_id", run.ID, "tenant_id", run.TenantID, "goal", run.Goal)
        logger.Info("starting investigation run")

        hits, retrievalStep, resultErr := service.runRetrievalStep(ctx, &record, request)
        if resultErr != nil {
                return service.finishRunWithError(ctx, record, retrievalStep, resultErr)
        }
        record.Steps = append(record.Steps, retrievalStep)
        record.Run.UpdatedAt = completedAt(retrievalStep)
        if err := service.runs.Upsert(ctx, record); err != nil {
                return record, err
        }

        var (
                sessionEvents []unifiedsessions.Event
                sessionStep   agenttypes.Step
        )
        if request.MaxSteps >= 2 && service.sessions != nil {
                sessionEvents, sessionStep, resultErr = service.runSessionStep(ctx, &record, request, hits)
                if resultErr != nil {
                        return service.finishRunWithError(ctx, record, sessionStep, resultErr)
                }
                record.Steps = append(record.Steps, sessionStep)
                record.Run.UpdatedAt = completedAt(sessionStep)
                if err := service.runs.Upsert(ctx, record); err != nil {
                        return record, err
                }
        }

        if request.MaxSteps >= 3 {
                summaryStep, err := service.runSummaryStep(ctx, &record, request, hits, sessionEvents)
                if err != nil {
                        return service.finishRunWithError(ctx, record, summaryStep, err)
                }
                record.Steps = append(record.Steps, summaryStep)
                record.Run.UpdatedAt = completedAt(summaryStep)
        }

        completed := time.Now().UTC()
        record.Run.Status = agenttypes.RunStatusCompleted
        record.Run.UpdatedAt = completed
        record.Run.CompletedAt = &completed
        if err := service.runs.Upsert(ctx, record); err != nil {
                return record, err
        }

        logger.Info("investigation run completed", "steps", len(record.Steps))
        return record, nil
}

func (service *Service) runRetrievalStep(ctx context.Context, record *agentmetadata.Record, request agenttypes.RunRequest) ([]retrievalcontracts.Hit, agenttypes.Step, error) {
        step := startStep(record.Run.ID, "retrieve", "retrieve", "searching indexed evidence", "retrieve.evidence.search")
        decision, err := service.evaluate(ctx, &record.Run, step.ID, step.ToolName, record.Run.TenantID)
        if err != nil {
                return nil, step, err
        }
        if err := decisionError(decision); err != nil {
                step.Summary = decision.Reason
                return nil, step, err
        }

        query, err := buildRetrievalQuery(request)
        if err != nil {
                return nil, step, err
        }
        hits, err := service.retrieval.SearchEvidence(ctx, query)
        if err != nil {
                return nil, step, err
        }

        step.Status = agenttypes.StepStatusCompleted
        finished := time.Now().UTC()
        step.CompletedAt = &finished
        step.Summary = summarizeRetrievalStep(query.QueryText, hits)
        step.Evidence = evidenceFromHits(hits, maxEvidencePerStep)
        return hits, step, nil
}

func (service *Service) runSessionStep(ctx context.Context, record *agentmetadata.Record, request agenttypes.RunRequest, hits []retrievalcontracts.Hit) ([]unifiedsessions.Event, agenttypes.Step, error) {
        step := startStep(record.Run.ID, "session-enrichment", "query", "fetching normalized session context", "query.sessions.search")
        decision, err := service.evaluate(ctx, &record.Run, step.ID, step.ToolName, record.Run.TenantID)
        if err != nil {
                return nil, step, err
        }
        if err := decisionError(decision); err != nil {
                step.Summary = decision.Reason
                return nil, step, err
        }

        queryFilters, err := buildSessionQueries(request, hits)
        if err != nil {
                return nil, step, err
        }
        events, err := service.searchSessions(ctx, queryFilters)
        if err != nil {
                return nil, step, err
        }

        step.Status = agenttypes.StepStatusCompleted
        finished := time.Now().UTC()
        step.CompletedAt = &finished
        step.Summary = summarizeSessionStep(events)
        step.Evidence = evidenceFromSessionEvents(events, maxEvidencePerStep)
        return events, step, nil
}

func (service *Service) runSummaryStep(ctx context.Context, record *agentmetadata.Record, request agenttypes.RunRequest, hits []retrievalcontracts.Hit, sessionEvents []unifiedsessions.Event) (agenttypes.Step, error) {
        step := startStep(record.Run.ID, "explain", "explain", "assembling deterministic investigation summary", "explain.findings.summary")
        decision, err := service.evaluate(ctx, &record.Run, step.ID, step.ToolName, record.Run.TenantID)
        if err != nil {
                return step, err
        }
        if err := decisionError(decision); err != nil {
                step.Summary = decision.Reason
                return step, err
        }

        step.Status = agenttypes.StepStatusCompleted
        finished := time.Now().UTC()
        step.CompletedAt = &finished
        step.Summary = summarizeInvestigation(request.Goal, hits, sessionEvents)
        step.Evidence = combineEvidence(record.Steps, hits, sessionEvents)
        return step, nil
}

func (service *Service) searchSessions(ctx context.Context, queries []sessionstore.Query) ([]unifiedsessions.Event, error) {
        if service.sessions == nil {
                return nil, nil
        }

        eventsByID := make(map[string]unifiedsessions.Event)
        for _, query := range queries {
                events, err := service.sessions.SearchNetworkSessions(ctx, query)
                if err != nil {
                        return nil, err
                }
                for _, event := range events {
                        eventsByID[event.EventID] = event
                }
        }

        events := make([]unifiedsessions.Event, 0, len(eventsByID))
        for _, event := range eventsByID {
                events = append(events, event)
        }
        sort.Slice(events, func(left, right int) bool {
                if events[left].OccurredAt.Equal(events[right].OccurredAt) {
                        return events[left].IngestedAt.After(events[right].IngestedAt)
                }
                return events[left].OccurredAt.After(events[right].OccurredAt)
        })
        if len(events) > maxEvidencePerStep {
                events = events[:maxEvidencePerStep]
        }
        return events, nil
}

func (service *Service) evaluate(ctx context.Context, run *agenttypes.Run, stepID, toolName, tenantID string) (policy.Decision, error) {
        decision, err := service.policy.Evaluate(ctx, run, policy.Action{
                RunID:          run.ID,
                SessionID:      run.SessionID,
                StepID:         stepID,
                Tool:           tools.Definition{Name: toolName, Description: toolName},
                TargetTenantID: tenantID,
        })
        if err != nil {
                return policy.Decision{}, err
        }
        return decision, nil
}

func (service *Service) finishRunWithError(ctx context.Context, record agentmetadata.Record, step agenttypes.Step, runErr error) (agentmetadata.Record, error) {
        now := time.Now().UTC()
        if step.ID != "" {
                switch {
                case errors.Is(runErr, ErrApprovalRequired):
                        step.Status = agenttypes.StepStatusBlocked
                        record.Run.Status = agenttypes.RunStatusAwaitingApproval
                case errors.Is(runErr, ErrPolicyDenied):
                        step.Status = agenttypes.StepStatusFailed
                        record.Run.Status = agenttypes.RunStatusFailed
                default:
                        step.Status = agenttypes.StepStatusFailed
                        record.Run.Status = agenttypes.RunStatusFailed
                }
                step.CompletedAt = &now
                if strings.TrimSpace(step.Summary) == "" {
                        step.Summary = runErr.Error()
                }
                record.Steps = append(record.Steps, step)
        }

        record.Run.UpdatedAt = now
        if record.Run.Status == agenttypes.RunStatusFailed {
                record.Run.CompletedAt = &now
        }
        if persistErr := service.runs.Upsert(ctx, record); persistErr != nil {
                return record, errors.Join(runErr, persistErr)
        }
        return record, runErr
}

func normalizeRequest(request agenttypes.RunRequest, now time.Time) (agenttypes.RunRequest, error) {
        request.TenantID = strings.TrimSpace(request.TenantID)
        request.CaseID = strings.TrimSpace(request.CaseID)
        request.SessionID = strings.TrimSpace(request.SessionID)
        request.Goal = strings.TrimSpace(request.Goal)
        request.Inputs = cloneInputs(request.Inputs)

        query := strings.TrimSpace(request.Inputs[inputKeyQuery])
        switch {
        case request.Goal == "" && query == "":
                return agenttypes.RunRequest{}, fmt.Errorf("goal or inputs.query is required")
        case request.Goal == "":
                request.Goal = query
        case query == "":
                request.Inputs[inputKeyQuery] = request.Goal
        }

        if request.TenantID == "" {
                return agenttypes.RunRequest{}, fmt.Errorf("tenant_id is required")
        }
        if request.SessionID == "" {
                request.SessionID = composeID("agent-session", request.TenantID, request.Goal, now)
        }
        if request.MaxSteps <= 0 {
                request.MaxSteps = defaultMaxSteps
        }
        if request.MaxSteps > defaultMaxSteps {
                request.MaxSteps = defaultMaxSteps
        }
        return request, nil
}

func buildRetrievalQuery(request agenttypes.RunRequest) (retrievalstore.Query, error) {
        filter := retrievalstore.Query{
                TenantID:        request.TenantID,
                QueryText:       strings.TrimSpace(request.Inputs[inputKeyQuery]),
                SubscriberID:    strings.TrimSpace(request.Inputs[inputKeySubscriberID]),
                SessionID:       strings.TrimSpace(request.Inputs[inputKeySessionID]),
                Status:          strings.TrimSpace(request.Inputs[inputKeyStatus]),
                NASIPAddress:    strings.TrimSpace(request.Inputs[inputKeyNASIPAddress]),
                ClientIPAddress: strings.TrimSpace(request.Inputs[inputKeyClientIP]),
        }
        if filter.QueryText == "" {
                filter.QueryText = request.Goal
        }

        from, err := parseTimeInput(request.Inputs[inputKeyFrom])
        if err != nil {
                return retrievalstore.Query{}, fmt.Errorf("invalid from: %w", err)
        }
        to, err := parseTimeInput(request.Inputs[inputKeyTo])
        if err != nil {
                return retrievalstore.Query{}, fmt.Errorf("invalid to: %w", err)
        }
        filter.From = from
        filter.To = to

        if limitValue := strings.TrimSpace(request.Inputs[inputKeyLimit]); limitValue != "" {
                limit, err := strconv.Atoi(limitValue)
                if err != nil || limit <= 0 {
                        return retrievalstore.Query{}, fmt.Errorf("limit must be a positive integer")
                }
                filter.Limit = limit
        }

        return filter, nil
}

func buildSessionQueries(request agenttypes.RunRequest, hits []retrievalcontracts.Hit) ([]sessionstore.Query, error) {
        baseQuery := sessionstore.Query{
                TenantID:        request.TenantID,
                SubscriberID:    strings.TrimSpace(request.Inputs[inputKeySubscriberID]),
                SessionID:       strings.TrimSpace(request.Inputs[inputKeySessionID]),
                NASIPAddress:    strings.TrimSpace(request.Inputs[inputKeyNASIPAddress]),
                ClientIPAddress: strings.TrimSpace(request.Inputs[inputKeyClientIP]),
                Limit:           maxEvidencePerStep,
        }
        from, err := parseTimeInput(request.Inputs[inputKeyFrom])
        if err != nil {
                return nil, fmt.Errorf("invalid from: %w", err)
        }
        to, err := parseTimeInput(request.Inputs[inputKeyTo])
        if err != nil {
                return nil, fmt.Errorf("invalid to: %w", err)
        }
        baseQuery.From = from
        baseQuery.To = to

        sessionIDs := topSessionIDs(hits, maxSessionLookups)
        if len(sessionIDs) == 0 && baseQuery.SessionID != "" {
                sessionIDs = append(sessionIDs, baseQuery.SessionID)
        }

        if len(sessionIDs) > 0 {
                queries := make([]sessionstore.Query, 0, len(sessionIDs))
                for _, sessionID := range sessionIDs {
                        query := baseQuery
                        query.SessionID = sessionID
                        queries = append(queries, query)
                }
                return queries, nil
        }

        if baseQuery.SessionID == "" && baseQuery.SubscriberID == "" && baseQuery.NASIPAddress == "" && baseQuery.ClientIPAddress == "" {
                return nil, nil
        }

        return []sessionstore.Query{baseQuery}, nil
}

func summarizeRetrievalStep(query string, hits []retrievalcontracts.Hit) string {
        query = strings.TrimSpace(query)
        if len(hits) == 0 {
                return fmt.Sprintf("Retrieved no indexed evidence hits for query %q.", query)
        }
        return fmt.Sprintf("Retrieved %d indexed evidence hits for query %q. Top match: %s.", len(hits), query, hits[0].Document.Title)
}

func summarizeSessionStep(events []unifiedsessions.Event) string {
        if len(events) == 0 {
                return "No normalized session context matched the current investigation evidence."
        }

        sessionIDs := make([]string, 0, len(events))
        seen := make(map[string]struct{})
        for _, event := range events {
                if _, ok := seen[event.SessionID]; ok {
                        continue
                }
                seen[event.SessionID] = struct{}{}
                sessionIDs = append(sessionIDs, event.SessionID)
        }
        sort.Strings(sessionIDs)
        return fmt.Sprintf("Fetched %d normalized session events across %d session IDs: %s.", len(events), len(sessionIDs), strings.Join(sessionIDs, ", "))
}

func summarizeInvestigation(goal string, hits []retrievalcontracts.Hit, events []unifiedsessions.Event) string {
        parts := []string{fmt.Sprintf("Investigated goal %q.", strings.TrimSpace(goal))}
        if len(hits) == 0 {
                parts = append(parts, "No indexed evidence matched the current query.")
        } else {
                top := hits[0].Document
                parts = append(parts, fmt.Sprintf("Top evidence is %s with score %.2f.", top.Title, hits[0].Score))
        }
        if len(events) == 0 {
                parts = append(parts, "No normalized session context was available for further enrichment.")
        } else {
                latest := events[0]
                parts = append(parts, fmt.Sprintf("Latest normalized session state is %s for subscriber %s on session %s at %s.", latest.Status, latest.SubscriberID, latest.SessionID, latest.OccurredAt.Format(time.RFC3339)))
                if latest.NASIPAddress != "" {
                        parts = append(parts, fmt.Sprintf("The most recent network edge context points to NAS %s.", latest.NASIPAddress))
                }
        }
        return strings.Join(parts, " ")
}

func evidenceFromHits(hits []retrievalcontracts.Hit, limit int) []agenttypes.EvidenceRef {
        if limit <= 0 || len(hits) < limit {
                limit = len(hits)
        }
        evidence := make([]agenttypes.EvidenceRef, 0, limit)
        for _, hit := range hits[:limit] {
                evidence = append(evidence, agenttypes.EvidenceRef{
                        Kind:    "retrieval_document",
                        ID:      hit.Document.DocumentID,
                        Source:  hit.Document.Source,
                        Summary: hit.Document.Title,
                        URI:     "/v1/retrieve#document:" + hit.Document.DocumentID,
                        Score:   hit.Score,
                })
        }
        return evidence
}

func evidenceFromSessionEvents(events []unifiedsessions.Event, limit int) []agenttypes.EvidenceRef {
        if limit <= 0 || len(events) < limit {
                limit = len(events)
        }
        evidence := make([]agenttypes.EvidenceRef, 0, limit)
        for _, event := range events[:limit] {
                evidence = append(evidence, agenttypes.EvidenceRef{
                        Kind:    "network_session",
                        ID:      event.EventID,
                        Source:  event.Source,
                        Summary: event.SemanticText,
                        URI:     fmt.Sprintf("/v1/sessions?tenant_id=%s&session_id=%s", event.TenantID, event.SessionID),
                        Score:   1.0,
                })
        }
        return evidence
}

func combineEvidence(existingSteps []agenttypes.Step, hits []retrievalcontracts.Hit, events []unifiedsessions.Event) []agenttypes.EvidenceRef {
        evidence := make(map[string]agenttypes.EvidenceRef)
        for _, step := range existingSteps {
                for _, ref := range step.Evidence {
                        evidence[ref.Kind+":"+ref.ID] = ref
                }
        }
        for _, ref := range evidenceFromHits(hits, maxEvidencePerStep) {
                evidence[ref.Kind+":"+ref.ID] = ref
        }
        for _, ref := range evidenceFromSessionEvents(events, maxEvidencePerStep) {
                evidence[ref.Kind+":"+ref.ID] = ref
        }

        combined := make([]agenttypes.EvidenceRef, 0, len(evidence))
        for _, ref := range evidence {
                combined = append(combined, ref)
        }
        sort.Slice(combined, func(left, right int) bool {
                if combined[left].Score == combined[right].Score {
                        return combined[left].ID < combined[right].ID
                }
                return combined[left].Score > combined[right].Score
        })
        if len(combined) > maxEvidencePerStep {
                combined = combined[:maxEvidencePerStep]
        }
        return combined
}

func topSessionIDs(hits []retrievalcontracts.Hit, limit int) []string {
        if limit <= 0 {
                return nil
        }

        seen := make(map[string]struct{})
        sessionIDs := make([]string, 0, limit)
        for _, hit := range hits {
                sessionID := strings.TrimSpace(hit.Document.SessionID)
                if sessionID == "" {
                        continue
                }
                if _, ok := seen[sessionID]; ok {
                        continue
                }
                seen[sessionID] = struct{}{}
                sessionIDs = append(sessionIDs, sessionID)
                if len(sessionIDs) == limit {
                        break
                }
        }
        return sessionIDs
}

func decisionError(decision policy.Decision) error {
        switch decision.State {
        case policy.DecisionAllow:
                return nil
        case policy.DecisionRequireApproval:
                return ErrApprovalRequired
        case policy.DecisionDeny:
                return ErrPolicyDenied
        default:
                return fmt.Errorf("unsupported policy decision %q", decision.State)
        }
}

func completedAt(step agenttypes.Step) time.Time {
        if step.CompletedAt != nil {
                return step.CompletedAt.UTC()
        }
        return time.Now().UTC()
}

func startStep(runID, suffix, kind, summary, toolName string) agenttypes.Step {
        now := time.Now().UTC()
        return agenttypes.Step{
                ID:        composeID("step", runID, suffix, now),
                Kind:      kind,
                Summary:   summary,
                ToolName:  toolName,
                Status:    agenttypes.StepStatusRunning,
                StartedAt: now,
        }
}

func cloneInputs(values map[string]string) map[string]string {
        cloned := make(map[string]string, len(values))
        for key, value := range values {
                key = strings.TrimSpace(key)
                if key == "" {
                        continue
                }
                cloned[key] = strings.TrimSpace(value)
        }
        return cloned
}

func parseTimeInput(value string) (*time.Time, error) {
        value = strings.TrimSpace(value)
        if value == "" {
                return nil, nil
        }

        parsed, err := time.Parse(time.RFC3339, value)
        if err != nil {
                parsed, err = time.Parse(time.RFC3339Nano, value)
                if err != nil {
                        return nil, err
                }
        }
        parsed = parsed.UTC()
        return &parsed, nil
}

func composeID(kind string, parts ...any) string {
        values := []string{kind}
        for _, part := range parts {
                switch value := part.(type) {
                case string:
                        value = sanitizeIDPart(value)
                        if value != "" {
                                values = append(values, value)
                        }
                case time.Time:
                        values = append(values, value.UTC().Format("20060102T150405.000000000"))
                default:
                        text := sanitizeIDPart(fmt.Sprint(value))
                        if text != "" {
                                values = append(values, text)
                        }
                }
        }
        if len(values) == 1 {
                values = append(values, fmt.Sprintf("%d", time.Now().UTC().UnixNano()))
        }
        return strings.Join(values, ":")
}

func sanitizeIDPart(value string) string {
        value = strings.TrimSpace(strings.ToLower(value))
        if value == "" {
                return ""
        }

        var builder strings.Builder
        for _, r := range value {
                switch {
                case unicode.IsLetter(r), unicode.IsDigit(r):
                        builder.WriteRune(r)
                case r == '-', r == '_', r == '.', r == ':':
                        builder.WriteRune(r)
                default:
                        builder.WriteRune('-')
                }
        }
        return strings.Trim(builder.String(), "-")
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

func (service *Service) logger() *slog.Logger {
        if service != nil && service.baseLogger != nil {
                return service.baseLogger
        }
        return slog.New(slog.NewTextHandler(io.Discard, nil))
}
