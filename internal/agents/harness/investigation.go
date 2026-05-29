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
	unifiedaccess "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/access"
	retrievalcontracts "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/retrieval"
	unifiedsessions "github.com/asimarora/semantic-intelligence-layer/internal/contracts/unified/sessions"
	sessionquery "github.com/asimarora/semantic-intelligence-layer/internal/services/query"
	accessstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/access"
	agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
	retrievalstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/retrieval"
	sessionstore "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/sessions"
)

const (
	defaultMaxSteps       = 4
	maxHarnessSteps       = 5
	maxEvidencePerStep    = 5
	maxAccessLookups      = 3
	maxSessionLookups     = 3
	correlationWindowPad  = 15 * time.Minute
	defaultHarnessRuntime = "deterministic-harness"
	defaultHarnessPlanner = "deterministic-investigation-v1"
	inputKeyQuery         = "query"
	inputKeySubscriberID  = "subscriber_id"
	inputKeySessionID     = "session_id"
	inputKeyRequestID     = "request_id"
	inputKeyStatus        = "status"
	inputKeyOutcome       = "outcome"
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
	access      *sessionquery.AccessService
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
		access:      deps.Access,
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
		accessEvents  []unifiedaccess.Event
		accessStep    agenttypes.Step
		sessionEvents []unifiedsessions.Event
		sessionStep   agenttypes.Step
	)
	remainingSteps := request.MaxSteps - 1
	runSummary := remainingSteps > 0
	enrichmentBudget := remainingSteps
	if runSummary {
		enrichmentBudget--
	}

	if enrichmentBudget > 0 && service.access != nil {
		accessEvents, accessStep, resultErr = service.runAccessStep(ctx, &record, request, hits)
		if resultErr != nil {
			return service.finishRunWithError(ctx, record, accessStep, resultErr)
		}
		record.Steps = append(record.Steps, accessStep)
		record.Run.UpdatedAt = completedAt(accessStep)
		if err := service.runs.Upsert(ctx, record); err != nil {
			return record, err
		}
		enrichmentBudget--
	}

	if enrichmentBudget > 0 && service.sessions != nil {
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

	if runSummary {
		summaryStep, err := service.runSummaryStep(ctx, &record, request, hits, accessEvents, sessionEvents)
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

func (service *Service) runAccessStep(ctx context.Context, record *agentmetadata.Record, request agenttypes.RunRequest, hits []retrievalcontracts.Hit) ([]unifiedaccess.Event, agenttypes.Step, error) {
	step := startStep(record.Run.ID, "access-enrichment", "query", "fetching normalized access context", "query.access.search")
	decision, err := service.evaluate(ctx, &record.Run, step.ID, step.ToolName, record.Run.TenantID)
	if err != nil {
		return nil, step, err
	}
	if err := decisionError(decision); err != nil {
		step.Summary = decision.Reason
		return nil, step, err
	}

	queryFilters, err := buildAccessQueries(request, hits)
	if err != nil {
		return nil, step, err
	}
	events, err := service.searchAccess(ctx, queryFilters)
	if err != nil {
		return nil, step, err
	}

	step.Status = agenttypes.StepStatusCompleted
	finished := time.Now().UTC()
	step.CompletedAt = &finished
	step.Summary = summarizeAccessStep(events)
	step.Evidence = evidenceFromAccessEvents(events, maxEvidencePerStep)
	return events, step, nil
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

func (service *Service) runSummaryStep(ctx context.Context, record *agentmetadata.Record, request agenttypes.RunRequest, hits []retrievalcontracts.Hit, accessEvents []unifiedaccess.Event, sessionEvents []unifiedsessions.Event) (agenttypes.Step, error) {
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
	step.Summary = summarizeInvestigation(request.Goal, hits, accessEvents, sessionEvents)
	step.Evidence = combineEvidence(record.Steps, hits, accessEvents, sessionEvents)
	return step, nil
}

func (service *Service) searchAccess(ctx context.Context, queries []accessstore.Query) ([]unifiedaccess.Event, error) {
	if service.access == nil {
		return nil, nil
	}

	eventsByID := make(map[string]unifiedaccess.Event)
	for _, query := range queries {
		events, err := service.access.SearchNetworkAccessEvents(ctx, query)
		if err != nil {
			return nil, err
		}
		for _, event := range events {
			eventsByID[event.EventID] = event
		}
	}

	events := make([]unifiedaccess.Event, 0, len(eventsByID))
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
	if boundedQuery := rewriteBoundedQuestionQuery(request.Goal, request.Inputs); boundedQuery != "" {
		request.Inputs[inputKeyQuery] = boundedQuery
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
	if request.MaxSteps > maxHarnessSteps {
		request.MaxSteps = maxHarnessSteps
	}
	return request, nil
}

func buildRetrievalQuery(request agenttypes.RunRequest) (retrievalstore.Query, error) {
	filter := retrievalstore.Query{
		TenantID:        request.TenantID,
		QueryText:       strings.TrimSpace(request.Inputs[inputKeyQuery]),
		SubscriberID:    strings.TrimSpace(request.Inputs[inputKeySubscriberID]),
		SessionID:       strings.TrimSpace(request.Inputs[inputKeySessionID]),
		RequestID:       strings.TrimSpace(request.Inputs[inputKeyRequestID]),
		Status:          strings.TrimSpace(request.Inputs[inputKeyStatus]),
		Outcome:         strings.TrimSpace(request.Inputs[inputKeyOutcome]),
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

func buildAccessQueries(request agenttypes.RunRequest, hits []retrievalcontracts.Hit) ([]accessstore.Query, error) {
	baseQuery := accessstore.Query{
		TenantID:        request.TenantID,
		SubscriberID:    strings.TrimSpace(request.Inputs[inputKeySubscriberID]),
		RequestID:       strings.TrimSpace(request.Inputs[inputKeyRequestID]),
		SessionID:       strings.TrimSpace(request.Inputs[inputKeySessionID]),
		Outcome:         strings.TrimSpace(request.Inputs[inputKeyOutcome]),
		NASIPAddress:    strings.TrimSpace(request.Inputs[inputKeyNASIPAddress]),
		ClientIPAddress: strings.TrimSpace(request.Inputs[inputKeyClientIP]),
		Limit:           maxEvidencePerStep,
	}
	from, to, err := resolveInvestigationWindow(request, hits)
	if err != nil {
		return nil, err
	}
	baseQuery.From = from
	baseQuery.To = to

	if baseQuery.SubscriberID == "" {
		baseQuery.SubscriberID = firstHitSubscriberID(hits)
	}
	if baseQuery.NASIPAddress == "" {
		baseQuery.NASIPAddress = firstHitNASIPAddress(hits)
	}
	if baseQuery.ClientIPAddress == "" {
		baseQuery.ClientIPAddress = firstHitClientIPAddress(hits)
	}
	if baseQuery.SessionID == "" && baseQuery.RequestID == "" && baseQuery.SubscriberID == "" && baseQuery.NASIPAddress == "" && baseQuery.ClientIPAddress == "" {
		baseQuery.SessionID = firstHitSessionID(hits)
	}
	if baseQuery.RequestID == "" && baseQuery.SubscriberID == "" && baseQuery.SessionID == "" && baseQuery.NASIPAddress == "" && baseQuery.ClientIPAddress == "" {
		baseQuery.RequestID = firstHitRequestID(hits)
	}

	if baseQuery.RequestID == "" && baseQuery.SubscriberID == "" && baseQuery.SessionID == "" && baseQuery.NASIPAddress == "" && baseQuery.ClientIPAddress == "" && baseQuery.Outcome == "" {
		return nil, nil
	}
	return []accessstore.Query{baseQuery}, nil
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
	from, to, err := resolveInvestigationWindow(request, hits)
	if err != nil {
		return nil, err
	}
	baseQuery.From = from
	baseQuery.To = to
	if baseQuery.SubscriberID == "" {
		baseQuery.SubscriberID = firstHitSubscriberID(hits)
	}
	if baseQuery.NASIPAddress == "" {
		baseQuery.NASIPAddress = firstHitNASIPAddress(hits)
	}
	if baseQuery.ClientIPAddress == "" {
		baseQuery.ClientIPAddress = firstHitClientIPAddress(hits)
	}

	sessionIDs := topSessionIDs(hits, maxSessionLookups)
	if baseQuery.SubscriberID != "" || baseQuery.NASIPAddress != "" || baseQuery.ClientIPAddress != "" {
		sessionIDs = nil
	}
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

func resolveInvestigationWindow(request agenttypes.RunRequest, hits []retrievalcontracts.Hit) (*time.Time, *time.Time, error) {
	from, err := parseTimeInput(request.Inputs[inputKeyFrom])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid from: %w", err)
	}
	to, err := parseTimeInput(request.Inputs[inputKeyTo])
	if err != nil {
		return nil, nil, fmt.Errorf("invalid to: %w", err)
	}
	if from != nil && to != nil && from.After(*to) {
		return nil, nil, fmt.Errorf("from must be before or equal to to")
	}
	if from != nil && to != nil {
		return from, to, nil
	}

	earliest, latest := hitWindow(hits)
	if earliest.IsZero() || latest.IsZero() {
		return from, to, nil
	}
	if from == nil {
		value := earliest.Add(-correlationWindowPad)
		from = &value
	}
	if to == nil {
		value := latest.Add(correlationWindowPad)
		to = &value
	}
	return from, to, nil
}

func hitWindow(hits []retrievalcontracts.Hit) (time.Time, time.Time) {
	if len(hits) == 0 {
		return time.Time{}, time.Time{}
	}
	earliest := hits[0].Document.OccurredAt.UTC()
	latest := earliest
	for _, hit := range hits[1:] {
		occurredAt := hit.Document.OccurredAt.UTC()
		if occurredAt.Before(earliest) {
			earliest = occurredAt
		}
		if occurredAt.After(latest) {
			latest = occurredAt
		}
	}
	return earliest, latest
}

func summarizeRetrievalStep(query string, hits []retrievalcontracts.Hit) string {
	query = strings.TrimSpace(query)
	if len(hits) == 0 {
		return fmt.Sprintf("Retrieved no indexed evidence hits for query %q.", query)
	}
	return fmt.Sprintf("Retrieved %d indexed evidence hits for query %q. Top match: %s.", len(hits), query, hits[0].Document.Title)
}

func summarizeAccessStep(events []unifiedaccess.Event) string {
	if len(events) == 0 {
		return "No normalized access context matched the current investigation evidence."
	}

	summaries := make([]string, 0, len(events))
	seen := make(map[string]struct{})
	for _, event := range sortAccessAscending(events) {
		requestID := strings.TrimSpace(event.RequestID)
		if requestID == "" {
			requestID = event.EventID
		}
		if _, ok := seen[requestID]; ok {
			continue
		}
		seen[requestID] = struct{}{}

		entry := fmt.Sprintf("%s (%s)", requestID, event.Outcome)
		if reason := strings.TrimSpace(event.RejectReason); reason != "" {
			entry += ": " + reason
		}
		summaries = append(summaries, entry)
	}
	return fmt.Sprintf("Fetched %d normalized access events across %d requests: %s.", len(events), len(summaries), strings.Join(summaries, ", "))
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

func summarizeInvestigation(goal string, hits []retrievalcontracts.Hit, accessEvents []unifiedaccess.Event, sessionEvents []unifiedsessions.Event) string {
	if directAnswer := summarizeGoalAnswer(goal, hits, accessEvents, sessionEvents); directAnswer != "" {
		parts := []string{directAnswer}
		if correlationSummary := SummarizeCrossSourceCorrelation(accessEvents, sessionEvents); correlationSummary != "" {
			parts = append(parts, correlationSummary)
		}
		if accessSummary := summarizeAccessTimeline(accessEvents); accessSummary != "" {
			parts = append(parts, accessSummary)
		}
		if sessionSummary := summarizeSessionTimeline(sessionEvents); sessionSummary != "" {
			parts = append(parts, sessionSummary)
		}
		return strings.Join(parts, " ")
	}

	parts := []string{fmt.Sprintf("Investigated goal %q.", strings.TrimSpace(goal))}
	if len(hits) == 0 {
		parts = append(parts, "No indexed evidence matched the current query.")
	} else {
		top := hits[0].Document
		parts = append(parts, fmt.Sprintf("Top evidence is %s with score %.2f.", top.Title, hits[0].Score))
		if top.EntityType == retrievalcontracts.EntityTypeNetworkAccess {
			parts = append(parts, fmt.Sprintf("The strongest access signal is outcome %s for request %s.", top.Outcome, top.RequestID))
			if top.RejectReason != "" {
				parts = append(parts, fmt.Sprintf("The access decision was explained as %s.", top.RejectReason))
			}
			if top.PolicyName != "" {
				parts = append(parts, fmt.Sprintf("The access policy context points to %s.", top.PolicyName))
			}
		}
	}
	if accessSummary := summarizeAccessTimeline(accessEvents); accessSummary != "" {
		parts = append(parts, accessSummary)
	} else {
		parts = append(parts, "No normalized access context was available for further enrichment.")
	}
	if sessionSummary := summarizeSessionTimeline(sessionEvents); sessionSummary != "" {
		parts = append(parts, sessionSummary)
	} else {
		parts = append(parts, "No normalized session context was available for further enrichment.")
	}
	if correlationSummary := SummarizeCrossSourceCorrelation(accessEvents, sessionEvents); correlationSummary != "" {
		parts = append(parts, correlationSummary)
	}
	return strings.Join(parts, " ")
}

func summarizeGoalAnswer(goal string, hits []retrievalcontracts.Hit, accessEvents []unifiedaccess.Event, sessionEvents []unifiedsessions.Event) string {
	normalizedGoal := strings.ToLower(strings.TrimSpace(goal))
	switch {
	case isBeforeConnectQuestion(normalizedGoal):
		orderedAccess := sortAccessAscending(accessEvents)
		orderedSessions := sortSessionsAscending(sessionEvents)
		firstStart := firstSessionEventWithStatus(orderedSessions, unifiedsessions.StatusStart)
		rejectedBeforeStart := latestAccessOutcomeBefore(orderedAccess, unifiedaccess.OutcomeRejected, firstStart)
		switch {
		case rejectedBeforeStart != nil && firstStart != nil:
			return "Yes. Authentication failed before the subscriber connected."
		case rejectedBeforeStart != nil && firstStart == nil:
			return "Partially. Authentication failed, but no session start is available in the current investigation window."
		case rejectedBeforeStart == nil && firstStart != nil:
			return "No. No rejected authentication appeared before the subscriber connected in the current investigation window."
		default:
			return ""
		}
	case isStatusQuestion(normalizedGoal):
		return summarizeStatusAnswer(accessEvents, sessionEvents)
	case isTenantMembershipQuestion(normalizedGoal):
		return summarizeTenantMembershipAnswer(hits, accessEvents, sessionEvents)
	case isDisconnectReasonQuestion(normalizedGoal):
		return summarizeDisconnectReasonAnswer(sessionEvents)
	case isQuestionLike(normalizedGoal):
		return "I don't have a grounded answer template for that question yet. Review the details below for the evidence I found."
	default:
		return ""
	}
}

func rewriteBoundedQuestionQuery(goal string, inputs map[string]string) string {
	goal = strings.TrimSpace(goal)
	query := strings.TrimSpace(inputs[inputKeyQuery])
	if goal == "" {
		return ""
	}
	if query != "" && !strings.EqualFold(query, goal) {
		return ""
	}

	normalizedGoal := strings.ToLower(goal)
	subscriberID := strings.TrimSpace(inputs[inputKeySubscriberID])
	switch {
	case isBeforeConnectQuestion(normalizedGoal):
		return joinQueryTerms(subscriberID, "authentication", "rejected", "accepted", "connected", "session", "start", "invalid password")
	case isStatusQuestion(normalizedGoal):
		return joinQueryTerms(subscriberID, "session", "status", "connected", "disconnected", "latest")
	case isTenantMembershipQuestion(normalizedGoal):
		return joinQueryTerms(subscriberID, "subscriber", "tenant", "membership")
	case isDisconnectReasonQuestion(normalizedGoal):
		return joinQueryTerms(subscriberID, "session", "stop", "disconnect", "disconnected", "latest")
	default:
		return ""
	}
}

func isBeforeConnectQuestion(normalizedGoal string) bool {
	if !strings.HasPrefix(normalizedGoal, "did ") {
		return false
	}
	return containsAny(normalizedGoal,
		"fail auth before connect",
		"fail auth before connecting",
		"failed auth before connect",
		"failed auth before connecting",
		"authentication fail before connect",
		"authentication failed before connect",
		"authentication fail before connecting",
		"authentication failed before connecting",
	)
}

func isStatusQuestion(normalizedGoal string) bool {
	if normalizedGoal == "" {
		return false
	}
	return strings.HasPrefix(normalizedGoal, "how is ") ||
		strings.HasPrefix(normalizedGoal, "how's ") ||
		strings.HasPrefix(normalizedGoal, "what is the status of ") ||
		strings.HasPrefix(normalizedGoal, "what's the status of ") ||
		strings.HasPrefix(normalizedGoal, "status of ")
}

func isTenantMembershipQuestion(normalizedGoal string) bool {
	if normalizedGoal == "" || !strings.Contains(normalizedGoal, "tenant") {
		return false
	}
	return containsAny(normalizedGoal,
		"belong to this tenant",
		"belongs to this tenant",
		"member of this tenant",
		"part of this tenant",
		"within this tenant",
	)
}

func isDisconnectReasonQuestion(normalizedGoal string) bool {
	if normalizedGoal == "" {
		return false
	}
	return containsAny(normalizedGoal,
		"why did",
		"why was",
	) && containsAny(normalizedGoal,
		"disconnect",
		"disconnected",
	)
}

func isQuestionLike(normalizedGoal string) bool {
	normalizedGoal = strings.TrimSpace(normalizedGoal)
	if normalizedGoal == "" {
		return false
	}
	if strings.HasSuffix(normalizedGoal, "?") {
		return true
	}
	return strings.HasPrefix(normalizedGoal, "who ") ||
		strings.HasPrefix(normalizedGoal, "what ") ||
		strings.HasPrefix(normalizedGoal, "when ") ||
		strings.HasPrefix(normalizedGoal, "where ") ||
		strings.HasPrefix(normalizedGoal, "why ") ||
		strings.HasPrefix(normalizedGoal, "how ") ||
		strings.HasPrefix(normalizedGoal, "does ") ||
		strings.HasPrefix(normalizedGoal, "do ") ||
		strings.HasPrefix(normalizedGoal, "did ") ||
		strings.HasPrefix(normalizedGoal, "is ") ||
		strings.HasPrefix(normalizedGoal, "are ") ||
		strings.HasPrefix(normalizedGoal, "can ") ||
		strings.HasPrefix(normalizedGoal, "should ") ||
		strings.HasPrefix(normalizedGoal, "will ")
}

func summarizeStatusAnswer(accessEvents []unifiedaccess.Event, sessionEvents []unifiedsessions.Event) string {
	orderedAccess := sortAccessAscending(accessEvents)
	orderedSessions := sortSessionsAscending(sessionEvents)
	subscriberID := firstNonEmpty(firstAccessSubscriberID(orderedAccess), firstSessionSubscriberID(orderedSessions))
	if subscriberID == "" {
		subscriberID = "the subscriber"
	}

	if len(orderedSessions) > 0 {
		latestSession := orderedSessions[len(orderedSessions)-1]
		switch latestSession.Status {
		case unifiedsessions.StatusStart:
			return fmt.Sprintf("Subscriber %s is currently connected in the current investigation window.", subscriberID)
		case unifiedsessions.StatusStop:
			return fmt.Sprintf("Subscriber %s is not currently connected in the current investigation window.", subscriberID)
		}
	}

	latestAccepted := latestAccessOutcomeBefore(orderedAccess, unifiedaccess.OutcomeAccepted, nil)
	latestRejected := latestAccessOutcomeBefore(orderedAccess, unifiedaccess.OutcomeRejected, nil)

	switch {
	case latestAccepted != nil:
		return fmt.Sprintf("Subscriber %s authenticated successfully, but no session start is visible in the current investigation window.", subscriberID)
	case latestRejected != nil:
		summary := fmt.Sprintf("Subscriber %s's latest visible state is rejected authentication", subscriberID)
		if reason := strings.TrimSpace(latestRejected.RejectReason); reason != "" {
			summary += " because " + reason
		}
		return summary + "."
	default:
		return ""
	}
}

func summarizeTenantMembershipAnswer(hits []retrievalcontracts.Hit, accessEvents []unifiedaccess.Event, sessionEvents []unifiedsessions.Event) string {
	subscriberID := firstNonEmpty(
		firstHitSubscriberID(hits),
		firstAccessSubscriberID(accessEvents),
		firstSessionSubscriberID(sessionEvents),
	)
	if subscriberID == "" {
		subscriberID = "this subscriber"
	}

	hasEvidence := len(hits) > 0 || len(accessEvents) > 0 || len(sessionEvents) > 0
	if hasEvidence {
		return fmt.Sprintf("Yes. Subscriber %s has evidence in this tenant.", subscriberID)
	}
	return fmt.Sprintf("No. No evidence places %s in this tenant.", subscriberID)
}

func summarizeDisconnectReasonAnswer(sessionEvents []unifiedsessions.Event) string {
	orderedSessions := sortSessionsAscending(sessionEvents)
	latestStop := lastSessionEventWithStatus(orderedSessions, unifiedsessions.StatusStop)
	if latestStop == nil {
		return "I can't explain a disconnect from the current evidence because no session stop is available in the investigation window."
	}

	summary := fmt.Sprintf(
		"The latest visible disconnect was session %s stopping at %s",
		latestStop.SessionID,
		latestStop.OccurredAt.Format(time.RFC3339),
	)
	if latestStop.SessionTimeSeconds != nil {
		summary += fmt.Sprintf(" after %d seconds", *latestStop.SessionTimeSeconds)
	}
	summary += "."
	if strings.TrimSpace(latestStop.SemanticText) == "" {
		summary += " The current evidence does not include an explicit disconnect reason beyond the stop event."
	} else {
		summary += " The current evidence does not expose a richer disconnect cause beyond that stop event."
	}
	return summary
}

func joinQueryTerms(values ...string) string {
	parts := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		parts = append(parts, value)
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

func evidenceFromAccessEvents(events []unifiedaccess.Event, limit int) []agenttypes.EvidenceRef {
	if limit <= 0 || len(events) < limit {
		limit = len(events)
	}
	evidence := make([]agenttypes.EvidenceRef, 0, limit)
	for _, event := range events[:limit] {
		uri := fmt.Sprintf("/v1/access-events?tenant_id=%s&subscriber_id=%s", event.TenantID, event.SubscriberID)
		if requestID := strings.TrimSpace(event.RequestID); requestID != "" {
			uri = fmt.Sprintf("/v1/access-events?tenant_id=%s&request_id=%s", event.TenantID, requestID)
		}
		evidence = append(evidence, agenttypes.EvidenceRef{
			Kind:    "network_access",
			ID:      event.EventID,
			Source:  event.Source,
			Summary: event.SemanticText,
			URI:     uri,
			Score:   1.0,
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

func combineEvidence(existingSteps []agenttypes.Step, hits []retrievalcontracts.Hit, accessEvents []unifiedaccess.Event, sessionEvents []unifiedsessions.Event) []agenttypes.EvidenceRef {
	evidence := make(map[string]agenttypes.EvidenceRef)
	for _, step := range existingSteps {
		for _, ref := range step.Evidence {
			evidence[ref.Kind+":"+ref.ID] = ref
		}
	}
	for _, ref := range evidenceFromHits(hits, maxEvidencePerStep) {
		evidence[ref.Kind+":"+ref.ID] = ref
	}
	for _, ref := range evidenceFromAccessEvents(accessEvents, maxEvidencePerStep) {
		evidence[ref.Kind+":"+ref.ID] = ref
	}
	for _, ref := range evidenceFromSessionEvents(sessionEvents, maxEvidencePerStep) {
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

func summarizeAccessTimeline(events []unifiedaccess.Event) string {
	if len(events) == 0 {
		return ""
	}
	ordered := sortAccessAscending(events)
	segments := make([]string, 0, len(ordered))
	for _, event := range ordered {
		segment := fmt.Sprintf("request %s %s at %s", event.RequestID, event.Outcome, event.OccurredAt.Format(time.RFC3339))
		if reason := strings.TrimSpace(event.RejectReason); reason != "" {
			segment += " because " + reason
		}
		segments = append(segments, segment)
	}
	return "Access timeline shows " + strings.Join(segments, ", then ") + "."
}

func summarizeSessionTimeline(events []unifiedsessions.Event) string {
	if len(events) == 0 {
		return ""
	}
	ordered := sortSessionsAscending(events)
	segments := make([]string, 0, len(ordered))
	for _, event := range ordered {
		segment := fmt.Sprintf("session %s %s at %s", event.SessionID, event.Status, event.OccurredAt.Format(time.RFC3339))
		if event.Status == unifiedsessions.StatusStop && event.SessionTimeSeconds != nil {
			segment += fmt.Sprintf(" after %d seconds", *event.SessionTimeSeconds)
		}
		segments = append(segments, segment)
	}
	return "Session timeline shows " + strings.Join(segments, ", then ") + "."
}

// SummarizeCrossSourceCorrelation explains the strongest deterministic access-to-session timeline finding.
func SummarizeCrossSourceCorrelation(accessEvents []unifiedaccess.Event, sessionEvents []unifiedsessions.Event) string {
	if len(accessEvents) == 0 && len(sessionEvents) == 0 {
		return ""
	}

	orderedAccess := sortAccessAscending(accessEvents)
	orderedSessions := sortSessionsAscending(sessionEvents)
	firstStart := firstSessionEventWithStatus(orderedSessions, unifiedsessions.StatusStart)
	lastStop := lastSessionEventWithStatus(orderedSessions, unifiedsessions.StatusStop)
	rejectedBeforeStart := latestAccessOutcomeBefore(orderedAccess, unifiedaccess.OutcomeRejected, firstStart)
	challengedBeforeStart := latestAccessOutcomeBefore(orderedAccess, unifiedaccess.OutcomeChallenged, firstStart)
	acceptedBeforeStart := latestAccessOutcomeBefore(orderedAccess, unifiedaccess.OutcomeAccepted, firstStart)

	subscriberID := firstNonEmpty(firstAccessSubscriberID(orderedAccess), firstSessionSubscriberID(orderedSessions))
	if subscriberID == "" {
		subscriberID = "the subscriber"
	}

	switch {
	case rejectedBeforeStart != nil && acceptedBeforeStart != nil && firstStart != nil:
		summary := fmt.Sprintf(
			"Correlated finding: subscriber %s was rejected at %s",
			subscriberID,
			rejectedBeforeStart.OccurredAt.Format(time.RFC3339),
		)
		if reason := strings.TrimSpace(rejectedBeforeStart.RejectReason); reason != "" {
			summary += " because " + reason
		}
		summary += fmt.Sprintf(
			", later accepted at %s, and then started session %s at %s.",
			acceptedBeforeStart.OccurredAt.Format(time.RFC3339),
			firstStart.SessionID,
			firstStart.OccurredAt.Format(time.RFC3339),
		)
		if lastStop != nil {
			summary += fmt.Sprintf(" The latest stop in the same timeline was at %s.", lastStop.OccurredAt.Format(time.RFC3339))
		}
		return summary
	case challengedBeforeStart != nil && acceptedBeforeStart != nil && firstStart != nil:
		return fmt.Sprintf(
			"Correlated finding: subscriber %s faced an access challenge at %s, later accepted at %s, and then started session %s at %s.",
			subscriberID,
			challengedBeforeStart.OccurredAt.Format(time.RFC3339),
			acceptedBeforeStart.OccurredAt.Format(time.RFC3339),
			firstStart.SessionID,
			firstStart.OccurredAt.Format(time.RFC3339),
		)
	case acceptedBeforeStart != nil && firstStart != nil:
		return fmt.Sprintf(
			"Correlated finding: subscriber %s was accepted at %s and then started session %s at %s.",
			subscriberID,
			acceptedBeforeStart.OccurredAt.Format(time.RFC3339),
			firstStart.SessionID,
			firstStart.OccurredAt.Format(time.RFC3339),
		)
	case acceptedBeforeStart != nil && firstStart == nil:
		return fmt.Sprintf(
			"Correlated finding: subscriber %s was accepted at %s but no normalized session start followed in the current investigation window.",
			subscriberID,
			acceptedBeforeStart.OccurredAt.Format(time.RFC3339),
		)
	case rejectedBeforeStart != nil && firstStart == nil:
		summary := fmt.Sprintf(
			"Correlated finding: subscriber %s was rejected at %s",
			subscriberID,
			rejectedBeforeStart.OccurredAt.Format(time.RFC3339),
		)
		if reason := strings.TrimSpace(rejectedBeforeStart.RejectReason); reason != "" {
			summary += " because " + reason
		}
		summary += " and no normalized session start followed in the current investigation window."
		return summary
	}
	return ""
}

func firstHitSubscriberID(hits []retrievalcontracts.Hit) string {
	for _, hit := range hits {
		if value := strings.TrimSpace(hit.Document.SubscriberID); value != "" {
			return value
		}
	}
	return ""
}

func firstHitSessionID(hits []retrievalcontracts.Hit) string {
	for _, hit := range hits {
		if value := strings.TrimSpace(hit.Document.SessionID); value != "" {
			return value
		}
	}
	return ""
}

func firstHitRequestID(hits []retrievalcontracts.Hit) string {
	for _, hit := range hits {
		if value := strings.TrimSpace(hit.Document.RequestID); value != "" {
			return value
		}
	}
	return ""
}

func firstHitNASIPAddress(hits []retrievalcontracts.Hit) string {
	for _, hit := range hits {
		if value := strings.TrimSpace(hit.Document.NASIPAddress); value != "" {
			return value
		}
	}
	return ""
}

func firstHitClientIPAddress(hits []retrievalcontracts.Hit) string {
	for _, hit := range hits {
		if value := strings.TrimSpace(hit.Document.ClientIPAddress); value != "" {
			return value
		}
	}
	return ""
}

func sortAccessAscending(events []unifiedaccess.Event) []unifiedaccess.Event {
	ordered := append([]unifiedaccess.Event(nil), events...)
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].OccurredAt.Equal(ordered[right].OccurredAt) {
			return ordered[left].IngestedAt.Before(ordered[right].IngestedAt)
		}
		return ordered[left].OccurredAt.Before(ordered[right].OccurredAt)
	})
	return ordered
}

func sortSessionsAscending(events []unifiedsessions.Event) []unifiedsessions.Event {
	ordered := append([]unifiedsessions.Event(nil), events...)
	sort.Slice(ordered, func(left, right int) bool {
		if ordered[left].OccurredAt.Equal(ordered[right].OccurredAt) {
			return ordered[left].IngestedAt.Before(ordered[right].IngestedAt)
		}
		return ordered[left].OccurredAt.Before(ordered[right].OccurredAt)
	})
	return ordered
}

func firstAccessSubscriberID(events []unifiedaccess.Event) string {
	for _, event := range events {
		if value := strings.TrimSpace(event.SubscriberID); value != "" {
			return value
		}
	}
	return ""
}

func firstSessionSubscriberID(events []unifiedsessions.Event) string {
	for _, event := range events {
		if value := strings.TrimSpace(event.SubscriberID); value != "" {
			return value
		}
	}
	return ""
}

func firstSessionEventWithStatus(events []unifiedsessions.Event, status unifiedsessions.Status) *unifiedsessions.Event {
	for _, event := range events {
		if event.Status == status {
			cloned := event
			return &cloned
		}
	}
	return nil
}

func lastSessionEventWithStatus(events []unifiedsessions.Event, status unifiedsessions.Status) *unifiedsessions.Event {
	for index := len(events) - 1; index >= 0; index-- {
		if events[index].Status == status {
			cloned := events[index]
			return &cloned
		}
	}
	return nil
}

func latestAccessOutcomeBefore(events []unifiedaccess.Event, outcome unifiedaccess.Outcome, before *unifiedsessions.Event) *unifiedaccess.Event {
	var latest *unifiedaccess.Event
	for _, event := range events {
		if event.Outcome != outcome {
			continue
		}
		if before != nil && event.OccurredAt.After(before.OccurredAt) {
			continue
		}
		cloned := event
		latest = &cloned
	}
	return latest
}

func containsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
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
