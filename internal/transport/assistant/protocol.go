package assistant

import (
	"fmt"
	"strings"
	"time"

	agenttypes "github.com/asimarora/semantic-intelligence-layer/internal/agents"
	agentmetadata "github.com/asimarora/semantic-intelligence-layer/internal/storage/metadata/agents"
)

const MinMaxSteps = 4

type Filters struct {
	SubscriberID    string `json:"subscriber_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	RequestID       string `json:"request_id,omitempty"`
	Status          string `json:"status,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
	NASIPAddress    string `json:"nas_ip_address,omitempty"`
	ClientIPAddress string `json:"client_ip_address,omitempty"`
	From            string `json:"from,omitempty"`
	To              string `json:"to,omitempty"`
	Limit           int    `json:"limit,omitempty"`
}

type MessageRequest struct {
	Type         string  `json:"type,omitempty"`
	TenantID     string  `json:"tenant_id"`
	CaseID       string  `json:"case_id,omitempty"`
	RunSessionID string  `json:"run_session_id,omitempty"`
	Message      string  `json:"message"`
	Filters      Filters `json:"filters,omitempty"`
	MaxSteps     int     `json:"max_steps,omitempty"`
}

type StepPayload struct {
	ID            string `json:"id,omitempty"`
	Kind          string `json:"kind,omitempty"`
	ToolName      string `json:"tool_name"`
	Summary       string `json:"summary"`
	Status        string `json:"status"`
	EvidenceCount int    `json:"evidence_count,omitempty"`
}

type MessageResponse struct {
	Type        string        `json:"type"`
	Message     string        `json:"message,omitempty"`
	ShortAnswer string        `json:"short_answer,omitempty"`
	Error       string        `json:"error,omitempty"`
	RunID       string        `json:"run_id,omitempty"`
	RunStatus   string        `json:"run_status,omitempty"`
	Goal        string        `json:"goal,omitempty"`
	Steps       []StepPayload `json:"steps,omitempty"`
	StartedAt   string        `json:"started_at,omitempty"`
	CompletedAt string        `json:"completed_at,omitempty"`
	Timestamp   string        `json:"timestamp"`
}

func BuildRunRequest(message MessageRequest, minMaxSteps int) (agenttypes.RunRequest, error) {
	messageType := strings.ToLower(strings.TrimSpace(message.Type))
	if messageType != "" && messageType != "ask" && messageType != "message" {
		return agenttypes.RunRequest{}, fmt.Errorf("message type must be empty, ask, or message")
	}
	if strings.TrimSpace(message.TenantID) == "" {
		return agenttypes.RunRequest{}, fmt.Errorf("tenant_id is required")
	}
	if strings.TrimSpace(message.Message) == "" {
		return agenttypes.RunRequest{}, fmt.Errorf("message is required")
	}

	maxSteps := message.MaxSteps
	if maxSteps < minMaxSteps {
		maxSteps = minMaxSteps
	}

	text := strings.TrimSpace(message.Message)
	inputs := map[string]string{
		"query": text,
	}
	addInput(inputs, "subscriber_id", message.Filters.SubscriberID)
	addInput(inputs, "session_id", message.Filters.SessionID)
	addInput(inputs, "request_id", message.Filters.RequestID)
	addInput(inputs, "status", message.Filters.Status)
	addInput(inputs, "outcome", message.Filters.Outcome)
	addInput(inputs, "nas_ip_address", message.Filters.NASIPAddress)
	addInput(inputs, "client_ip_address", message.Filters.ClientIPAddress)
	addInput(inputs, "from", message.Filters.From)
	addInput(inputs, "to", message.Filters.To)
	if message.Filters.Limit > 0 {
		inputs["limit"] = fmt.Sprintf("%d", message.Filters.Limit)
	}

	return agenttypes.RunRequest{
		TenantID:  strings.TrimSpace(message.TenantID),
		CaseID:    strings.TrimSpace(message.CaseID),
		SessionID: strings.TrimSpace(message.RunSessionID),
		Goal:      text,
		MaxSteps:  maxSteps,
		Inputs:    inputs,
	}, nil
}

func BuildResultResponse(record agentmetadata.Record) MessageResponse {
	completedAt := ""
	if record.Run.CompletedAt != nil {
		completedAt = record.Run.CompletedAt.UTC().Format(time.RFC3339)
	}
	return MessageResponse{
		Type:        "assistant_message",
		Message:     latestStepSummary(record),
		ShortAnswer: extractShortAnswer(latestStepSummary(record)),
		RunID:       record.Run.ID,
		RunStatus:   string(record.Run.Status),
		Goal:        record.Run.Goal,
		Steps:       mapSteps(record.Steps),
		StartedAt:   formatTime(record.Run.StartedAt),
		CompletedAt: completedAt,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func BuildErrorResponse(err error, record agentmetadata.Record) MessageResponse {
	completedAt := ""
	if record.Run.CompletedAt != nil {
		completedAt = record.Run.CompletedAt.UTC().Format(time.RFC3339)
	}
	message := latestStepSummary(record)
	shortAnswer := strings.TrimSpace(err.Error())
	if message != "" {
		shortAnswer = extractShortAnswer(message)
	}
	return MessageResponse{
		Type:        "error",
		Message:     message,
		ShortAnswer: shortAnswer,
		Error:       err.Error(),
		RunID:       record.Run.ID,
		RunStatus:   string(record.Run.Status),
		Goal:        record.Run.Goal,
		StartedAt:   formatTime(record.Run.StartedAt),
		CompletedAt: completedAt,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
	}
}

func addInput(inputs map[string]string, key, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	inputs[key] = value
}

func extractShortAnswer(summary string) string {
	summary = strings.TrimSpace(summary)
	if summary == "" {
		return ""
	}
	sentences := splitSentences(summary)
	if len(sentences) == 0 {
		return summary
	}
	if len(sentences) == 1 {
		return sentences[0]
	}
	return strings.Join(sentences[:2], " ")
}

func splitSentences(summary string) []string {
	parts := strings.Split(summary, ". ")
	sentences := make([]string, 0, len(parts))
	for index, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if index < len(parts)-1 && !strings.HasSuffix(part, ".") {
			part += "."
		}
		sentences = append(sentences, part)
	}
	return sentences
}

func mapSteps(steps []agenttypes.Step) []StepPayload {
	payload := make([]StepPayload, 0, len(steps))
	for _, step := range steps {
		payload = append(payload, StepPayload{
			ID:            step.ID,
			Kind:          step.Kind,
			ToolName:      step.ToolName,
			Summary:       step.Summary,
			Status:        string(step.Status),
			EvidenceCount: len(step.Evidence),
		})
	}
	return payload
}

func latestStepSummary(record agentmetadata.Record) string {
	for index := len(record.Steps) - 1; index >= 0; index-- {
		summary := strings.TrimSpace(record.Steps[index].Summary)
		if summary != "" {
			return summary
		}
	}
	if strings.TrimSpace(record.Run.Goal) != "" {
		return fmt.Sprintf("Investigation for %q completed with status %s.", record.Run.Goal, record.Run.Status)
	}
	if record.Run.Status != "" {
		return fmt.Sprintf("Investigation completed with status %s.", record.Run.Status)
	}
	return ""
}

func formatTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}
